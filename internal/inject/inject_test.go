package inject

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

const scriptTag = `<script src="/pdf/button.js" defer data-outline-pdf-injector></script>`

const outlinePage = `<!DOCTYPE html><html><head><meta charset="utf-8"><title>Wiki</title></head><body><div id="root"></div></body></html>`

// newProxy starts the injector in front of upstream and returns its server.
func newProxy(t *testing.T, upstream http.Handler, mutate func(*Config)) *httptest.Server {
	t.Helper()
	up := httptest.NewServer(upstream)
	t.Cleanup(up.Close)
	cfg := Config{Upstream: up.URL}
	if mutate != nil {
		mutate(&cfg)
	}
	h, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv
}

// rawClient does not decompress by itself, so tests see the wire bytes.
var rawClient = &http.Client{Transport: &http.Transport{DisableCompression: true}}

func get(t *testing.T, url string, header http.Header) (*http.Response, []byte) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range header {
		req.Header[k] = v
	}
	return do(t, req)
}

func do(t *testing.T, req *http.Request) (*http.Response, []byte) {
	t.Helper()
	resp, err := rawClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", req.Method, req.URL, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return resp, body
}

func serveHTML(contentType, body string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Etag", `"v1"`)
		_, _ = io.WriteString(w, body)
	})
}

func TestInjectsBeforeClosingBody(t *testing.T) {
	srv := newProxy(t, serveHTML("text/html; charset=utf-8", outlinePage), nil)

	resp, body := get(t, srv.URL+"/doc/x", nil)

	want := strings.Replace(outlinePage, "</body>", scriptTag+"</body>", 1)
	if string(body) != want {
		t.Fatalf("body =\n%s\nwant\n%s", body, want)
	}
	if got := resp.Header.Get("Content-Length"); got != strconv.Itoa(len(want)) {
		t.Errorf("Content-Length = %q, want %d", got, len(want))
	}
	if resp.Header.Get("Etag") != "" {
		t.Errorf("Etag survived the rewrite: %q", resp.Header.Get("Etag"))
	}
}

func TestInjectionPointVariants(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{"upper case body", "<HTML><BODY>x</BODY></HTML>", "<HTML><BODY>x" + scriptTag + "</BODY></HTML>"},
		{"last body wins", "<body>a</body> <!-- </body> --></html>", "<body>a</body> <!-- " + scriptTag + "</body> --></html>"},
		{"no body falls back to head", "<html><head><title>t</title></head></html>", "<html><head><title>t</title>" + scriptTag + "</head></html>"},
		{"mixed case head", "<html><HeAd></hEAD></html>", "<html><HeAd>" + scriptTag + "</hEAD></html>"},
		{"neither is left alone", "<p>fragment</p>", "<p>fragment</p>"},
		{"non ascii before the tag", "<body>Grüße İ</body>", "<body>Grüße İ" + scriptTag + "</body>"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := newProxy(t, serveHTML("text/html", tc.in), nil)
			_, body := get(t, srv.URL+"/", nil)
			if string(body) != tc.want {
				t.Errorf("body =\n%s\nwant\n%s", body, tc.want)
			}
		})
	}
}

func TestContentTypeVariantsAreHTML(t *testing.T) {
	for _, ct := range []string{"text/html", "TEXT/HTML; charset=ISO-8859-1", `text/html; charset="utf-8"`} {
		srv := newProxy(t, serveHTML(ct, outlinePage), nil)
		_, body := get(t, srv.URL+"/", nil)
		if !bytes.Contains(body, []byte(scriptTag)) {
			t.Errorf("%q: tag not injected", ct)
		}
	}
}

func TestOtherContentIsPassedThroughUntouched(t *testing.T) {
	const withBody = "</body>"
	tests := []struct{ name, contentType string }{
		{"json", "application/json"},
		{"javascript", "text/javascript"},
		{"image", "image/png"},
		{"plain text", "text/plain"},
		{"xhtml", "application/xhtml+xml"},
		{"missing", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			up := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if tc.contentType != "" {
					w.Header().Set("Content-Type", tc.contentType)
				} else {
					w.Header()["Content-Type"] = nil
				}
				w.Header().Set("Etag", `"keep"`)
				_, _ = io.WriteString(w, withBody)
			})
			srv := newProxy(t, up, nil)
			resp, body := get(t, srv.URL+"/x", nil)
			if string(body) != withBody {
				t.Errorf("body = %q, want it unchanged", body)
			}
			if resp.Header.Get("Etag") != `"keep"` {
				t.Errorf("Etag = %q, want it kept", resp.Header.Get("Etag"))
			}
		})
	}
}

func TestOnlyFullHTMLResponsesAreRewritten(t *testing.T) {
	t.Run("304 has no body and keeps its headers", func(t *testing.T) {
		up := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			w.Header().Set("Etag", `"v1"`)
			w.WriteHeader(http.StatusNotModified)
		})
		srv := newProxy(t, up, nil)
		resp, body := get(t, srv.URL+"/asset", nil)
		if resp.StatusCode != http.StatusNotModified || len(body) != 0 || resp.Header.Get("Etag") != `"v1"` {
			t.Errorf("got %d %q etag %q", resp.StatusCode, body, resp.Header.Get("Etag"))
		}
	})
	t.Run("204", func(t *testing.T) {
		up := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusNoContent)
		})
		srv := newProxy(t, up, nil)
		resp, body := get(t, srv.URL+"/", nil)
		if resp.StatusCode != http.StatusNoContent || len(body) != 0 {
			t.Errorf("got %d %q", resp.StatusCode, body)
		}
	})
	t.Run("HEAD keeps the length of the page", func(t *testing.T) {
		srv := newProxy(t, serveHTML("text/html", outlinePage), nil)
		req, _ := http.NewRequest(http.MethodHead, srv.URL+"/", nil)
		resp, body := do(t, req)
		if len(body) != 0 || resp.Header.Get("Content-Length") != strconv.Itoa(len(outlinePage)) {
			t.Errorf("HEAD: body %q, Content-Length %q, want %d", body, resp.Header.Get("Content-Length"), len(outlinePage))
		}
	})
	t.Run("range responses", func(t *testing.T) {
		up := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			w.Header().Set("Content-Range", "bytes 0-6/100")
			w.WriteHeader(http.StatusPartialContent)
			_, _ = io.WriteString(w, "</body>")
		})
		srv := newProxy(t, up, nil)
		resp, body := get(t, srv.URL+"/", http.Header{"Range": {"bytes=0-6"}})
		if resp.StatusCode != http.StatusPartialContent || string(body) != "</body>" {
			t.Errorf("got %d %q", resp.StatusCode, body)
		}
	})
	t.Run("error pages", func(t *testing.T) {
		up := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, outlinePage)
		})
		srv := newProxy(t, up, nil)
		resp, body := get(t, srv.URL+"/missing", nil)
		if resp.StatusCode != http.StatusNotFound || string(body) != outlinePage {
			t.Errorf("got %d %q", resp.StatusCode, body)
		}
	})
}

func TestNeverInjectsTwice(t *testing.T) {
	once := strings.Replace(outlinePage, "</body>", scriptTag+"</body>", 1)
	srv := newProxy(t, serveHTML("text/html", once), nil)
	_, body := get(t, srv.URL+"/", nil)
	if got := bytes.Count(body, []byte("data-outline-pdf-injector")); got != 1 {
		t.Errorf("marker count = %d, want 1", got)
	}
}

func TestScriptURLIsEscapedAndConfigurable(t *testing.T) {
	srv := newProxy(t, serveHTML("text/html", "<body></body>"), func(c *Config) {
		c.ScriptURL = `/static/pdf.js?a=1&b="2"`
	})
	_, body := get(t, srv.URL+"/", nil)
	want := `<body><script src="/static/pdf.js?a=1&amp;b=&#34;2&#34;" defer data-outline-pdf-injector></script></body>`
	if string(body) != want {
		t.Errorf("body = %s, want %s", body, want)
	}
}

func gzipped(t *testing.T, s string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write([]byte(s)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestGzipResponseIsDecodedInjectedAndSentAsIdentity(t *testing.T) {
	packed := gzipped(t, outlinePage)
	up := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("Content-Length", strconv.Itoa(len(packed)))
		w.Header().Set("Etag", `"gz"`)
		_, _ = w.Write(packed)
	})
	srv := newProxy(t, up, nil)

	resp, body := get(t, srv.URL+"/", http.Header{"Accept-Encoding": {"gzip"}})

	want := strings.Replace(outlinePage, "</body>", scriptTag+"</body>", 1)
	if string(body) != want {
		t.Fatalf("body = %q, want %q", body, want)
	}
	if enc := resp.Header.Get("Content-Encoding"); enc != "" {
		t.Errorf("Content-Encoding = %q, want none", enc)
	}
	if got := resp.Header.Get("Content-Length"); got != strconv.Itoa(len(want)) {
		t.Errorf("Content-Length = %q, want %d", got, len(want))
	}
	if resp.Header.Get("Etag") != "" {
		t.Errorf("Etag = %q, want none", resp.Header.Get("Etag"))
	}
}

func TestUnsupportedEncodingIsPassedThroughByteForByte(t *testing.T) {
	payload := []byte("\x00br-bytes</body>")
	up := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Header().Set("Content-Encoding", "br")
		_, _ = w.Write(payload)
	})
	srv := newProxy(t, up, nil)
	resp, body := get(t, srv.URL+"/", nil)
	if !bytes.Equal(body, payload) || resp.Header.Get("Content-Encoding") != "br" {
		t.Errorf("got %q encoding %q", body, resp.Header.Get("Content-Encoding"))
	}
}

func TestCorruptGzipIsABadGateway(t *testing.T) {
	up := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Header().Set("Content-Encoding", "gzip")
		_, _ = io.WriteString(w, "this is not gzip")
	})
	srv := newProxy(t, up, nil)
	resp, _ := get(t, srv.URL+"/", nil)
	if resp.StatusCode != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", resp.StatusCode)
	}
}

func TestNavigationsAskUpstreamForFreshUncompressedHTML(t *testing.T) {
	var got http.Header
	up := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, outlinePage)
	})
	srv := newProxy(t, up, nil)

	get(t, srv.URL+"/doc/x", http.Header{
		"Accept":            {"text/html,application/xhtml+xml,*/*;q=0.8"},
		"Accept-Encoding":   {"gzip, br"},
		"If-None-Match":     {`"old"`},
		"If-Modified-Since": {"Mon, 02 Jan 2006 15:04:05 GMT"},
	})
	if got.Get("Accept-Encoding") != "identity" || got.Get("If-None-Match") != "" || got.Get("If-Modified-Since") != "" {
		t.Errorf("navigation headers upstream: %v", got)
	}

	get(t, srv.URL+"/api/x", http.Header{
		"Accept":          {"application/json"},
		"Accept-Encoding": {"gzip"},
		"If-None-Match":   {`"old"`},
	})
	if got.Get("Accept-Encoding") != "gzip" || got.Get("If-None-Match") != `"old"` {
		t.Errorf("non-navigation headers upstream: %v", got)
	}
}

func TestLargeHTMLIsStreamedUnchanged(t *testing.T) {
	big := "<body>" + strings.Repeat("x", 4096) + "</body>"
	srv := newProxy(t, serveHTML("text/html", big), func(c *Config) { c.MaxHTMLBytes = 1024 })
	resp, body := get(t, srv.URL+"/", nil)
	if string(body) != big {
		t.Errorf("body changed: %d bytes, want %d", len(body), len(big))
	}
	if resp.Header.Get("Etag") != `"v1"` {
		t.Errorf("Etag = %q, want it kept for an untouched body", resp.Header.Get("Etag"))
	}
}

func TestLargeGzipHTMLIsStreamedDecoded(t *testing.T) {
	big := "<body>" + strings.Repeat("x", 4096) + "</body>"
	packed := gzipped(t, big)
	up := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("Content-Length", strconv.Itoa(len(packed)))
		_, _ = w.Write(packed)
	})
	srv := newProxy(t, up, func(c *Config) { c.MaxHTMLBytes = 1024 })
	resp, body := get(t, srv.URL+"/", nil)
	if string(body) != big || resp.Header.Get("Content-Encoding") != "" {
		t.Errorf("got %d bytes, encoding %q", len(body), resp.Header.Get("Content-Encoding"))
	}
}

func TestStreamingResponsesAreNotBuffered(t *testing.T) {
	release := make(chan struct{})
	up := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "data: one\n\n")
		w.(http.Flusher).Flush()
		<-release
		_, _ = io.WriteString(w, "data: two\n\n")
	})
	srv := newProxy(t, up, nil)
	resp, err := rawClient.Get(srv.URL + "/api/events")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	line := make(chan string, 1)
	go func() {
		s, _ := bufio.NewReader(resp.Body).ReadString('\n')
		line <- s
	}()
	select {
	case s := <-line:
		if s != "data: one\n" {
			t.Errorf("first event = %q", s)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("first event did not arrive while the stream was open")
	}
	close(release)
}

func TestWebSocketUpgradePassesThrough(t *testing.T) {
	up := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
			http.Error(w, "no upgrade", http.StatusBadRequest)
			return
		}
		conn, rw, err := w.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		defer conn.Close()
		_, _ = rw.WriteString("HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: websocket\r\nContent-Type: text/html\r\n\r\n")
		_ = rw.Flush()
		buf := make([]byte, 5)
		if _, err := io.ReadFull(rw, buf); err != nil {
			return
		}
		_, _ = conn.Write(append([]byte("echo:"), buf...))
	})
	srv := newProxy(t, up, nil)

	conn, err := net.Dial("tcp", strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	_, _ = io.WriteString(conn, "GET /realtime HTTP/1.1\r\nHost: wiki.example.invalid\r\nConnection: Upgrade\r\nUpgrade: websocket\r\n\r\n")
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		t.Fatalf("read upgrade response: %v", err)
	}
	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("status = %d, want 101", resp.StatusCode)
	}
	_, _ = io.WriteString(conn, "hello")
	got := make([]byte, len("echo:hello"))
	if _, err := io.ReadFull(br, got); err != nil || string(got) != "echo:hello" {
		t.Errorf("echo = %q, err %v", got, err)
	}
}

func TestHopByHopHeadersAreNotForwarded(t *testing.T) {
	var got http.Header
	up := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Connection", "X-Resp-Hop")
		w.Header().Set("X-Resp-Hop", "secret")
		w.Header().Set("Keep-Alive", "timeout=5")
		_, _ = io.WriteString(w, "{}")
	})
	srv := newProxy(t, up, nil)
	resp, _ := get(t, srv.URL+"/api/x", http.Header{
		"Connection":          {"X-Req-Hop"},
		"X-Req-Hop":           {"secret"},
		"Keep-Alive":          {"timeout=5"},
		"Proxy-Authorization": {"Basic abc"},
	})
	for _, h := range []string{"X-Req-Hop", "Keep-Alive", "Proxy-Authorization"} {
		if got.Get(h) != "" {
			t.Errorf("request header %s reached upstream: %q", h, got.Get(h))
		}
	}
	for _, h := range []string{"X-Resp-Hop", "Keep-Alive"} {
		if resp.Header.Get(h) != "" {
			t.Errorf("response header %s reached the client: %q", h, resp.Header.Get(h))
		}
	}
}

func TestUpstreamIsFixed(t *testing.T) {
	var gotHost, gotPath string
	up := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHost, gotPath = r.Host, r.URL.Path
	})
	srv := newProxy(t, up, nil)

	// An absolute-form request line names another host; it must not be followed.
	conn, err := net.Dial("tcp", strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	_, _ = io.WriteString(conn, "GET http://evil.example.invalid/steal HTTP/1.1\r\nHost: evil.example.invalid\r\nConnection: close\r\n\r\n")
	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if gotPath != "/steal" {
		t.Fatalf("upstream path = %q, want /steal (served by the fixed upstream)", gotPath)
	}
	if gotHost != "evil.example.invalid" {
		t.Errorf("upstream saw Host %q; the public host should be kept for Outline", gotHost)
	}
}

func TestForwardedHeadersAreAppended(t *testing.T) {
	var got http.Header
	up := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { got = r.Header.Clone() })
	srv := newProxy(t, up, nil)
	get(t, srv.URL+"/", http.Header{"X-Forwarded-Proto": {"https"}, "X-Forwarded-For": {"203.0.113.9"}})
	if got.Get("X-Forwarded-Proto") != "https" {
		t.Errorf("X-Forwarded-Proto = %q, want it kept", got.Get("X-Forwarded-Proto"))
	}
	if !strings.HasPrefix(got.Get("X-Forwarded-For"), "203.0.113.9, ") {
		t.Errorf("X-Forwarded-For = %q", got.Get("X-Forwarded-For"))
	}
}

func TestUpstreamDownIsABadGateway(t *testing.T) {
	up := httptest.NewServer(http.NotFoundHandler())
	url := up.URL
	up.Close()
	h, err := New(Config{Upstream: url})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h)
	defer srv.Close()
	resp, body := get(t, srv.URL+"/", nil)
	if resp.StatusCode != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", resp.StatusCode)
	}
	if strings.Contains(string(body), "127.0.0.1") {
		t.Errorf("error body leaks the upstream address: %q", body)
	}
}

func TestHealthz(t *testing.T) {
	called := false
	srv := newProxy(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }), nil)
	resp, _ := get(t, srv.URL+"/_injector/healthz", nil)
	if resp.StatusCode != http.StatusOK || called {
		t.Errorf("status %d, upstream called %v", resp.StatusCode, called)
	}
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/_injector/healthz", nil)
	resp, _ = do(t, req)
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("POST status = %d, want 405", resp.StatusCode)
	}
}
