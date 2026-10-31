// Package gateway is the Gateway role. It talks to Outline with the user's
// session cookie, resolves the Binding and the Metadata Block into a job
// bundle, hands the bundle to a Worker and streams the PDF back. It never
// runs Pandoc or TeX, and the bundle it builds holds no cookie, URL or
// secret.
package gateway

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/taskmedia/outline-latex-pdf/internal/bundle"
	"github.com/taskmedia/outline-latex-pdf/internal/config"
	"github.com/taskmedia/outline-latex-pdf/internal/metadata"
	"github.com/taskmedia/outline-latex-pdf/internal/outline"
	"github.com/taskmedia/outline-latex-pdf/internal/packages"
)

const (
	isoDateLayout = "2006-01-02"
	requestIDLen  = 8
)

// documentIDRE accepts an Outline document id (UUID) or url slug, and
// nothing that could change the meaning of an API call.
var documentIDRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,99}$`)

// Outline is what the Gateway needs from the Outline client.
type Outline interface {
	Document(ctx context.Context, id string, s outline.Session) (outline.Document, error)
	Collection(ctx context.Context, id string, s outline.Session) (outline.Collection, error)
}

// Worker is what the Gateway needs from the Worker client.
type Worker interface {
	Render(ctx context.Context, b bundle.Bundle) ([]byte, error)
}

// Config wires the Gateway.
type Config struct {
	Registry *packages.Registry
	Outline  Outline
	Worker   Worker
	Logger   *slog.Logger
}

// Gateway serves the render endpoint.
type Gateway struct {
	cfg Config
}

// New returns a Gateway. Logger defaults to slog.Default().
func New(cfg Config) *Gateway {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return &Gateway{cfg: cfg}
}

// Handler serves GET /render/{documentId}, GET /status/{documentId} and
// GET /healthz.
//
// The render endpoint is a GET with the document id in the path so that a
// button can be a plain link: the browser sends the user's cookie by itself
// and downloads the PDF.
func (g *Gateway) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("GET /render/{documentId}", g.render)
	mux.HandleFunc("GET /status/{documentId}", g.status)
	return g.recoverPanic(mux)
}

type rendered struct {
	pdf   []byte
	title string
	jobID string
}

func (g *Gateway) render(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	reqID := newRequestID()
	docID := r.PathValue("documentId")

	out, f := g.process(r, docID)
	if f != nil {
		f.write(w)
		g.logFailure(reqID, docID, f, time.Since(start))
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", contentDisposition(out.title))
	w.Header().Set("Content-Length", strconv.Itoa(len(out.pdf)))
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if _, err := w.Write(out.pdf); err != nil {
		g.cfg.Logger.Warn("write PDF failed", "job", reqID)
		return
	}
	g.cfg.Logger.Info("render ok", "job", firstNonEmpty(out.jobID, reqID), "document", docID,
		"duration_ms", time.Since(start).Milliseconds(), "bytes", len(out.pdf))
}

// process is the whole request up to the PDF. The cookie leaves the Gateway
// only in Outline calls: the Document and its Collection.
func (g *Gateway) process(r *http.Request, docID string) (rendered, *failure) {
	if !documentIDRE.MatchString(docID) {
		return rendered{}, fail(http.StatusBadRequest, PhaseRequest, "bad_document_id", "invalid document id", nil)
	}
	session := outline.Session{Cookie: r.Header.Get("Cookie")}
	if session.Cookie == "" {
		return rendered{}, outlineFailure(outline.ErrUnauthorized)
	}
	doc, err := g.cfg.Outline.Document(r.Context(), docID, session)
	if err != nil {
		return rendered{}, outlineFailure(err)
	}
	if len(doc.Text) > bundle.MaxMarkdownBytes {
		return rendered{}, bundleFailure(&bundle.LimitError{What: "markdown"})
	}
	sel, f := g.selectBinding(r.Context(), doc, session)
	if f != nil {
		return rendered{}, f
	}
	b, f := g.buildBundle(doc, sel.Binding)
	if f != nil {
		return rendered{}, f
	}
	// Admission control (global slot limit, per-user limit) belongs here:
	// the Outline-confirmed user is known and no Worker time is spent yet.
	pdf, err := g.cfg.Worker.Render(r.Context(), b)
	if err != nil {
		return rendered{}, workerFailure(err)
	}
	return rendered{pdf: pdf, title: doc.Title}, nil
}

// buildBundle resolves the Metadata Block with the derived title and date
// for the selected Binding, and validates the result.
func (g *Gateway) buildBundle(doc outline.Document, binding config.Binding) (bundle.Bundle, *failure) {
	tpl, okT := g.cfg.Registry.Template(binding.Template)
	cfg, okC := g.cfg.Registry.Configuration(binding.Configuration)
	if !okT || !okC {
		return bundle.Bundle{}, fail(http.StatusInternalServerError, PhaseBinding, "missing_package", "the document could not be rendered", nil)
	}
	res, err := metadata.Resolve(metadata.Input{
		Markdown:      []byte(doc.Text),
		Title:         doc.Title,
		Modified:      modifiedDate(doc.UpdatedAt),
		Template:      tpl,
		Configuration: cfg,
	})
	if err != nil {
		return bundle.Bundle{}, metadataFailure(err)
	}
	b := bundle.Bundle{
		Markdown:      res.Body,
		Values:        res.Values,
		Template:      binding.Template,
		DocumentType:  res.DocumentType,
		Configuration: binding.Configuration,
	}
	if err := b.Validate(); err != nil {
		return bundle.Bundle{}, bundleFailure(err)
	}
	return b, nil
}

func modifiedDate(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(isoDateLayout)
}

// logFailure logs where and how a request failed, never why in the user's
// terms: no Document content, no metadata values, no cookie. The cause is an
// error of the clients, which carry no response text.
func (g *Gateway) logFailure(reqID, docID string, f *failure, d time.Duration) {
	attrs := []any{
		"job", firstNonEmpty(f.jobID, reqID), "document", docID, "phase", f.phase, "kind", f.kind,
		"status", f.status, "duration_ms", d.Milliseconds(),
	}
	level := slog.LevelInfo
	if f.status >= http.StatusInternalServerError {
		level = slog.LevelError
		if f.cause != nil {
			attrs = append(attrs, "cause", f.cause.Error())
		}
	}
	g.cfg.Logger.Log(context.Background(), level, "render failed", attrs...)
}

func (g *Gateway) recoverPanic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recover() != nil {
				g.cfg.Logger.Error("gateway panic", "path", "render")
				fail(http.StatusInternalServerError, PhaseRequest, "internal", "the document could not be rendered", nil).write(w)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func newRequestID() string {
	b := make([]byte, requestIDLen)
	if _, err := rand.Read(b); err != nil {
		return "unknown"
	}
	return hex.EncodeToString(b)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
