package gateway

import (
	"strings"
	"testing"
)

func TestServesTheButtonScript(t *testing.T) {
	e := newEnv(t, true)
	resp, body := e.getPath(t, "/button.js", "")
	if resp.StatusCode != 200 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/javascript; charset=utf-8" {
		t.Errorf("Content-Type = %q", ct)
	}
	if resp.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Error("missing nosniff")
	}
	if cc := resp.Header.Get("Cache-Control"); !strings.Contains(cc, "max-age=300") || strings.Contains(cc, "immutable") {
		t.Errorf("Cache-Control = %q, want a short max-age", cc)
	}
	if !strings.Contains(string(body), "olp-pdf-button") {
		t.Error("body is not the button script")
	}
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("CORS header %q on the script; the Gateway is same-origin only", got)
	}
	if len(e.outline.seen) != 0 {
		t.Error("serving the script called Outline")
	}
}

func TestStatusAndRenderTakeTheURLIDOfADocumentPath(t *testing.T) {
	const urlID = "aB3dE5fGh1"
	e := newEnv(t, true)
	e.outline.add(urlID, 200, "T", docText)
	if code, sb := e.status(t, urlID, cookieCanary); code != 200 || !sb.Available {
		t.Errorf("status of a urlId: %d %+v", code, sb)
	}
	if resp, body := e.get(t, urlID, cookieCanary); resp.StatusCode != 200 {
		t.Errorf("render of a urlId: %d %s", resp.StatusCode, body)
	}
}

func TestGatewayAnswersNoCORSPreflight(t *testing.T) {
	e := newEnv(t, true)
	resp, _ := e.getPath(t, "/status/d", cookieCanary)
	if resp.Header.Get("Access-Control-Allow-Origin") != "" || resp.Header.Get("Access-Control-Allow-Credentials") != "" {
		t.Errorf("CORS headers on the status endpoint: %v", resp.Header)
	}
}
