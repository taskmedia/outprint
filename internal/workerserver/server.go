// Package workerserver is the Worker role: an HTTP server that turns one job
// bundle at a time into a PDF. It holds no secret except an optional shared
// token, never talks to the network and restarts (the process exits) after a
// timeout, a kill or an unexpected failure so the next job starts clean.
package workerserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/taskmedia/outprint/internal/bundle"
	"github.com/taskmedia/outprint/internal/worker"
	"github.com/taskmedia/outprint/internal/workerapi"
)

// ErrUnknownPackage is returned by a Renderer when the bundle names a
// Template, Configuration or Document Type this Worker does not have.
var ErrUnknownPackage = errors.New("bundle names an unknown or disallowed package")

// Renderer turns a validated bundle into a PDF. The production
// implementation is PackageRenderer; tests use a fake.
type Renderer interface {
	Render(ctx context.Context, b bundle.Bundle) ([]byte, error)
}

// Config configures the HTTP handler.
type Config struct {
	// Token, if not empty, is required as a bearer token on /render.
	Token  string
	Logger *slog.Logger
	// OnRestart is called, in its own goroutine and after the response was
	// handed to the connection, when the process must exit. While it runs
	// the Worker answers every render with busy.
	OnRestart func(reason string)
}

// Server is the Worker's HTTP handler.
type Server struct {
	cfg      Config
	renderer Renderer
	slot     chan struct{}
}

// New returns a Server. Logger defaults to slog.Default().
func New(cfg Config, r Renderer) *Server {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return &Server{cfg: cfg, renderer: r, slot: make(chan struct{}, 1)}
}

// Handler serves POST /render, GET /healthz (liveness, always 200) and
// GET /readyz (200 only while a job can start).
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+workerapi.HealthPath, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("GET "+workerapi.ReadyPath, s.ready)
	mux.HandleFunc("POST "+workerapi.RenderPath, s.render)
	return mux
}

// statusClientClosedRequest is the nginx convention for a caller that hung
// up. Nobody reads the response; it only shows in logs.
const statusClientClosedRequest = 499

func (s *Server) render(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Token != "" && !workerapi.TokenMatches(r.Header.Get("Authorization"), s.cfg.Token) {
		writeError(w, http.StatusUnauthorized, workerapi.ErrorBody{Phase: workerapi.PhaseAuth, Kind: workerapi.KindUnauthorized})
		return
	}
	if !s.tryAcquire() {
		w.Header().Set("Retry-After", "1")
		writeError(w, http.StatusServiceUnavailable, workerapi.ErrorBody{Phase: workerapi.PhaseQueue, Kind: workerapi.KindBusy})
		return
	}
	restart := ""
	defer func() { s.finish(restart) }()

	start := time.Now()
	b, status, body := s.readBundle(w, r)
	if status != 0 {
		writeError(w, status, body)
		return
	}
	pdf, err := s.safeRender(r.Context(), b)
	if err != nil {
		var status int
		status, body, restart = s.classify(err)
		s.logFailure(body, time.Since(start), err)
		writeError(w, status, body)
		return
	}
	if len(pdf) > workerapi.MaxPDFBytes {
		writeError(w, http.StatusRequestEntityTooLarge, workerapi.ErrorBody{Phase: workerapi.PhaseOutput, Kind: workerapi.KindTooLarge})
		return
	}
	s.cfg.Logger.Debug("render finished", "duration_ms", time.Since(start).Milliseconds(), "bytes", len(pdf))
	w.Header().Set("Content-Type", workerapi.ContentTypePDF)
	w.Header().Set("Content-Length", strconv.Itoa(len(pdf)))
	if _, err := w.Write(pdf); err != nil {
		s.cfg.Logger.Warn("write PDF failed", "error", err)
	}
}

// ready answers 503 while a job runs or the Worker is going away; the slot
// stays taken in both cases.
func (s *Server) ready(w http.ResponseWriter, _ *http.Request) {
	if len(s.slot) > 0 {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *Server) tryAcquire() bool {
	select {
	case s.slot <- struct{}{}:
		return true
	default:
		return false
	}
}

// finish releases the slot, unless the process is about to exit: then the
// slot stays taken so no job starts on a Worker that is going away.
func (s *Server) finish(restartReason string) {
	if restartReason == "" {
		<-s.slot
		return
	}
	s.cfg.Logger.Error("worker restarting", "reason", restartReason)
	if s.cfg.OnRestart != nil {
		go s.cfg.OnRestart(restartReason)
	}
}

// readBundle returns the bundle, or the status and error body to answer.
func (s *Server) readBundle(w http.ResponseWriter, r *http.Request) (bundle.Bundle, int, workerapi.ErrorBody) {
	r.Body = http.MaxBytesReader(w, r.Body, bundle.MaxEncodedBytes)
	b, err := bundle.Decode(r.Body)
	if err == nil {
		return b, 0, workerapi.ErrorBody{}
	}
	var tooBig *http.MaxBytesError
	var limit *bundle.LimitError
	if errors.As(err, &tooBig) || errors.As(err, &limit) {
		return bundle.Bundle{}, http.StatusRequestEntityTooLarge, workerapi.ErrorBody{Phase: workerapi.PhaseBundle, Kind: workerapi.KindTooLarge}
	}
	return bundle.Bundle{}, http.StatusBadRequest, workerapi.ErrorBody{Phase: workerapi.PhaseBundle, Kind: workerapi.KindInvalidBundle}
}

// safeRender turns a panic of the renderer into an error.
func (s *Server) safeRender(ctx context.Context, b bundle.Bundle) (pdf []byte, err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("renderer panicked: %v", p)
		}
	}()
	return s.renderer.Render(ctx, b)
}

// classify maps a render error to a status and body, and says whether the
// process must restart (a non-empty reason).
func (s *Server) classify(err error) (int, workerapi.ErrorBody, string) {
	if errors.Is(err, ErrUnknownPackage) {
		return http.StatusBadRequest, workerapi.ErrorBody{Phase: workerapi.PhaseBundle, Kind: workerapi.KindInvalidBundle}, ""
	}
	var failure *worker.Error
	if !errors.As(err, &failure) {
		return http.StatusInternalServerError, workerapi.ErrorBody{Phase: string(worker.PhaseSetup), Kind: workerapi.KindInternal}, "unexpected failure"
	}
	body := workerapi.ErrorBody{Phase: string(failure.Phase), Kind: string(failure.Kind), JobID: failure.JobID}
	switch failure.Kind {
	case worker.KindTimeout:
		return http.StatusGatewayTimeout, body, "timeout"
	case worker.KindCanceled:
		return statusClientClosedRequest, body, ""
	case worker.KindResource:
		return http.StatusInternalServerError, body, "resource exhaustion"
	case worker.KindPagesExceeded, worker.KindPDFTooLarge:
		return http.StatusRequestEntityTooLarge, body, ""
	}
	return http.StatusInternalServerError, body, ""
}

// logFailure logs phase, kind, job and duration, and the first TeX error
// line for TeX failures. Document content and metadata values are not in it.
func (s *Server) logFailure(body workerapi.ErrorBody, d time.Duration, err error) {
	attrs := []any{"job", body.JobID, "phase", body.Phase, "kind", body.Kind, "duration_ms", d.Milliseconds()}
	var failure *worker.Error
	if errors.As(err, &failure) && failure.FirstTeXError != "" {
		attrs = append(attrs, "tex_error", failure.FirstTeXError)
	}
	if body.Kind == workerapi.KindInternal {
		attrs = append(attrs, "error", err.Error())
	}
	s.cfg.Logger.Warn("render failed", attrs...)
}

func writeError(w http.ResponseWriter, status int, body workerapi.ErrorBody) {
	w.Header().Set("Content-Type", workerapi.ContentTypeJSON)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
