package workerserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/taskmedia/outline-latex-pdf/internal/bundle"
	"github.com/taskmedia/outline-latex-pdf/internal/worker"
	"github.com/taskmedia/outline-latex-pdf/internal/workerapi"
)

type fakeRenderer struct {
	mu      sync.Mutex
	got     []bundle.Bundle
	started chan struct{}
	release chan struct{}
	pdf     []byte
	err     error
	panics  bool
}

func (f *fakeRenderer) Render(_ context.Context, b bundle.Bundle) ([]byte, error) {
	f.mu.Lock()
	f.got = append(f.got, b)
	f.mu.Unlock()
	if f.started != nil {
		f.started <- struct{}{}
	}
	if f.release != nil {
		<-f.release
	}
	if f.panics {
		panic("boom with canary-content")
	}
	return f.pdf, f.err
}

func sample() bundle.Bundle {
	return bundle.Bundle{
		Markdown: []byte("Hello"), Values: map[string]any{"title": "T"},
		Template: "neutral", DocumentType: "letter", Configuration: "example",
	}
}

type harness struct {
	srv      *httptest.Server
	logs     *bytes.Buffer
	restarts chan string
}

func newHarness(t *testing.T, token string, r Renderer) *harness {
	t.Helper()
	h := &harness{logs: &bytes.Buffer{}, restarts: make(chan string, 4)}
	log := slog.New(slog.NewTextHandler(h.logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	s := New(Config{Token: token, Logger: log, OnRestart: func(reason string) { h.restarts <- reason }}, r)
	h.srv = httptest.NewServer(s.Handler())
	t.Cleanup(h.srv.Close)
	return h
}

func (h *harness) post(t *testing.T, body io.Reader, auth string) (*http.Response, []byte) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, h.srv.URL+workerapi.RenderPath, body)
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return resp, data
}

func encoded(t *testing.T, b bundle.Bundle) io.Reader {
	t.Helper()
	data, err := b.Encode()
	if err != nil {
		t.Fatal(err)
	}
	return bytes.NewReader(data)
}

func errorBody(t *testing.T, data []byte) workerapi.ErrorBody {
	t.Helper()
	var e workerapi.ErrorBody
	if err := json.Unmarshal(data, &e); err != nil {
		t.Fatalf("not an error body: %q", data)
	}
	return e
}

func TestRenderReturnsPDF(t *testing.T) {
	f := &fakeRenderer{pdf: []byte("%PDF-fake")}
	h := newHarness(t, "", f)
	resp, data := h.post(t, encoded(t, sample()), "")
	if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "application/pdf" || string(data) != "%PDF-fake" {
		t.Fatalf("status %d type %q body %q", resp.StatusCode, resp.Header.Get("Content-Type"), data)
	}
	if len(f.got) != 1 || f.got[0].Template != "neutral" || string(f.got[0].Markdown) != "Hello" {
		t.Errorf("renderer saw %+v", f.got)
	}
}

func TestHealthzNeedsNoToken(t *testing.T) {
	h := newHarness(t, "s3cret", &fakeRenderer{})
	resp, err := http.Get(h.srv.URL + workerapi.HealthPath)
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("healthz = %v, %v", resp, err)
	}
	resp.Body.Close()
}

func TestBearerToken(t *testing.T) {
	f := &fakeRenderer{pdf: []byte("pdf")}
	h := newHarness(t, "s3cret", f)
	for _, auth := range []string{"", "Bearer wrong", "s3cret", "Basic s3cret", "Bearer "} {
		resp, data := h.post(t, encoded(t, sample()), auth)
		if resp.StatusCode != 401 || errorBody(t, data).Kind != workerapi.KindUnauthorized {
			t.Errorf("auth %q: status %d body %s", auth, resp.StatusCode, data)
		}
	}
	if len(f.got) != 0 {
		t.Error("renderer ran without a valid token")
	}
	if resp, _ := h.post(t, encoded(t, sample()), "Bearer s3cret"); resp.StatusCode != 200 {
		t.Errorf("valid token: status %d", resp.StatusCode)
	}
}

func TestOneJobAtATime(t *testing.T) {
	f := &fakeRenderer{pdf: []byte("pdf"), started: make(chan struct{}, 2), release: make(chan struct{})}
	h := newHarness(t, "", f)

	first := make(chan int, 1)
	go func() {
		resp, _ := h.post(t, encoded(t, sample()), "")
		first <- resp.StatusCode
	}()
	<-f.started

	resp, data := h.post(t, encoded(t, sample()), "")
	if resp.StatusCode != http.StatusServiceUnavailable || errorBody(t, data).Kind != workerapi.KindBusy {
		t.Fatalf("second job: status %d body %s", resp.StatusCode, data)
	}
	close(f.release)
	if status := <-first; status != 200 {
		t.Errorf("first job status %d", status)
	}
	if resp, _ := h.post(t, encoded(t, sample()), ""); resp.StatusCode != 200 {
		t.Errorf("third job status %d", resp.StatusCode)
	}
}

func TestInvalidBundles(t *testing.T) {
	h := newHarness(t, "", &fakeRenderer{pdf: []byte("pdf")})
	bad := sample()
	bad.Template = "../x"
	badData, _ := json.Marshal(bad)
	tests := map[string]struct {
		body   io.Reader
		status int
		kind   string
	}{
		"garbage":      {strings.NewReader("nope"), 400, workerapi.KindInvalidBundle},
		"bad name":     {bytes.NewReader(badData), 400, workerapi.KindInvalidBundle},
		"unknown key":  {strings.NewReader(`{"cookie":"x"}`), 400, workerapi.KindInvalidBundle},
		"markdown cap": {strings.NewReader(`{"markdown":"` + strings.Repeat("a", bundle.MaxMarkdownBytes+1) + `","template":"neutral","documentType":"letter","configuration":"example"}`), 413, workerapi.KindTooLarge},
		"body cap":     {strings.NewReader(strings.Repeat(" ", bundle.MaxEncodedBytes+1)), 413, workerapi.KindTooLarge},
	}
	for name, tt := range tests {
		resp, data := h.post(t, tt.body, "")
		if resp.StatusCode != tt.status || errorBody(t, data).Kind != tt.kind {
			t.Errorf("%s: status %d body %s", name, resp.StatusCode, data)
		}
	}
}

func TestTimeoutAnswers504ThenRestartsAndStaysBusy(t *testing.T) {
	f := &fakeRenderer{err: &worker.Error{Phase: worker.PhaseTimeout, Kind: worker.KindTimeout, JobID: "job1"}}
	h := newHarness(t, "", f)
	resp, data := h.post(t, encoded(t, sample()), "")
	body := errorBody(t, data)
	if resp.StatusCode != 504 || body.Phase != "timeout" || body.Kind != "timeout" || body.JobID != "job1" {
		t.Fatalf("status %d body %s", resp.StatusCode, data)
	}
	select {
	case reason := <-h.restarts:
		if reason != "timeout" {
			t.Errorf("restart reason = %q", reason)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no restart was requested")
	}
	resp, _ = h.post(t, encoded(t, sample()), "")
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("after the restart request the Worker must stay busy, got %d", resp.StatusCode)
	}
}

func TestPanicIsContainedAndRestarts(t *testing.T) {
	h := newHarness(t, "", &fakeRenderer{panics: true})
	resp, data := h.post(t, encoded(t, sample()), "")
	if resp.StatusCode != 500 || strings.Contains(string(data), "canary-content") {
		t.Fatalf("status %d body %s", resp.StatusCode, data)
	}
	select {
	case <-h.restarts:
	case <-time.After(2 * time.Second):
		t.Fatal("no restart after a panic")
	}
}

func TestPipelineFailureKeepsWorkerAlive(t *testing.T) {
	f := &fakeRenderer{err: &worker.Error{Phase: worker.PhaseTeX, Kind: worker.KindTeXError, JobID: "job2", FirstTeXError: "! Undefined control sequence."}}
	h := newHarness(t, "", f)
	resp, data := h.post(t, encoded(t, sample()), "")
	body := errorBody(t, data)
	if resp.StatusCode != 500 || body.Phase != "tex" || body.Kind != "tex_error" || body.JobID != "job2" {
		t.Fatalf("status %d body %s", resp.StatusCode, data)
	}
	if strings.Contains(string(data), "Undefined") {
		t.Error("the TeX error line travels in the response")
	}
	if !strings.Contains(h.logs.String(), "! Undefined control sequence.") || !strings.Contains(h.logs.String(), "job2") {
		t.Errorf("log lacks the first TeX error line or the job id:\n%s", h.logs)
	}
	select {
	case r := <-h.restarts:
		t.Errorf("an ordinary TeX failure restarted the Worker: %s", r)
	case <-time.After(100 * time.Millisecond):
	}
	f.err, f.pdf = nil, []byte("pdf")
	if resp, _ := h.post(t, encoded(t, sample()), ""); resp.StatusCode != 200 {
		t.Errorf("next job status %d", resp.StatusCode)
	}
}

func TestUnknownPackageIs400(t *testing.T) {
	h := newHarness(t, "", &fakeRenderer{err: ErrUnknownPackage})
	resp, data := h.post(t, encoded(t, sample()), "")
	if resp.StatusCode != 400 || errorBody(t, data).Kind != workerapi.KindInvalidBundle {
		t.Errorf("status %d body %s", resp.StatusCode, data)
	}
}

func TestUnexpectedErrorRestarts(t *testing.T) {
	h := newHarness(t, "", &fakeRenderer{err: errors.New("disk on fire")})
	resp, _ := h.post(t, encoded(t, sample()), "")
	if resp.StatusCode != 500 {
		t.Errorf("status %d", resp.StatusCode)
	}
	select {
	case <-h.restarts:
	case <-time.After(2 * time.Second):
		t.Fatal("no restart")
	}
}

func TestOversizedPDFIs413(t *testing.T) {
	h := newHarness(t, "", &fakeRenderer{pdf: make([]byte, workerapi.MaxPDFBytes+1)})
	resp, data := h.post(t, encoded(t, sample()), "")
	if resp.StatusCode != 413 || errorBody(t, data).Phase != workerapi.PhaseOutput {
		t.Errorf("status %d body %s", resp.StatusCode, data)
	}
}

func TestClientAgainstServer(t *testing.T) {
	h := newHarness(t, "tok", &fakeRenderer{pdf: []byte("%PDF-x")})
	c, err := workerapi.NewClient(h.srv.URL, "tok", 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	pdf, err := c.Render(context.Background(), sample())
	if err != nil || string(pdf) != "%PDF-x" {
		t.Fatalf("Render = %q, %v", pdf, err)
	}
	bad, _ := workerapi.NewClient(h.srv.URL, "nope", 5*time.Second)
	_, err = bad.Render(context.Background(), sample())
	var werr *workerapi.Error
	if !errors.As(err, &werr) || werr.Status != 401 {
		t.Errorf("err = %v", err)
	}
}

func TestLimitFailuresAnswer413WithoutRestart(t *testing.T) {
	for _, kind := range []worker.Kind{worker.KindPagesExceeded, worker.KindPDFTooLarge} {
		f := &fakeRenderer{err: &worker.Error{Phase: worker.PhaseOutput, Kind: kind, JobID: "job1"}}
		h := newHarness(t, "", f)
		resp, data := h.post(t, encoded(t, sample()), "")
		body := errorBody(t, data)
		if resp.StatusCode != 413 || body.Phase != "output" || body.Kind != string(kind) || body.JobID != "job1" {
			t.Errorf("%s: status %d body %s", kind, resp.StatusCode, data)
		}
		select {
		case reason := <-h.restarts:
			t.Errorf("%s: unexpected restart %q", kind, reason)
		case <-time.After(100 * time.Millisecond):
		}
		if resp, _ = h.post(t, encoded(t, sample()), ""); resp.StatusCode == http.StatusServiceUnavailable {
			t.Errorf("%s: the Worker stays busy after a limit failure", kind)
		}
	}
}

func TestResourceFailureAnswersStructuredErrorThenRestarts(t *testing.T) {
	f := &fakeRenderer{err: &worker.Error{Phase: worker.PhaseTeX, Kind: worker.KindResource, JobID: "job1"}}
	h := newHarness(t, "", f)
	resp, data := h.post(t, encoded(t, sample()), "")
	if body := errorBody(t, data); resp.StatusCode != 500 || body.Kind != "resource" {
		t.Fatalf("status %d body %s", resp.StatusCode, data)
	}
	select {
	case reason := <-h.restarts:
		if reason != "resource exhaustion" {
			t.Errorf("restart reason = %q", reason)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no restart was requested")
	}
}
