package gateway

// Seam 1, security layer 4: the image fetch of the Gateway against a fake
// Outline, a fake file storage and a fake attacker, with a resolver and a
// dialer of the test's own. Nothing here touches a real network.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/taskmedia/outline-latex-pdf/internal/bundle"
	"github.com/taskmedia/outline-latex-pdf/internal/config"
	"github.com/taskmedia/outline-latex-pdf/internal/limits"
	"github.com/taskmedia/outline-latex-pdf/internal/metadata"
	"github.com/taskmedia/outline-latex-pdf/internal/outline"
	"github.com/taskmedia/outline-latex-pdf/internal/startup"
	"github.com/taskmedia/outline-latex-pdf/internal/workerapi"
)

const (
	storageHost  = "files.example.invalid"
	attackerHost = "attacker.example.invalid"
	storageIP    = "11.1.0.1"
	attackerIP   = "11.2.0.1"
	uuidA        = "0b6f1a52-3c41-4d2e-9a77-5f0c2e8d1b34"
	uuidB        = "7d2e9c10-84aa-4f0b-b1c3-12e5a6f70c58"
	userCookie   = "accessToken=USER-SESSION-CANARY"
)

type hit struct{ path, cookie, authorization string }

// server is an httptest server that records what it was sent.
type server struct {
	*httptest.Server
	mu   sync.Mutex
	hits []hit
	fn   func(w http.ResponseWriter, r *http.Request)
}

func newServer(t *testing.T) *server {
	t.Helper()
	s := &server{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.hits = append(s.hits, hit{r.URL.RequestURI(), r.Header.Get("Cookie"), r.Header.Get("Authorization")})
		fn := s.fn
		s.mu.Unlock()
		if fn == nil {
			http.NotFound(w, r)
			return
		}
		fn(w, r)
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *server) handle(fn func(w http.ResponseWriter, r *http.Request)) {
	s.mu.Lock()
	s.fn = fn
	s.mu.Unlock()
}

func (s *server) recorded() []hit {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]hit(nil), s.hits...)
}

func (s *server) addr() string { return strings.TrimPrefix(s.URL, "http://") }

// resolver answers from a table. A name may have a sequence of answers, one
// per lookup, the last one repeating, to play DNS rebinding.
type resolver struct {
	mu    sync.Mutex
	table map[string][][]netip.Addr
	calls map[string]int
}

func (r *resolver) set(host string, answers ...[]string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var seq [][]netip.Addr
	for _, a := range answers {
		var addrs []netip.Addr
		for _, s := range a {
			addrs = append(addrs, netip.MustParseAddr(s))
		}
		seq = append(seq, addrs)
	}
	r.table[host] = seq
}

func (r *resolver) LookupNetIP(_ context.Context, _, host string) ([]netip.Addr, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	seq, ok := r.table[host]
	if !ok {
		return nil, fmt.Errorf("no such host %s", host)
	}
	n := r.calls[host]
	r.calls[host]++
	return seq[min(n, len(seq)-1)], nil
}

// world is one Gateway with its fake neighbours.
type world struct {
	t        *testing.T
	outline  *server
	storage  *server
	attacker *server
	res      *resolver
	worker   *fakeWorker
	gw       *httptest.Server
	docText  string
	logs     *bytes.Buffer
	// attach holds what the Outline does per attachment id, paths per path.
	attach map[string]http.HandlerFunc
	paths  map[string]http.HandlerFunc

	mu     sync.Mutex
	dialed []string
}

func newWorld(t *testing.T, imageHosts ...string) *world {
	t.Helper()
	w := &world{
		t: t, outline: newServer(t), storage: newServer(t), attacker: newServer(t),
		res:    &resolver{table: map[string][][]netip.Addr{}, calls: map[string]int{}},
		worker: newFakeWorker(t), logs: &bytes.Buffer{},
		attach: map[string]http.HandlerFunc{}, paths: map[string]http.HandlerFunc{},
	}
	w.res.set(storageHost, []string{storageIP})
	w.res.set(attackerHost, []string{attackerIP})
	w.outline.handle(w.outlineAPI)

	started, err := startup.FromService(config.Service{
		TemplatesRoot: "../../templates", ConfigurationsRoot: "../../configurations",
		Bindings:       []config.Binding{{Template: "neutral", Configuration: "example"}},
		DefaultBinding: &config.Binding{Template: "neutral", Configuration: "example"},
	})
	if err != nil {
		t.Fatal(err)
	}
	oc, err := outline.NewClient(w.outline.URL, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	oc, err = oc.WithImages(outline.ImageOptions{Hosts: imageHosts, Resolver: w.res, Dial: w.dial, Timeout: 10 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	wc, err := workerapi.NewClient(w.worker.srv.URL, "", 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(w.logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	w.gw = httptest.NewServer(New(Config{Registry: started.Registry, Outline: oc, Worker: wc, Logger: log}).Handler())
	t.Cleanup(w.gw.Close)
	return w
}

// dial connects the fake public addresses to the fake servers and records
// every address the Gateway tried to reach.
func (w *world) dial(ctx context.Context, network, address string) (net.Conn, error) {
	w.mu.Lock()
	w.dialed = append(w.dialed, address)
	w.mu.Unlock()
	host, _, _ := net.SplitHostPort(address)
	target := address
	switch host {
	case storageIP:
		target = w.storage.addr()
	case attackerIP:
		target = w.attacker.addr()
	default:
		if address != w.outline.addr() {
			return nil, fmt.Errorf("test dialer: unexpected address %s", address)
		}
	}
	return (&net.Dialer{}).DialContext(ctx, network, target)
}

func (w *world) outlineAPI(rw http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/api/documents.info":
		rw.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(rw).Encode(map[string]any{"data": map[string]any{
			"id": "doc1", "title": "Doc", "text": w.docText, "updatedAt": "2026-03-02T23:30:00.000Z", "collectionId": "c1"}})
	case "/api/collections.info":
		rw.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(rw).Encode(map[string]any{"data": map[string]any{"id": "c1", "name": "C", "description": ""}})
	case "/api/attachments.redirect":
		if fn := w.attach[r.URL.Query().Get("id")]; fn != nil {
			fn(rw, r)
			return
		}
		http.NotFound(rw, r)
	default:
		if fn := w.paths[r.URL.Path]; fn != nil {
			fn(rw, r)
			return
		}
		http.NotFound(rw, r)
	}
}

func testPNG(t testing.TB, w, h int) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(x * 7), G: uint8(y * 11), B: 90, A: 255})
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func serve(data []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(data)
	}
}

func redirect(to string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, to, http.StatusFound) }
}

func status(code int) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(code) }
}

func attachmentURL(id string) string { return "/api/attachments.redirect?id=" + id }

// render requests the PDF of the world's Document with the user's cookie.
func (w *world) render() (int, []byte) {
	w.t.Helper()
	req, _ := http.NewRequest(http.MethodGet, w.gw.URL+"/render/doc1", nil)
	req.Header.Set("Cookie", userCookie)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		w.t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, body
}

// errorsOf decodes a 422 body.
func errorsOf(t *testing.T, code int, body []byte) []metadata.Error {
	t.Helper()
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("status %d, want 422: %s", code, body)
	}
	var list metadata.ErrorList
	if err := json.Unmarshal(body, &list); err != nil {
		t.Fatalf("body is not an error list: %s", body)
	}
	return list.Errors
}

func codes(errs []metadata.Error) []string {
	out := make([]string, len(errs))
	for i, e := range errs {
		out[i] = e.Code
	}
	return out
}

// noCookieElsewhere asserts that the user's cookie reached the Outline only.
func (w *world) noCookieElsewhere() {
	w.t.Helper()
	for name, s := range map[string]*server{"storage": w.storage, "attacker": w.attacker} {
		for _, h := range s.recorded() {
			if h.cookie != "" || h.authorization != "" {
				w.t.Errorf("%s received credentials on %s: cookie %q authorization %q", name, h.path, h.cookie, h.authorization)
			}
		}
	}
	if strings.Contains(w.logs.String(), "USER-SESSION-CANARY") {
		w.t.Error("the cookie reached the log")
	}
}

func (w *world) bundleSent() bundle.Bundle {
	w.t.Helper()
	w.worker.mu.Lock()
	defer w.worker.mu.Unlock()
	if len(w.worker.bodies) != 1 {
		w.t.Fatalf("worker received %d bundles, want 1", len(w.worker.bodies))
	}
	b, err := bundle.Decode(bytes.NewReader(w.worker.bodies[0]))
	if err != nil {
		w.t.Fatal(err)
	}
	return b
}

func (w *world) assertWorkerUntouched() {
	w.t.Helper()
	w.worker.mu.Lock()
	defer w.worker.mu.Unlock()
	if n := len(w.worker.bodies); n != 0 {
		w.t.Errorf("worker received %d bundles after an image problem", n)
	}
}

func imageDoc(urls ...string) string {
	var b strings.Builder
	for i, u := range urls {
		fmt.Fprintf(&b, "![Picture %d](%s)\n\n", i+1, u)
	}
	return b.String()
}

func TestImagesRenderFromOutline(t *testing.T) {
	w := newWorld(t)
	w.attach[uuidA] = serve(testPNG(t, 80, 40))
	w.docText = "Intro\n\n![Chart one](" + attachmentURL(uuidA) + " \"=160x\")\n"
	code, body := w.render()
	if code != 200 {
		t.Fatalf("status %d: %s", code, body)
	}
	b := w.bundleSent()
	if got := string(b.Markdown); got != "Intro\n\n![Chart one](images/img-1.png){width=120.00pt height=60.00pt}\n" {
		t.Errorf("markdown = %q", got)
	}
	if len(b.Images) != 1 || len(b.Images["img-1.png"]) == 0 {
		t.Errorf("images = %d", len(b.Images))
	}
	hits := w.outline.recorded()
	last := hits[len(hits)-1]
	if last.path != attachmentURL(uuidA) || last.cookie != userCookie {
		t.Errorf("attachment request %+v: the Outline must get the user's cookie", last)
	}
	w.noCookieElsewhere()
}

func TestImagesAbsoluteAndDuplicateReferencesFetchOnce(t *testing.T) {
	w := newWorld(t)
	w.attach[uuidA] = serve(testPNG(t, 8, 8))
	w.docText = imageDoc(attachmentURL(uuidA), w.outline.URL+attachmentURL(uuidA), attachmentURL(uuidA)+"&x=1")
	if code, body := w.render(); code != 200 {
		t.Fatalf("status %d: %s", code, body)
	}
	n := 0
	for _, h := range w.outline.recorded() {
		if strings.HasPrefix(h.path, "/api/attachments.redirect") {
			n++
		}
	}
	if n != 1 {
		t.Errorf("attachment fetched %d times, want 1", n)
	}
	if got := len(w.bundleSent().Images); got != 1 {
		t.Errorf("%d images in the bundle, want 1", got)
	}
}

func TestImagesRedirectToAllowListedStorageWithoutCookie(t *testing.T) {
	w := newWorld(t, storageHost)
	w.attach[uuidA] = redirect("http://" + storageHost + "/bucket/a.png?sig=1")
	w.storage.handle(serve(testPNG(t, 8, 8)))
	w.docText = imageDoc(attachmentURL(uuidA))
	if code, body := w.render(); code != 200 {
		t.Fatalf("status %d: %s", code, body)
	}
	if hits := w.storage.recorded(); len(hits) != 1 || hits[0].path != "/bucket/a.png?sig=1" {
		t.Errorf("storage hits = %+v", hits)
	}
	w.noCookieElsewhere()
}

func TestImagesRedirectToOutlineOriginCarriesNoCookie(t *testing.T) {
	w := newWorld(t)
	w.attach[uuidA] = redirect("/api/files.get?key=k")
	w.paths["/api/files.get"] = serve(testPNG(t, 8, 8))
	w.docText = imageDoc(attachmentURL(uuidA))
	if code, body := w.render(); code != 200 {
		t.Fatalf("status %d: %s", code, body)
	}
	hits := w.outline.recorded()
	if last := hits[len(hits)-1]; last.path != "/api/files.get?key=k" || last.cookie != "" {
		t.Errorf("redirect hop to the origin: %+v, want no cookie", last)
	}
}

func TestImagesRedirectToAttackerIsRejected(t *testing.T) {
	cases := map[string]string{
		"host not in the allow-list":   "http://" + attackerHost + "/steal.png",
		"protocol-relative location":   "//" + attackerHost + "/steal.png",
		"userinfo naming the attacker": "http://" + storageHost + "@" + attackerHost + "/steal.png",
		"userinfo on an allowed host":  "http://" + attackerHost + "@" + storageHost + "/a.png",
		"file scheme":                  "file:///etc/passwd",
		"ftp scheme":                   "ftp://" + storageHost + "/a.png",
		"allowed name with other port": "http://" + storageHost + ":8080/a.png",
		"allowed name as a subdomain":  "http://evil." + storageHost + "/a.png",
		"allowed name as a suffix":     "http://" + storageHost + ".example.invalid/a.png",
		"attacker via https":           "https://" + attackerHost + "/steal.png",
	}
	for name, location := range cases {
		t.Run(name, func(t *testing.T) {
			w := newWorld(t, storageHost)
			w.res.set("evil."+storageHost, []string{attackerIP})
			w.res.set(storageHost+".example.invalid", []string{attackerIP})
			w.attach[uuidA] = redirect(location)
			w.attacker.handle(serve(testPNG(t, 8, 8)))
			w.docText = imageDoc(attachmentURL(uuidA))
			code, body := w.render()
			errs := errorsOf(t, code, body)
			if len(errs) != 1 || errs[0].Code != "forbidden" {
				t.Fatalf("errors = %+v", errs)
			}
			if n := len(w.attacker.recorded()); n != 0 {
				t.Errorf("the attacker was contacted %d times", n)
			}
			w.assertWorkerUntouched()
			w.noCookieElsewhere()
		})
	}
}

func TestImagesRedirectToAllowListedAttackerStillGetsNoCookie(t *testing.T) {
	w := newWorld(t, attackerHost)
	w.attach[uuidA] = redirect("http://" + attackerHost + "/a.png")
	w.attacker.handle(serve(testPNG(t, 8, 8)))
	w.docText = imageDoc(attachmentURL(uuidA))
	if code, body := w.render(); code != 200 {
		t.Fatalf("status %d: %s", code, body)
	}
	if len(w.attacker.recorded()) != 1 {
		t.Fatal("the allow-listed host should have been fetched")
	}
	w.noCookieElsewhere()
}

func TestImagesRedirectChain(t *testing.T) {
	for _, tt := range []struct {
		redirects int
		ok        bool
	}{{5, true}, {6, false}, {40, false}} {
		t.Run(fmt.Sprintf("%d redirects", tt.redirects), func(t *testing.T) {
			w := newWorld(t, storageHost)
			w.attach[uuidA] = redirect("http://" + storageHost + "/hop/1")
			w.storage.handle(func(rw http.ResponseWriter, r *http.Request) {
				var n int
				_, _ = fmt.Sscanf(r.URL.Path, "/hop/%d", &n)
				if n < tt.redirects {
					http.Redirect(rw, r, fmt.Sprintf("/hop/%d", n+1), http.StatusFound)
					return
				}
				serve(testPNG(t, 8, 8))(rw, r)
			})
			w.docText = imageDoc(attachmentURL(uuidA))
			code, body := w.render()
			if tt.ok {
				if code != 200 {
					t.Fatalf("status %d: %s", code, body)
				}
				return
			}
			errs := errorsOf(t, code, body)
			if len(errs) != 1 || errs[0].Code != "forbidden" || !strings.Contains(errs[0].Message, "redirects") {
				t.Errorf("errors = %+v", errs)
			}
			if n := len(w.storage.recorded()); n > limits.MaxImageRedirects {
				t.Errorf("followed %d hops", n)
			}
			w.noCookieElsewhere()
		})
	}
}

func TestImagesAllowedNameResolvingToNonPublicAddressIsRejected(t *testing.T) {
	addrs := []string{
		"127.0.0.1", "127.1.2.3", "10.0.0.5", "172.16.0.9", "192.168.1.1", "169.254.169.254", "100.100.100.200",
		"0.0.0.0", "192.0.0.8", "198.18.0.1", "224.0.0.1", "255.255.255.255", "168.63.129.16",
		"::1", "::", "fe80::1", "fc00::1", "fd00:ec2::254", "::ffff:127.0.0.1", "::ffff:10.0.0.1",
		"::ffff:169.254.169.254", "64:ff9b::7f00:1", "2002:7f00:1::1", "2001:db8::1", "ff02::1",
	}
	for _, addr := range addrs {
		t.Run(addr, func(t *testing.T) {
			w := newWorld(t, storageHost)
			w.res.set(storageHost, []string{addr})
			w.attach[uuidA] = redirect("http://" + storageHost + "/a.png")
			w.docText = imageDoc(attachmentURL(uuidA))
			code, body := w.render()
			errs := errorsOf(t, code, body)
			if len(errs) != 1 || errs[0].Code != "forbidden" {
				t.Fatalf("errors = %+v", errs)
			}
			for _, d := range w.dialed {
				if d != w.outline.addr() {
					t.Errorf("dialed %s", d)
				}
			}
		})
	}
}

func TestImagesRedirectToLiteralAddressesIsRejected(t *testing.T) {
	targets := []string{
		"http://127.0.0.1/a.png", "http://127.0.0.1:" + "80/a.png", "http://localhost/a.png",
		"http://[::1]/a.png", "http://[::ffff:127.0.0.1]/a.png", "http://[0:0:0:0:0:ffff:7f00:1]/a.png",
		"http://[fe80::1%25eth0]/a.png", "http://169.254.169.254/latest/meta-data/",
		"http://0x7f.1/a.png", "http://2130706433/a.png", "http://127.1/a.png", "http://0177.0.0.1/a.png",
		"http://10.0.0.5/a.png", "http://[fd00:ec2::254]/latest/meta-data/",
	}
	for _, target := range targets {
		t.Run(target, func(t *testing.T) {
			w := newWorld(t, storageHost)
			w.attach[uuidA] = redirect(target)
			w.docText = imageDoc(attachmentURL(uuidA))
			code, body := w.render()
			if errs := errorsOf(t, code, body); len(errs) != 1 || errs[0].Code != "forbidden" {
				t.Errorf("errors = %+v", errs)
			}
			for _, d := range w.dialed {
				if d != w.outline.addr() {
					t.Errorf("dialed %s", d)
				}
			}
		})
	}
}

func TestImagesDNSRebindingIsResolvedOnce(t *testing.T) {
	t.Run("public first, loopback on the second lookup", func(t *testing.T) {
		w := newWorld(t, storageHost)
		w.res.set(storageHost, []string{storageIP}, []string{"127.0.0.1"})
		w.attach[uuidA] = redirect("http://" + storageHost + "/a.png")
		w.storage.handle(serve(testPNG(t, 8, 8)))
		w.docText = imageDoc(attachmentURL(uuidA))
		if code, body := w.render(); code != 200 {
			t.Fatalf("status %d: %s", code, body)
		}
		if n := w.res.calls[storageHost]; n != 1 {
			t.Errorf("%s resolved %d times for one hop, want 1", storageHost, n)
		}
		for _, d := range w.dialed {
			if strings.HasPrefix(d, "127.0.0.1:") && d != w.outline.addr() {
				t.Errorf("dialed %s", d)
			}
		}
	})
	t.Run("one answer mixes public and loopback", func(t *testing.T) {
		w := newWorld(t, storageHost)
		w.res.set(storageHost, []string{storageIP, "127.0.0.1"})
		w.attach[uuidA] = redirect("http://" + storageHost + "/a.png")
		w.storage.handle(serve(testPNG(t, 8, 8)))
		w.docText = imageDoc(attachmentURL(uuidA))
		code, body := w.render()
		if errs := errorsOf(t, code, body); len(errs) != 1 || errs[0].Code != "forbidden" {
			t.Errorf("errors = %+v", errs)
		}
		if len(w.storage.recorded()) != 0 {
			t.Error("storage contacted although one answer was loopback")
		}
	})
	t.Run("loopback first", func(t *testing.T) {
		w := newWorld(t, storageHost)
		w.res.set(storageHost, []string{"127.0.0.1"}, []string{storageIP})
		w.attach[uuidA] = redirect("http://" + storageHost + "/a.png")
		w.docText = imageDoc(attachmentURL(uuidA))
		code, body := w.render()
		if errs := errorsOf(t, code, body); len(errs) != 1 {
			t.Errorf("errors = %+v", errs)
		}
	})
}

func TestImagesExternalAndOddAddressesAreForbidden(t *testing.T) {
	addresses := []string{
		"https://cdn.example.invalid/pic.png",
		"http://cdn.example.invalid/pic.png",
		"//cdn.example.invalid/pic.png",
		"file:///etc/passwd",
		"data:image/png;base64,iVBORw0KGgo=",
		"https://user:secret@cdn.example.invalid/pic.png",
		"http://[::1]/pic.png",
		"http://[::ffff:127.0.0.1]/pic.png",
		"http://127.0.0.1/pic.png",
		"http://169.254.169.254/latest/meta-data/",
		"javascript:alert(1)",
		"images/img-1.png",
		"../../etc/passwd",
		"/etc/passwd",
		"/api/documents.delete",
		w0() + "@" + attackerHost + "/api/attachments.redirect?id=" + uuidA,
	}
	w := newWorld(t, storageHost)
	w.docText = imageDoc(addresses...)
	code, body := w.render()
	errs := errorsOf(t, code, body)
	if len(errs) != len(addresses) {
		t.Fatalf("%d errors for %d images: %+v", len(errs), len(addresses), errs)
	}
	for i, e := range errs {
		if e.Code != "forbidden" || e.Source != "document" || e.Field != fmt.Sprintf("Picture %d", i+1) || e.Line == nil || *e.Line != 1+2*i {
			t.Errorf("error %d = %+v", i, e)
		}
	}
	for _, h := range w.outline.recorded() {
		if strings.HasPrefix(h.path, "/api/attachments") || strings.HasPrefix(h.path, "/api/files") || strings.Contains(h.path, "delete") {
			t.Errorf("the Outline was asked for %s", h.path)
		}
	}
	if len(w.storage.recorded())+len(w.attacker.recorded()) != 0 || len(w.dialed) != 0 {
		t.Error("a forbidden address caused a connection")
	}
	w.assertWorkerUntouched()
}

func w0() string { return "https://" + "outline.example.invalid" }

func TestImagesAllProblemsInOneList(t *testing.T) {
	w := newWorld(t, storageHost)
	w.attach[uuidA] = serve(testPNG(t, 8, 8))
	w.attach[uuidB] = serve([]byte("<!doctype html><title>Sign in</title>"))
	w.attach["c1c1c1c1-0000-4000-8000-000000000003"] = status(http.StatusNotFound)
	w.attach["c1c1c1c1-0000-4000-8000-000000000004"] = status(http.StatusForbidden)
	w.attach["c1c1c1c1-0000-4000-8000-000000000005"] = serve([]byte("<svg xmlns='http://www.w3.org/2000/svg'/>"))
	w.attach["c1c1c1c1-0000-4000-8000-000000000006"] = serve(append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 30)...))
	w.docText = "```yaml metadata\nsubjct: oops\n```\n\n" +
		imageDoc(attachmentURL(uuidA), attachmentURL(uuidB),
			attachmentURL("c1c1c1c1-0000-4000-8000-000000000003"), attachmentURL("c1c1c1c1-0000-4000-8000-000000000004"),
			attachmentURL("c1c1c1c1-0000-4000-8000-000000000005"), attachmentURL("c1c1c1c1-0000-4000-8000-000000000006"),
			"https://cdn.example.invalid/x.png", "/api/attachments.redirect")
	code, body := w.render()
	errs := errorsOf(t, code, body)
	want := []string{"unknown", "format", "missing", "forbidden", "svg_rejected", "decode", "forbidden", "missing"}
	if got := codes(errs); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("codes = %v, want %v\n%s", got, want, body)
	}
	if errs[0].Field != "subjct" || errs[1].Field != "Picture 2" || errs[1].Line == nil || *errs[1].Line != 7 {
		t.Errorf("errors = %+v", errs)
	}
	w.assertWorkerUntouched()
	w.noCookieElsewhere()
}

func TestImagesStatusesOfTheAttachmentFetch(t *testing.T) {
	cases := map[string]struct {
		handler http.HandlerFunc
		code    string
	}{
		"not found":  {status(404), "missing"},
		"gone":       {status(410), "unavailable"},
		"forbidden":  {status(403), "forbidden"},
		"html page":  {serve([]byte("<html>login</html>")), "format"},
		"empty body": {serve(nil), "format"},
	}
	for name, tt := range cases {
		t.Run(name, func(t *testing.T) {
			w := newWorld(t)
			w.attach[uuidA] = tt.handler
			w.docText = imageDoc(attachmentURL(uuidA))
			code, body := w.render()
			if tt.code == "unavailable" {
				if code != 500 {
					t.Fatalf("status %d: %s", code, body)
				}
				return
			}
			if errs := errorsOf(t, code, body); len(errs) != 1 || errs[0].Code != tt.code {
				t.Errorf("errors = %+v", errs)
			}
		})
	}
}

func TestImagesUnauthorizedAttachmentIs401(t *testing.T) {
	w := newWorld(t)
	w.attach[uuidA] = status(http.StatusUnauthorized)
	w.docText = imageDoc(attachmentURL(uuidA))
	if code, _ := w.render(); code != http.StatusUnauthorized {
		t.Errorf("status %d, want 401", code)
	}
}

func TestImagesTooLargeDownload(t *testing.T) {
	big := make([]byte, limits.MaxImageBytes+1)
	copy(big, "\x89PNG\r\n\x1a\n")
	for name, h := range map[string]http.HandlerFunc{
		"announced": serve(big),
		"streamed": func(rw http.ResponseWriter, _ *http.Request) {
			rw.Header().Set("Transfer-Encoding", "chunked")
			f := rw.(http.Flusher)
			for off := 0; off < len(big); off += 1 << 20 {
				_, _ = rw.Write(big[off:min(off+1<<20, len(big))])
				f.Flush()
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			w := newWorld(t)
			w.attach[uuidA] = h
			w.docText = imageDoc(attachmentURL(uuidA))
			code, body := w.render()
			if errs := errorsOf(t, code, body); len(errs) != 1 || errs[0].Code != "too_large" {
				t.Errorf("errors = %+v", errs)
			}
		})
	}
}

func TestImagesBlockedHostNamesTheHostForTheAuthor(t *testing.T) {
	w := newWorld(t)
	w.attach[uuidA] = redirect("http://" + storageHost + "/a.png")
	w.docText = imageDoc(attachmentURL(uuidA))
	code, body := w.render()
	errs := errorsOf(t, code, body)
	if len(errs) != 1 || !strings.Contains(errs[0].Message, storageHost) || !strings.Contains(errs[0].Message, "imageHosts") {
		t.Errorf("errors = %+v", errs)
	}
}

func TestImagesDoNotRunForCodeBlocks(t *testing.T) {
	w := newWorld(t)
	w.docText = "```\n![a](https://cdn.example.invalid/x.png)\n```\n\nand `![b](https://cdn.example.invalid/y.png)`\n"
	if code, body := w.render(); code != 200 {
		t.Fatalf("status %d: %s", code, body)
	}
}

func TestImagesNoCanaryInLogs(t *testing.T) {
	w := newWorld(t)
	w.attach[uuidA] = status(http.StatusInternalServerError)
	w.docText = imageDoc(attachmentURL(uuidA))
	w.render()
	if strings.Contains(w.logs.String(), "USER-SESSION-CANARY") || strings.Contains(w.logs.String(), uuidA) {
		t.Errorf("log leaks:\n%s", w.logs)
	}
}
