package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/taskmedia/outprint/internal/admission"
	"github.com/taskmedia/outprint/internal/bundle"
	"github.com/taskmedia/outprint/internal/config"
	"github.com/taskmedia/outprint/internal/outline"
	"github.com/taskmedia/outprint/internal/startup"
	"github.com/taskmedia/outprint/internal/workerapi"
)

const (
	cookieCanary  = "accessToken=COOKIE-CANARY-9f3a"
	contentCanary = "CONTENT-CANARY-77c1"
	fakePDF       = "%PDF-1.7 fake pdf bytes"
	docText       = "Hello " + contentCanary + "\n"
)

type outlineDoc struct {
	status int
	doc    map[string]any
}

// fakeOutline is an in-process Outline. It records what it was sent.
type fakeOutline struct {
	srv  *httptest.Server
	mu   sync.Mutex
	docs map[string]outlineDoc
	cols map[string]outlineDoc
	seen []seenRequest
	// users maps a cookie to the user id auth.info confirms; an unmapped
	// cookie is user-1. authStatus, when set, is the status auth.info answers.
	users      map[string]string
	authStatus int
	// language is what auth.info reports as the user's language.
	language string
}

type seenRequest struct{ path, cookie string }

func newFakeOutline(t *testing.T) *fakeOutline {
	t.Helper()
	f := &fakeOutline{docs: map[string]outlineDoc{}, cols: map[string]outlineDoc{}, users: map[string]string{}}
	f.addCollection("c1", 200, "")
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.mu.Lock()
		f.seen = append(f.seen, seenRequest{r.URL.Path, r.Header.Get("Cookie")})
		table := f.docs
		if r.URL.Path == "/api/auth.info" {
			id, status := f.userFor(r.Header.Get("Cookie"))
			lang := f.language
			f.mu.Unlock()
			w.WriteHeader(status)
			if status == 200 {
				user := map[string]any{"id": id}
				if lang != "" {
					user["language"] = lang
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"user": user}})
			}
			return
		}
		if r.URL.Path == "/api/collections.info" {
			table = f.cols
		}
		d, ok := table[body["id"]]
		f.mu.Unlock()
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(d.status)
		if d.status == 200 {
			_ = json.NewEncoder(w).Encode(map[string]any{"data": d.doc})
		}
	}))
	t.Cleanup(f.srv.Close)
	return f
}

// userFor must be called with f.mu held.
func (f *fakeOutline) userFor(cookie string) (string, int) {
	status := f.authStatus
	if status == 0 {
		status = 200
	}
	if id, ok := f.users[cookie]; ok {
		return id, status
	}
	return "user-1", status
}

func (f *fakeOutline) setUser(cookie, id string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.users[cookie] = id
}

func (f *fakeOutline) add(id string, status int, title, text string) {
	f.docs[id] = outlineDoc{status: status, doc: map[string]any{
		"id": id, "title": title, "text": text, "updatedAt": "2026-03-02T23:30:00.000Z", "collectionId": "c1",
	}}
}

func (f *fakeOutline) addCollection(id string, status int, overview string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cols[id] = outlineDoc{status: status, doc: map[string]any{"id": id, "name": "Team", "description": overview}}
}

func (f *fakeOutline) calls(path string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, r := range f.seen {
		if r.path == path {
			n++
		}
	}
	return n
}

// fakeWorker implements the Worker HTTP contract and records what arrived.
type fakeWorker struct {
	srv     *httptest.Server
	mu      sync.Mutex
	bodies  [][]byte
	headers []http.Header
	status  int
	pdf     string
	errBody workerapi.ErrorBody
	// hook, when set, runs for each request before the answer.
	hook func()
}

func newFakeWorker(t *testing.T) *fakeWorker {
	t.Helper()
	f := &fakeWorker{status: 200, pdf: fakePDF}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		f.bodies = append(f.bodies, body)
		f.headers = append(f.headers, r.Header.Clone())
		status, pdf, eb, hook := f.status, f.pdf, f.errBody, f.hook
		f.mu.Unlock()
		if hook != nil {
			hook()
		}
		if status == 200 {
			w.Header().Set("Content-Type", "application/pdf")
			io.WriteString(w, pdf)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(eb)
	}))
	t.Cleanup(f.srv.Close)
	return f
}

type env struct {
	gw      *httptest.Server
	outline *fakeOutline
	worker  *fakeWorker
	logs    *bytes.Buffer
}

func newEnv(t *testing.T, defaultBinding bool) *env {
	t.Helper()
	return newEnvWithAdmission(t, defaultBinding, nil)
}

// newEnvWithAdmission is newEnv with a custom admission controller.
func newEnvWithAdmission(t *testing.T, defaultBinding bool, admit *admission.Controller) *env {
	t.Helper()
	return newEnvFull(t, defaultBinding, admit, nil)
}

// newEnvFull also lets a test replace the HTTP Worker client with a stub.
func newEnvFull(t *testing.T, defaultBinding bool, admit *admission.Controller, stub Worker) *env {
	t.Helper()
	e := &env{outline: newFakeOutline(t), worker: newFakeWorker(t), logs: &bytes.Buffer{}}
	svc := config.Service{
		TemplatesRoot: "../../templates", ConfigurationsRoot: "../../configurations",
		Bindings: []config.Binding{{Template: "neutral", Configuration: "example"}},
	}
	if defaultBinding {
		svc.DefaultBinding = &config.Binding{Template: "neutral", Configuration: "example"}
	}
	started, err := startup.FromService(svc)
	if err != nil {
		t.Fatal(err)
	}
	oc, err := outline.NewClient(e.outline.srv.URL, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	wc, err := workerapi.NewClient(e.worker.srv.URL, "", 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	var w Worker = wc
	if stub != nil {
		w = stub
	}
	log := slog.New(slog.NewTextHandler(e.logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	e.gw = httptest.NewServer(New(Config{Registry: started.Registry, Outline: oc, Worker: w, Admission: admit, Logger: log}).Handler())
	t.Cleanup(e.gw.Close)
	return e
}

func (e *env) get(t *testing.T, id, cookie string) (*http.Response, []byte) {
	t.Helper()
	return e.getPath(t, "/render/"+id, cookie)
}

func (e *env) getPath(t *testing.T, path, cookie string) (*http.Response, []byte) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, e.gw.URL+path, nil)
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp, body
}

func (e *env) assertNoLeak(t *testing.T, resp *http.Response, body []byte) {
	t.Helper()
	surfaces := map[string]string{"body": string(body), "logs": e.logs.String(), "headers": fmtHeaders(resp.Header)}
	for where, text := range surfaces {
		for _, canary := range []string{cookieCanary, "COOKIE-CANARY", contentCanary} {
			if strings.Contains(text, canary) {
				t.Errorf("%s leaks %q:\n%s", where, canary, text)
			}
		}
	}
}

func fmtHeaders(h http.Header) string {
	var b strings.Builder
	for k, v := range h {
		b.WriteString(k + ": " + strings.Join(v, ",") + "\n")
	}
	return b.String()
}

func TestHappyPath(t *testing.T) {
	e := newEnv(t, true)
	e.outline.add("doc-1", 200, "Spring plant sale", docText)

	resp, body := e.get(t, "doc-1", cookieCanary)

	if resp.StatusCode != 200 || string(body) != fakePDF {
		t.Fatalf("status %d body %q", resp.StatusCode, body)
	}
	if got := resp.Header.Get("Content-Type"); got != "application/pdf" {
		t.Errorf("Content-Type = %q", got)
	}
	if got := resp.Header.Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q", got)
	}
	_, params, err := mime.ParseMediaType(resp.Header.Get("Content-Disposition"))
	if err != nil || params["filename"] != "Spring plant sale.pdf" {
		t.Errorf("Content-Disposition = %q (%v, %v)", resp.Header.Get("Content-Disposition"), params, err)
	}

	wantPaths := []string{"/api/documents.info", "/api/collections.info", "/api/auth.info"}
	if len(e.outline.seen) != len(wantPaths) {
		t.Fatalf("outline saw %+v", e.outline.seen)
	}
	for i, seen := range e.outline.seen {
		if seen.path != wantPaths[i] || seen.cookie != cookieCanary {
			t.Errorf("outline call %d = %+v", i, seen)
		}
	}
	if len(e.worker.bodies) != 1 {
		t.Fatalf("worker got %d requests", len(e.worker.bodies))
	}
	got, err := bundle.Decode(bytes.NewReader(e.worker.bodies[0]))
	if err != nil {
		t.Fatalf("worker received an invalid bundle: %v", err)
	}
	if got.Template != "neutral" || got.Configuration != "example" || got.DocumentType != "letter" {
		t.Errorf("bundle names = %s/%s/%s", got.Template, got.DocumentType, got.Configuration)
	}
	if got.Values["title"] != "Spring plant sale" {
		t.Errorf("title = %v", got.Values["title"])
	}
	if got.Values["date"] != "2 March 2026" && got.Values["date"] != "2026-03-02" {
		t.Errorf("date = %v, want the last-modified date", got.Values["date"])
	}
	if strings.Contains(string(got.Markdown), "```yaml") || !strings.Contains(string(got.Markdown), contentCanary) {
		t.Errorf("markdown = %q", got.Markdown)
	}
}

func TestWorkerNeverSeesCookieOrOutlineURL(t *testing.T) {
	e := newEnv(t, true)
	e.outline.add("doc-1", 200, "Title", docText)
	e.get(t, "doc-1", cookieCanary)

	outlineHost := mustHost(t, e.outline.srv.URL)
	for _, body := range e.worker.bodies {
		for _, forbidden := range []string{"COOKIE-CANARY", "accessToken", outlineHost, "http://", "https://"} {
			if strings.Contains(string(body), forbidden) {
				t.Errorf("bundle on the wire contains %q:\n%s", forbidden, body)
			}
		}
	}
	for _, h := range e.worker.headers {
		for name, values := range h {
			if strings.EqualFold(name, "Cookie") || strings.Contains(strings.Join(values, ""), "COOKIE-CANARY") {
				t.Errorf("worker request header %s carries the session", name)
			}
		}
	}
}

func mustHost(t *testing.T, raw string) string {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u.Host
}

func TestOutlineErrorsAreMapped(t *testing.T) {
	tests := []struct {
		name   string
		status int
		want   int
	}{
		{"unauthorized", 401, 401}, {"forbidden", 403, 403}, {"not found", 404, 404}, {"server error", 500, 500},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newEnv(t, true)
			e.outline.add("d", tt.status, "T", docText)
			resp, body := e.get(t, "d", cookieCanary)
			if resp.StatusCode != tt.want {
				t.Errorf("status %d, want %d (%s)", resp.StatusCode, tt.want, body)
			}
			if len(e.worker.bodies) != 0 {
				t.Error("the Worker was called although Outline refused")
			}
			e.assertNoLeak(t, resp, body)
		})
	}
}

func TestUnknownDocumentIs404(t *testing.T) {
	e := newEnv(t, true)
	resp, _ := e.get(t, "missing", cookieCanary)
	if resp.StatusCode != 404 {
		t.Errorf("status %d", resp.StatusCode)
	}
}

func TestMissingCookieIs401WithoutCallingOutline(t *testing.T) {
	e := newEnv(t, true)
	resp, _ := e.get(t, "doc-1", "")
	if resp.StatusCode != 401 || len(e.outline.seen) != 0 {
		t.Errorf("status %d, outline calls %d", resp.StatusCode, len(e.outline.seen))
	}
}

func TestBadDocumentIDAndMethod(t *testing.T) {
	e := newEnv(t, true)
	for _, id := range []string{"a%20b", "..%2f..", "-x", strings.Repeat("a", 101)} {
		if resp, _ := e.get(t, id, cookieCanary); resp.StatusCode != 400 && resp.StatusCode != 404 {
			t.Errorf("id %q: status %d", id, resp.StatusCode)
		}
	}
	if len(e.outline.seen) != 0 {
		t.Error("an invalid id reached Outline")
	}
	resp, err := http.Post(e.gw.URL+"/render/doc-1", "text/plain", nil)
	if err != nil || resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("POST = %v, %v", resp, err)
	}
}

func TestMetadataErrorsAre422WithTheList(t *testing.T) {
	e := newEnv(t, true)
	e.outline.add("d", 200, "T", "```yaml\nsubjct: oops\ntype: nonsense\n```\nBody "+contentCanary+"\n")
	resp, body := e.get(t, "d", cookieCanary)
	if resp.StatusCode != 422 {
		t.Fatalf("status %d: %s", resp.StatusCode, body)
	}
	var list struct {
		Errors []struct{ Field, Source, Code, Message string } `json:"errors"`
	}
	if err := json.Unmarshal(body, &list); err != nil || len(list.Errors) == 0 {
		t.Fatalf("body = %s (%v)", body, err)
	}
	codes := map[string]bool{}
	for _, e := range list.Errors {
		codes[e.Code] = true
	}
	if !codes["unknown"] {
		t.Errorf("errors = %+v, want an unknown-key error", list.Errors)
	}
	if len(e.worker.bodies) != 0 {
		t.Error("the Worker was called although the metadata is invalid")
	}
}

func TestNoDefaultBindingIs422(t *testing.T) {
	e := newEnv(t, false)
	e.outline.add("d", 200, "T", docText)
	resp, body := e.get(t, "d", cookieCanary)
	if resp.StatusCode != 422 || !strings.Contains(string(body), "no_binding") || !strings.Contains(string(body), "no binding") {
		t.Errorf("status %d body %s", resp.StatusCode, body)
	}
	if len(e.worker.bodies) != 0 {
		t.Error("the Worker was called without a Binding")
	}
}

func TestTooLargeDocumentIs413(t *testing.T) {
	e := newEnv(t, true)
	e.outline.add("d", 200, "T", strings.Repeat("a", bundle.MaxMarkdownBytes+1))
	resp, _ := e.get(t, "d", cookieCanary)
	if resp.StatusCode != 413 || len(e.worker.bodies) != 0 {
		t.Errorf("status %d, worker calls %d", resp.StatusCode, len(e.worker.bodies))
	}
}

func TestWorkerFailuresAreMapped(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		body       workerapi.ErrorBody
		want       int
		retryAfter bool
	}{
		{"busy", 503, workerapi.ErrorBody{Phase: "queue", Kind: "busy"}, 429, true},
		{"timeout", 504, workerapi.ErrorBody{Phase: "timeout", Kind: "timeout", JobID: "j1"}, 504, false},
		{"too large", 413, workerapi.ErrorBody{Phase: "bundle", Kind: "too_large"}, 413, false},
		{"too many pages", 413, workerapi.ErrorBody{Phase: "output", Kind: "pages_exceeded", JobID: "j3"}, 413, false},
		{"pdf too large", 413, workerapi.ErrorBody{Phase: "output", Kind: "pdf_too_large", JobID: "j4"}, 413, false},
		{"resource", 500, workerapi.ErrorBody{Phase: "tex", Kind: "resource", JobID: "j5"}, 413, false},
		{"tex error", 500, workerapi.ErrorBody{Phase: "tex", Kind: "tex_error", JobID: "j2"}, 500, false},
		{"unauthorized", 401, workerapi.ErrorBody{Phase: "auth", Kind: "unauthorized"}, 500, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newEnv(t, true)
			e.outline.add("d", 200, "T", docText)
			e.worker.status, e.worker.errBody = tt.status, tt.body
			resp, body := e.get(t, "d", cookieCanary)
			if resp.StatusCode != tt.want {
				t.Fatalf("status %d, want %d (%s)", resp.StatusCode, tt.want, body)
			}
			if (resp.Header.Get("Retry-After") != "") != tt.retryAfter {
				t.Errorf("Retry-After = %q", resp.Header.Get("Retry-After"))
			}
			var m map[string]string
			if err := json.Unmarshal(body, &m); err != nil || m["message"] == "" {
				t.Errorf("body = %s", body)
			}
			if tt.body.JobID != "" && m["job_id"] != tt.body.JobID {
				t.Errorf("job_id = %q", m["job_id"])
			}
			e.assertNoLeak(t, resp, body)
		})
	}
}

func TestInternalFailureBodyHoldsOnlyJobPhaseAndDocsLink(t *testing.T) {
	e := newEnv(t, true)
	e.outline.add("d", 200, "T", docText)
	e.worker.status = 500
	e.worker.errBody = workerapi.ErrorBody{Phase: "tex", Kind: "tex_error", JobID: "j9"}
	resp, body := e.get(t, "d", cookieCanary)
	var m map[string]any
	_ = json.Unmarshal(body, &m)
	if resp.StatusCode != 500 || len(m) != 4 || m["docs_url"] == "" || m["phase"] != "tex" || m["job_id"] != "j9" {
		t.Errorf("status %d body %s", resp.StatusCode, body)
	}
	for _, want := range []string{"j9", "tex", "tex_error", "status=500", "duration_ms"} {
		if !strings.Contains(e.logs.String(), want) {
			t.Errorf("log lacks %q:\n%s", want, e.logs)
		}
	}
	e.assertNoLeak(t, resp, body)
}

func TestUnreachableWorkerIs500(t *testing.T) {
	e := newEnv(t, true)
	e.outline.add("d", 200, "T", docText)
	e.worker.srv.Close()
	resp, body := e.get(t, "d", cookieCanary)
	if resp.StatusCode != 500 {
		t.Errorf("status %d", resp.StatusCode)
	}
	e.assertNoLeak(t, resp, body)
	if strings.Contains(string(body), e.worker.srv.URL) {
		t.Error("the Worker URL leaks to the client")
	}
}

func TestLogsOnSuccessHaveNoContent(t *testing.T) {
	e := newEnv(t, true)
	e.outline.add("d", 200, "Secret Title "+contentCanary, docText)
	resp, body := e.get(t, "d", cookieCanary)
	if resp.StatusCode != 200 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	_ = body
	for _, canary := range []string{"COOKIE-CANARY", contentCanary, "Secret Title"} {
		if strings.Contains(e.logs.String(), canary) {
			t.Errorf("log leaks %q:\n%s", canary, e.logs)
		}
	}
	if !strings.Contains(e.logs.String(), "render ok") {
		t.Errorf("no success log:\n%s", e.logs)
	}
}

func TestHealthz(t *testing.T) {
	e := newEnv(t, true)
	resp, err := http.Get(e.gw.URL + "/healthz")
	if err != nil || resp.StatusCode != 200 {
		t.Errorf("healthz = %v, %v", resp, err)
	}
}

func TestPanicIs500WithoutDetail(t *testing.T) {
	e := newEnv(t, true)
	g := New(Config{Registry: nil, Outline: panicOutline{}, Worker: nil, Logger: slog.New(slog.NewTextHandler(e.logs, nil))})
	srv := httptest.NewServer(g.Handler())
	defer srv.Close()
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/render/x", nil)
	req.Header.Set("Cookie", cookieCanary)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 500 || strings.Contains(string(body), contentCanary) {
		t.Errorf("status %d body %s", resp.StatusCode, body)
	}
	e.assertNoLeak(t, resp, body)
}

type panicOutline struct{}

func (panicOutline) Collection(_ context.Context, _ string, _ outline.Session) (outline.Collection, error) {
	panic(contentCanary)
}

func (panicOutline) Document(_ context.Context, _ string, _ outline.Session) (outline.Document, error) {
	panic(contentCanary)
}

func (panicOutline) Image(_ context.Context, _ string, _ outline.Session) ([]byte, error) {
	panic(contentCanary)
}

func (panicOutline) UserID(_ context.Context, _ outline.Session) (string, error) {
	panic(contentCanary)
}

func (panicOutline) UserLanguage(_ context.Context, _ outline.Session) (string, error) {
	panic(contentCanary)
}

func (panicOutline) BaseURL() string { return "https://outline.example.invalid" }
