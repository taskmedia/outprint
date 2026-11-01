package inject

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type seen struct {
	method, path, query, cookie, host string
}

// gatewayStub records the last request and answers like the Gateway.
func gatewayStub(t *testing.T, last *seen) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*last = seen{r.Method, r.URL.Path, r.URL.RawQuery, r.Header.Get("Cookie"), r.Host}
		if r.URL.Path == "/button.js" {
			w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
			_, _ = io.WriteString(w, "/* </body> */")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"available":true}`)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func proxyWithGateway(t *testing.T, gw *httptest.Server, upstreamHits *int) *httptest.Server {
	t.Helper()
	up := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		*upstreamHits++
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, outlinePage)
	})
	return newProxy(t, up, func(c *Config) { c.Gateway = gw.URL })
}

func TestGatewayPrefixIsStrippedAndCookieForwarded(t *testing.T) {
	var last seen
	var hits int
	gw := gatewayStub(t, &last)
	srv := proxyWithGateway(t, gw, &hits)

	resp, body := get(t, srv.URL+"/pdf/status/abc123?x=1", http.Header{"Cookie": {"accessToken=jwt-value; other=1"}})

	if resp.StatusCode != http.StatusOK || !json.Valid(body) {
		t.Fatalf("got %d %q", resp.StatusCode, body)
	}
	if last.method != "GET" || last.path != "/status/abc123" || last.query != "x=1" {
		t.Errorf("gateway saw %+v", last)
	}
	if last.cookie != "accessToken=jwt-value; other=1" {
		t.Errorf("cookie = %q, want it forwarded", last.cookie)
	}
	if last.host != strings.TrimPrefix(gw.URL, "http://") {
		t.Errorf("gateway Host = %q, want its own", last.host)
	}
	if hits != 0 {
		t.Errorf("Outline was called %d times for a gateway path", hits)
	}
}

func TestGatewayScriptIsNotRewritten(t *testing.T) {
	var last seen
	var hits int
	srv := proxyWithGateway(t, gatewayStub(t, &last), &hits)
	_, body := get(t, srv.URL+"/pdf/button.js", nil)
	if string(body) != "/* </body> */" {
		t.Errorf("script body = %q", body)
	}
}

func TestGatewayPathsOnlyAllowReads(t *testing.T) {
	var last seen
	var hits int
	srv := proxyWithGateway(t, gatewayStub(t, &last), &hits)
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/pdf/render/abc", strings.NewReader("x"))
	resp, _ := do(t, req)
	if resp.StatusCode != http.StatusMethodNotAllowed || last.method != "" {
		t.Errorf("POST: status %d, gateway saw %+v", resp.StatusCode, last)
	}
}

func TestGatewayRouteRejectsOddPaths(t *testing.T) {
	var last seen
	var hits int
	srv := proxyWithGateway(t, gatewayStub(t, &last), &hits)
	for _, p := range []string{"/pdf", "/pdf/", "/pdf/../api/users.list", "/pdf/render/../../x", "/pdf//status"} {
		last = seen{}
		resp, _ := get(t, srv.URL+p, nil)
		if resp.StatusCode != http.StatusNotFound || last.path != "" {
			t.Errorf("%s: status %d, gateway saw %+v", p, resp.StatusCode, last)
		}
	}
}

func TestOtherPathsStillGoToOutline(t *testing.T) {
	var last seen
	var hits int
	srv := proxyWithGateway(t, gatewayStub(t, &last), &hits)
	for _, p := range []string{"/pdfx/foo", "/api/pdf/status", "/doc/pdf-abc"} {
		get(t, srv.URL+p, nil)
	}
	if hits != 3 || last.path != "" {
		t.Errorf("outline hits %d, gateway saw %+v", hits, last)
	}
}

func TestCustomPrefix(t *testing.T) {
	var last seen
	gw := gatewayStub(t, &last)
	srv := newProxy(t, http.NotFoundHandler(), func(c *Config) {
		c.Gateway = gw.URL
		c.Prefix = "/_pdf"
	})
	get(t, srv.URL+"/_pdf/render/abc", nil)
	if last.path != "/render/abc" {
		t.Errorf("gateway saw %+v", last)
	}
	_, body := get(t, srv.URL+"/", nil)
	_ = body
}

func TestDefaultScriptURLFollowsPrefix(t *testing.T) {
	srv := newProxy(t, serveHTML("text/html", "<body></body>"), func(c *Config) { c.Prefix = "/_pdf" })
	_, body := get(t, srv.URL+"/", nil)
	if !strings.Contains(string(body), `src="/_pdf/button.js"`) {
		t.Errorf("body = %s", body)
	}
}

func TestWithoutGatewayThePrefixBelongsToOutline(t *testing.T) {
	var hits int
	up := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hits++ })
	srv := newProxy(t, up, nil)
	get(t, srv.URL+"/pdf/status/abc", nil)
	if hits != 1 {
		t.Errorf("outline hits = %d, want 1", hits)
	}
}

func TestGatewayDownIsAJSONBadGateway(t *testing.T) {
	gw := httptest.NewServer(http.NotFoundHandler())
	url := gw.URL
	gw.Close()
	srv := newProxy(t, http.NotFoundHandler(), func(c *Config) { c.Gateway = url })
	resp, body := get(t, srv.URL+"/pdf/status/abc", nil)
	var parsed struct{ Message, Phase string }
	if resp.StatusCode != http.StatusBadGateway || json.Unmarshal(body, &parsed) != nil || parsed.Message == "" {
		t.Errorf("got %d %q", resp.StatusCode, body)
	}
	if strings.Contains(string(body), "127.0.0.1") {
		t.Errorf("body leaks the gateway address: %q", body)
	}
}
