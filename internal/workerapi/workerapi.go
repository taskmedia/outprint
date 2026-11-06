// Package workerapi is the HTTP contract between the Gateway and the Worker
// and the Gateway's client for it. The request body is a job bundle; the
// response is a PDF or an ErrorBody.
package workerapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/taskmedia/outprint/internal/bundle"
	"github.com/taskmedia/outprint/internal/limits"
)

const (
	RenderPath = "/render"
	HealthPath = "/healthz"
	// ReadyPath answers 200 only while the Worker can take a job, so a
	// Service or load balancer can route around a busy or restarting Worker.
	ReadyPath = "/readyz"

	// MaxPDFBytes is the largest PDF the Worker returns.
	MaxPDFBytes = limits.MaxPDFBytes
	// MaxErrorBytes caps the error body the client reads.
	MaxErrorBytes = 4 << 10

	ContentTypePDF  = "application/pdf"
	ContentTypeJSON = "application/json"
)

// Phases the Worker server adds to the pipeline's own (setup, svg, pandoc,
// filter, tex, timeout).
const (
	PhaseQueue  = "queue"
	PhaseAuth   = "auth"
	PhaseBundle = "bundle"
	PhaseOutput = "output"
)

// Kinds the Worker server adds to the pipeline's own.
const (
	KindBusy          = "busy"
	KindUnauthorized  = "unauthorized"
	KindInvalidBundle = "invalid_bundle"
	KindTooLarge      = "too_large"
	KindInternal      = "internal"
	// KindTimeout is the pipeline's timeout kind, repeated here because the
	// Gateway maps it without importing the pipeline.
	KindTimeout = "timeout"
	// The pipeline's limit kinds, repeated for the same reason.
	KindPagesExceeded = "pages_exceeded"
	KindPDFTooLarge   = "pdf_too_large"
	KindResource      = "resource"
	// KindOutputTooLarge is an SVG that converts to a PDF over its limit.
	KindOutputTooLarge = "output_too_large"
)

// ErrorBody is what the Worker answers on failure. It names the phase, the
// kind and the job, and nothing from the Document or the tools.
type ErrorBody struct {
	Phase string `json:"phase"`
	Kind  string `json:"kind"`
	JobID string `json:"job_id,omitempty"`
}

// Error is a failure reported by the Worker.
type Error struct {
	Status int
	ErrorBody
}

func (e *Error) Error() string {
	return fmt.Sprintf("worker answered %d: phase %s (%s), job %s", e.Status, e.Phase, e.Kind, e.JobID)
}

// Busy reports that the Worker is rendering another job.
func (e *Error) Busy() bool { return e.Kind == KindBusy }

// ErrUnreachable is returned when the Worker could not be reached or its
// answer made no sense.
var ErrUnreachable = errors.New("worker unreachable")

// Client sends bundles to one Worker.
type Client struct {
	renderURL string
	token     string
	http      *http.Client
}

// NewClient returns a client for the Worker at baseURL. token, if not empty,
// is sent as a bearer token. timeout bounds the whole call; it must exceed
// the Worker's own render limit.
func NewClient(baseURL, token string, timeout time.Duration) (*Client, error) {
	u, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, errors.New("worker url must be an http or https URL with a host")
	}
	return &Client{
		renderURL: u.JoinPath(RenderPath).String(),
		token:     token,
		http: &http.Client{
			Timeout:       timeout,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}, nil
}

// Render sends the bundle and returns the PDF. Failures are *Error when the
// Worker answered, else wrap ErrUnreachable.
func (c *Client) Render(ctx context.Context, b bundle.Bundle) ([]byte, error) {
	body, err := b.Encode()
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.renderURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("%w: build request", ErrUnreachable)
	}
	req.Header.Set("Content-Type", ContentTypeJSON)
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: request failed", ErrUnreachable)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		return readPDF(resp)
	}
	return nil, readError(resp)
}

func readPDF(resp *http.Response) ([]byte, error) {
	if resp.Header.Get("Content-Type") != ContentTypePDF {
		return nil, fmt.Errorf("%w: answer is not a PDF", ErrUnreachable)
	}
	pdf, err := io.ReadAll(io.LimitReader(resp.Body, MaxPDFBytes+1))
	if err != nil {
		return nil, fmt.Errorf("%w: read PDF", ErrUnreachable)
	}
	if len(pdf) > MaxPDFBytes {
		return nil, &Error{Status: http.StatusRequestEntityTooLarge, ErrorBody: ErrorBody{Phase: PhaseOutput, Kind: KindTooLarge}}
	}
	return pdf, nil
}

func readError(resp *http.Response) error {
	e := &Error{Status: resp.StatusCode}
	data, err := io.ReadAll(io.LimitReader(resp.Body, MaxErrorBytes))
	if err != nil || json.Unmarshal(data, &e.ErrorBody) != nil || e.Kind == "" {
		return fmt.Errorf("%w: unexpected status %d", ErrUnreachable, resp.StatusCode)
	}
	return e
}

// TokenMatches compares a presented bearer token with the expected one in
// constant time.
func TokenMatches(authorization, token string) bool {
	presented, ok := strings.CutPrefix(authorization, "Bearer ")
	if !ok {
		return false
	}
	a, b := sha256.Sum256([]byte(presented)), sha256.Sum256([]byte(token))
	return subtle.ConstantTimeCompare(a[:], b[:]) == 1
}
