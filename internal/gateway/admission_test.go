package gateway

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/taskmedia/outprint/internal/admission"
	"github.com/taskmedia/outprint/internal/bundle"
	"github.com/taskmedia/outprint/internal/workerapi"
)

const (
	cookieA = "accessToken=session-of-ada"
	cookieB = "accessToken=session-of-ben"
	cookieC = "accessToken=session-of-cleo"
)

func randomCanary(t *testing.T) string {
	t.Helper()
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return "CANARY-" + hex.EncodeToString(b)
}

func newController(t *testing.T, slots, queue, perUser int) *admission.Controller {
	t.Helper()
	c, err := admission.New(admission.Config{
		Slots: slots, QueueDepth: queue, PerUser: perUser, MaxWait: 5 * time.Second, RenderTimeout: 60 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// gatedWorker makes the fake Worker hold every request until released.
type gatedWorker struct {
	entered chan struct{}
	gate    chan struct{}
	once    sync.Once
}

func gate(w *fakeWorker) *gatedWorker {
	g := &gatedWorker{entered: make(chan struct{}, 16), gate: make(chan struct{})}
	w.mu.Lock()
	w.hook = func() {
		g.entered <- struct{}{}
		<-g.gate
	}
	w.mu.Unlock()
	return g
}

func (g *gatedWorker) open() { g.once.Do(func() { close(g.gate) }) }

func (g *gatedWorker) waitEntered(t *testing.T) {
	t.Helper()
	select {
	case <-g.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("the Worker was never called")
	}
}

type answer struct {
	resp *http.Response
	body []byte
	err  error
}

// getAsync sends a render request and delivers the answer on the channel.
func (e *env) getAsync(ctx context.Context, id, cookie string, extra func(*http.Request)) <-chan answer {
	ch := make(chan answer, 1)
	go func() {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, e.gw.URL+"/render/"+id, nil)
		req.Header.Set("Cookie", cookie)
		if extra != nil {
			extra(req)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			ch <- answer{err: err}
			return
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		ch <- answer{resp: resp, body: body}
	}()
	return ch
}

func await(t *testing.T, ch <-chan answer) answer {
	t.Helper()
	select {
	case a := <-ch:
		if a.err != nil {
			t.Fatal(a.err)
		}
		return a
	case <-time.After(5 * time.Second):
		t.Fatal("no answer")
		return answer{}
	}
}

func waitUntil(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(time.Millisecond)
	}
}

type errorList struct {
	Errors []struct {
		Source, Code, Message string
	} `json:"errors"`
}

func decodeList(t *testing.T, body []byte) errorList {
	t.Helper()
	var l errorList
	if err := json.Unmarshal(body, &l); err != nil || len(l.Errors) != 1 {
		t.Fatalf("body is not a one-entry error list: %s (%v)", body, err)
	}
	return l
}

func (e *env) assertRejected(t *testing.T, a answer, code string) {
	t.Helper()
	if a.resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("status %d, want 429: %s", a.resp.StatusCode, a.body)
	}
	secs, err := strconv.Atoi(a.resp.Header.Get("Retry-After"))
	if err != nil || secs < 1 {
		t.Errorf("Retry-After = %q, want whole seconds >= 1", a.resp.Header.Get("Retry-After"))
	}
	l := decodeList(t, a.body)
	if l.Errors[0].Code != code || l.Errors[0].Source != "service" || !strings.Contains(l.Errors[0].Message, "busy, retry later") {
		t.Errorf("error = %+v, want code %s", l.Errors[0], code)
	}
	if got := a.resp.Header.Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q", got)
	}
	e.assertNoLeak(t, a.resp, a.body)
}

func TestFullQueueIs429WithRetryAfterAndQueuedRequestProceeds(t *testing.T) {
	ctrl := newController(t, 1, 1, 1)
	e := newEnvWithAdmission(t, true, ctrl)
	e.outline.add("d", 200, "T", docText)
	for cookie, id := range map[string]string{cookieA: "ada", cookieB: "ben", cookieC: "cleo"} {
		e.outline.setUser(cookie, id)
	}
	g := gate(e.worker)
	defer g.open()

	first := e.getAsync(context.Background(), "d", cookieA, nil)
	g.waitEntered(t)
	second := e.getAsync(context.Background(), "d", cookieB, nil)
	waitUntil(t, "second request to queue", func() bool { return ctrl.Stats().Queued == 1 })

	third := await(t, e.getAsync(context.Background(), "d", cookieC, nil))
	e.assertRejected(t, third, "busy")

	g.open()
	for name, ch := range map[string]<-chan answer{"first": first, "second": second} {
		if a := await(t, ch); a.resp.StatusCode != 200 || string(a.body) != fakePDF {
			t.Errorf("%s: status %d body %q", name, a.resp.StatusCode, a.body)
		}
	}
	if n := len(e.worker.bodies); n != 2 {
		t.Errorf("the Worker rendered %d times, want 2 (the rejected one never reaches it)", n)
	}
	waitUntil(t, "controller at rest", func() bool { return ctrl.Stats() == admission.Stats{} })
}

func TestQueuedRequestGivesUpAfterMaxWait(t *testing.T) {
	ctrl, err := admission.New(admission.Config{Slots: 1, QueueDepth: 1, PerUser: 1, MaxWait: 30 * time.Millisecond, RenderTimeout: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	e := newEnvWithAdmission(t, true, ctrl)
	e.outline.add("d", 200, "T", docText)
	e.outline.setUser(cookieA, "ada")
	e.outline.setUser(cookieB, "ben")
	g := gate(e.worker)
	defer g.open()

	first := e.getAsync(context.Background(), "d", cookieA, nil)
	g.waitEntered(t)
	e.assertRejected(t, await(t, e.getAsync(context.Background(), "d", cookieB, nil)), "busy")
	g.open()
	await(t, first)
}

func TestPerUserLimitBlocksTheUserNotOthers(t *testing.T) {
	ctrl := newController(t, 1, 4, 1)
	e := newEnvWithAdmission(t, true, ctrl)
	e.outline.add("d", 200, "T", docText)
	e.outline.setUser(cookieA, "ada")
	e.outline.setUser(cookieB, "ben")
	g := gate(e.worker)
	defer g.open()

	first := e.getAsync(context.Background(), "d", cookieA, nil)
	g.waitEntered(t)

	e.assertRejected(t, await(t, e.getAsync(context.Background(), "d", cookieA, nil)), "user_limit")

	other := e.getAsync(context.Background(), "d", cookieB, nil)
	waitUntil(t, "user B to queue behind user A", func() bool { return ctrl.Stats().Queued == 1 })

	g.open()
	if a := await(t, first); a.resp.StatusCode != 200 {
		t.Errorf("first: %d", a.resp.StatusCode)
	}
	if a := await(t, other); a.resp.StatusCode != 200 {
		t.Errorf("user B: %d %s", a.resp.StatusCode, a.body)
	}
	// Once A is done, A can render again.
	if resp, body := e.get(t, "d", cookieA); resp.StatusCode != 200 {
		t.Errorf("after release: %d %s", resp.StatusCode, body)
	}
}

func TestPerUserLimitIsKeyedByTheOutlineConfirmedUser(t *testing.T) {
	spoofs := map[string]func(*http.Request){
		"x-user-id header":       func(r *http.Request) { r.Header.Set("X-User-Id", "someone-else") },
		"forwarded-user header":  func(r *http.Request) { r.Header.Set("X-Forwarded-User", "someone-else") },
		"forwarded-for header":   func(r *http.Request) { r.Header.Set("X-Forwarded-For", "203.0.113.77") },
		"user query":             func(r *http.Request) { r.URL.RawQuery = "user=someone-else&userId=someone-else" },
		"authorization header":   func(r *http.Request) { r.Header.Set("Authorization", "Bearer someone-else") },
		"x-real-ip header":       func(r *http.Request) { r.Header.Set("X-Real-Ip", "203.0.113.78") },
		"outline user id header": func(r *http.Request) { r.Header.Set("X-Outline-User-Id", "someone-else") },
		"user id in cookie":      nil,
	}
	for name, spoof := range spoofs {
		t.Run(name, func(t *testing.T) {
			ctrl := newController(t, 1, 4, 1)
			e := newEnvWithAdmission(t, true, ctrl)
			e.outline.add("d", 200, "T", docText)
			e.outline.setUser(cookieA, "ada")
			spoofCookie := cookieA
			if spoof == nil {
				// Another cookie string, the same Outline user: a user id
				// planted in the cookie must not make a second identity.
				spoofCookie = cookieA + "; userId=someone-else"
				e.outline.setUser(spoofCookie, "ada")
			}
			g := gate(e.worker)
			defer g.open()

			first := e.getAsync(context.Background(), "d", cookieA, nil)
			g.waitEntered(t)
			e.assertRejected(t, await(t, e.getAsync(context.Background(), "d", spoofCookie, spoof)), "user_limit")
			g.open()
			await(t, first)
		})
	}
}

func TestSpoofedIdentityDoesNotJoinAnotherUsersLimit(t *testing.T) {
	// Two different Outline users that send the same headers are two users.
	ctrl := newController(t, 1, 4, 1)
	e := newEnvWithAdmission(t, true, ctrl)
	e.outline.add("d", 200, "T", docText)
	e.outline.setUser(cookieA, "ada")
	e.outline.setUser(cookieB, "ben")
	g := gate(e.worker)
	defer g.open()
	same := func(r *http.Request) {
		r.Header.Set("X-User-Id", "shared")
		r.Header.Set("X-Forwarded-For", "203.0.113.5")
	}

	first := e.getAsync(context.Background(), "d", cookieA, same)
	g.waitEntered(t)
	second := e.getAsync(context.Background(), "d", cookieB, same)
	waitUntil(t, "second user to queue", func() bool { return ctrl.Stats().Queued == 1 })
	g.open()
	await(t, first)
	if a := await(t, second); a.resp.StatusCode != 200 {
		t.Errorf("second user: %d %s", a.resp.StatusCode, a.body)
	}
}

func TestUnknownUserFailsClosed(t *testing.T) {
	tests := []struct {
		name   string
		status int
		want   int
	}{
		{"outline refuses the session", 401, 401},
		{"outline forbids", 403, 403},
		{"outline fails", 500, 500},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newEnv(t, true)
			e.outline.add("d", 200, "T", docText)
			e.outline.authStatus = tt.status
			resp, body := e.get(t, "d", cookieA)
			if resp.StatusCode != tt.want {
				t.Errorf("status %d, want %d: %s", resp.StatusCode, tt.want, body)
			}
			if len(e.worker.bodies) != 0 {
				t.Error("the Worker was called without a confirmed user")
			}
			e.assertNoLeak(t, resp, body)
		})
	}
}

func TestSlotAndUserAreReleasedAfterWorkerErrors(t *testing.T) {
	ctrl := newController(t, 1, 0, 1)
	e := newEnvWithAdmission(t, true, ctrl)
	e.outline.add("d", 200, "T", docText)
	e.worker.status = 500
	e.worker.errBody = workerapi.ErrorBody{Phase: "tex", Kind: "tex_error", JobID: "j1"}
	if resp, _ := e.get(t, "d", cookieA); resp.StatusCode != 500 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if got := ctrl.Stats(); got != (admission.Stats{}) {
		t.Fatalf("stats after a failed render = %+v", got)
	}
	e.worker.mu.Lock()
	e.worker.status = 200
	e.worker.mu.Unlock()
	if resp, body := e.get(t, "d", cookieA); resp.StatusCode != 200 {
		t.Errorf("next render: %d %s", resp.StatusCode, body)
	}
}

func TestInvalidDocumentsHoldNoSlotAndNoWorkerTime(t *testing.T) {
	ctrl := newController(t, 1, 0, 1)
	e := newEnvWithAdmission(t, true, ctrl)
	e.outline.add("bad", 200, "T", "```yaml\nsubjct: oops\n```\nBody\n")
	g := gate(e.worker)
	defer g.open()

	first := e.getAsync(context.Background(), "bad", cookieA, nil)
	if a := await(t, first); a.resp.StatusCode != 422 {
		t.Fatalf("status %d", a.resp.StatusCode)
	}
	if got := ctrl.Stats(); got != (admission.Stats{}) {
		t.Errorf("stats = %+v", got)
	}
}

func TestSlotIsReleasedWhenTheClientDisconnects(t *testing.T) {
	ctrl := newController(t, 1, 0, 1)
	e := newEnvWithAdmission(t, true, ctrl)
	e.outline.add("d", 200, "T", docText)
	g := gate(e.worker)
	defer g.open()

	ctx, cancel := context.WithCancel(context.Background())
	pending := e.getAsync(ctx, "d", cookieA, nil)
	g.waitEntered(t)
	if ctrl.Stats().Running != 1 {
		t.Fatalf("stats = %+v", ctrl.Stats())
	}
	cancel()
	if a := <-pending; a.err == nil {
		t.Fatal("expected the client call to fail")
	}
	waitUntil(t, "slot and user to be released", func() bool { return ctrl.Stats() == admission.Stats{} })
}

func TestQueuedRequestLeavesTheQueueWhenTheClientDisconnects(t *testing.T) {
	ctrl := newController(t, 1, 2, 1)
	e := newEnvWithAdmission(t, true, ctrl)
	e.outline.add("d", 200, "T", docText)
	e.outline.setUser(cookieA, "ada")
	e.outline.setUser(cookieB, "ben")
	g := gate(e.worker)
	defer g.open()

	first := e.getAsync(context.Background(), "d", cookieA, nil)
	g.waitEntered(t)
	ctx, cancel := context.WithCancel(context.Background())
	queued := e.getAsync(ctx, "d", cookieB, nil)
	waitUntil(t, "request to queue", func() bool { return ctrl.Stats().Queued == 1 })
	cancel()
	<-queued
	waitUntil(t, "queue to empty", func() bool { return ctrl.Stats().Queued == 0 && ctrl.Stats().Users == 1 })
	g.open()
	await(t, first)
	waitUntil(t, "controller at rest", func() bool { return ctrl.Stats() == admission.Stats{} })
}

type panicWorker struct{}

func (panicWorker) Render(context.Context, bundle.Bundle) ([]byte, error) { panic("worker client bug") }

func TestSlotIsReleasedOnPanic(t *testing.T) {
	ctrl := newController(t, 1, 0, 1)
	e := newEnvFull(t, true, ctrl, panicWorker{})
	e.outline.add("d", 200, "T", docText)
	resp, body := e.get(t, "d", cookieA)
	if resp.StatusCode != 500 {
		t.Fatalf("status %d: %s", resp.StatusCode, body)
	}
	if got := ctrl.Stats(); got != (admission.Stats{}) {
		t.Errorf("stats after a panic = %+v", got)
	}
}

func TestStatusEndpointNeverTakesASlotOrCountsAgainstTheUser(t *testing.T) {
	ctrl := newController(t, 1, 0, 1)
	e := newEnvWithAdmission(t, true, ctrl)
	e.outline.add("d", 200, "T", docText)
	g := gate(e.worker)
	defer g.open()
	first := e.getAsync(context.Background(), "d", cookieA, nil)
	g.waitEntered(t)
	before := ctrl.Stats()
	authCalls := e.outline.calls("/api/auth.info")

	for range 3 {
		resp, body := e.getPath(t, "/status/d", cookieA)
		if resp.StatusCode != 200 || !strings.Contains(string(body), `"available":true`) {
			t.Fatalf("status endpoint: %d %s", resp.StatusCode, body)
		}
	}
	if ctrl.Stats() != before {
		t.Errorf("stats changed from %+v to %+v", before, ctrl.Stats())
	}
	if e.outline.calls("/api/auth.info") != authCalls {
		t.Error("the status endpoint asked Outline for the user")
	}
	g.open()
	await(t, first)
}

func TestTwoIdenticalRendersHitTheWorkerTwiceAndAreNotCached(t *testing.T) {
	e := newEnv(t, true)
	e.outline.add("d", 200, "T", docText)
	for i := range 2 {
		resp, body := e.get(t, "d", cookieA)
		if resp.StatusCode != 200 || string(body) != fakePDF {
			t.Fatalf("render %d: %d %s", i, resp.StatusCode, body)
		}
		if got := resp.Header.Get("Cache-Control"); got != "no-store" {
			t.Errorf("render %d: Cache-Control = %q", i, got)
		}
	}
	if n := len(e.worker.bodies); n != 2 {
		t.Errorf("the Worker rendered %d times, want 2: a PDF must never be served from a cache", n)
	}
	if n := e.outline.calls("/api/documents.info"); n != 2 {
		t.Errorf("Outline was asked for the Document %d times, want 2", n)
	}
}

// failingWorker returns an error whose message carries a canary.
type failingWorker struct{ err error }

func (w failingWorker) Render(context.Context, bundle.Bundle) ([]byte, error) { return nil, w.err }

func TestNoTexLogOrCanaryInAnyErrorResponseOrLog(t *testing.T) {
	canary := randomCanary(t)
	texLog := "! Undefined control sequence. l.42 \\" + canary
	rawWorker := func(status int, contentType, body string) func(*env) {
		return func(e *env) {
			e.worker.srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", contentType)
				w.WriteHeader(status)
				io.WriteString(w, body)
			})
		}
	}
	jsonBody := func(m map[string]string) string { b, _ := json.Marshal(m); return string(b) }
	tests := []struct {
		name  string
		stub  Worker
		setup func(*env)
		want  int
	}{
		{"worker json with extra log fields", nil, rawWorker(500, "application/json",
			jsonBody(map[string]string{"phase": "tex", "kind": "tex_error", "job_id": "j1", "log": texLog, "message": texLog})), 500},
		{"worker plain text log", nil, rawWorker(500, "text/plain", texLog), 500},
		{"worker canary as kind", nil, rawWorker(500, "application/json",
			jsonBody(map[string]string{"phase": canary, "kind": canary, "job_id": canary + "/x"})), 500},
		{"worker canary in busy answer", nil, rawWorker(503, "application/json",
			jsonBody(map[string]string{"phase": "queue", "kind": "busy", "message": texLog})), 429},
		{"worker timeout with log", nil, rawWorker(504, "application/json",
			jsonBody(map[string]string{"phase": "timeout", "kind": "timeout", "message": texLog})), 504},
		{"worker too large with log", nil, rawWorker(413, "application/json",
			jsonBody(map[string]string{"phase": "output", "kind": "pages_exceeded", "message": texLog})), 413},
		{"client error carrying a log", failingWorker{errors.New(texLog)}, nil, 500},
		{"wrapped client error carrying a log", failingWorker{errors.Join(workerapi.ErrUnreachable, errors.New(texLog))}, nil, 500},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newEnvFull(t, true, nil, tt.stub)
			e.outline.add("d", 200, "T", docText)
			if tt.setup != nil {
				tt.setup(e)
			}
			resp, body := e.get(t, "d", cookieA)
			if resp.StatusCode != tt.want {
				t.Fatalf("status %d, want %d: %s", resp.StatusCode, tt.want, body)
			}
			for where, text := range map[string]string{"body": string(body), "logs": e.logs.String(), "headers": fmtHeaders(resp.Header)} {
				if strings.Contains(text, canary) || strings.Contains(text, "Undefined control sequence") {
					t.Errorf("%s leaks worker text:\n%s", where, text)
				}
			}
			e.assertNoLeak(t, resp, body)
		})
	}
}

func TestAdmissionRejectionsLeakNothing(t *testing.T) {
	canary := randomCanary(t)
	ctrl := newController(t, 1, 0, 1)
	e := newEnvWithAdmission(t, true, ctrl)
	e.outline.add("d", 200, "T", "Hello "+canary+"\n")
	e.outline.setUser(cookieA, "ada")
	e.outline.setUser(cookieB, "ben")
	g := gate(e.worker)
	defer g.open()
	first := e.getAsync(context.Background(), "d", cookieA, nil)
	g.waitEntered(t)

	for name, cookie := range map[string]string{"queue full": cookieB, "user limit": cookieA} {
		a := await(t, e.getAsync(context.Background(), "d", cookie, nil))
		if a.resp.StatusCode != 429 {
			t.Fatalf("%s: status %d", name, a.resp.StatusCode)
		}
		for where, text := range map[string]string{"body": string(a.body), "logs": e.logs.String(), "headers": fmtHeaders(a.resp.Header)} {
			if strings.Contains(text, canary) || strings.Contains(text, "ada") || strings.Contains(text, "ben") {
				t.Errorf("%s: %s leaks:\n%s", name, where, text)
			}
		}
	}
	g.open()
	await(t, first)
}

func TestLimitsMapToClearStatusesThroughTheGateway(t *testing.T) {
	tests := []struct {
		name   string
		doc    string
		status int
		body   workerapi.ErrorBody
		want   int
		images bool
	}{
		{"markdown over 1 MiB", strings.Repeat("a", bundle.MaxMarkdownBytes+1), 0, workerapi.ErrorBody{}, 413, false},
		{"more than 500 pages", docText, 413, workerapi.ErrorBody{Phase: "output", Kind: "pages_exceeded", JobID: "j1"}, 413, false},
		{"pdf over 50 MiB", docText, 413, workerapi.ErrorBody{Phase: "output", Kind: "pdf_too_large", JobID: "j2"}, 413, false},
		{"resource limit", docText, 500, workerapi.ErrorBody{Phase: "tex", Kind: "resource", JobID: "j3"}, 413, false},
		{"timeout", docText, 504, workerapi.ErrorBody{Phase: "timeout", Kind: "timeout", JobID: "j4"}, 504, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newEnv(t, true)
			e.outline.add("d", 200, "T", tt.doc)
			if tt.status != 0 {
				e.worker.status, e.worker.errBody = tt.status, tt.body
			}
			resp, body := e.get(t, "d", cookieA)
			if resp.StatusCode != tt.want {
				t.Fatalf("status %d, want %d: %s", resp.StatusCode, tt.want, body)
			}
			var m map[string]string
			if err := json.Unmarshal(body, &m); err != nil || m["message"] == "" {
				t.Errorf("body = %s, want a clear message", body)
			}
			if tt.status == 0 && (len(e.worker.bodies) != 0 || e.outline.calls("/api/collections.info") != 0) {
				t.Error("an oversized Markdown must fail before the binding, the images and the Worker")
			}
			e.assertNoLeak(t, resp, body)
		})
	}
}
