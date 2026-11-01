package inject

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRunServesAndStops(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<body></body>"))
	}))
	defer up.Close()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Run(ctx, Config{Upstream: up.URL, Listener: ln}) }()

	if err := Healthcheck(context.Background(), "http://"+ln.Addr().String()+"/_injector/healthz"); err != nil {
		t.Fatalf("healthcheck: %v", err)
	}
	_, body := get(t, "http://"+ln.Addr().String()+"/", nil)
	if !strings.Contains(string(body), "data-outline-pdf-injector") {
		t.Errorf("body = %q", body)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not stop")
	}
}

func TestRunRefusesBadConfig(t *testing.T) {
	if err := Run(context.Background(), Config{}); err == nil {
		t.Error("Run accepted an empty config")
	}
}

func TestHealthcheckFailures(t *testing.T) {
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) }))
	defer bad.Close()
	if err := Healthcheck(context.Background(), bad.URL); err == nil {
		t.Error("a 418 passed the healthcheck")
	}
	if err := Healthcheck(context.Background(), "http://127.0.0.1:1/"); err == nil {
		t.Error("an unreachable address passed the healthcheck")
	}
}

func TestMain_(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := Main(nil, envOf(nil), &out, &errOut); code != 2 || !strings.Contains(errOut.String(), "INJECTOR_UPSTREAM") {
		t.Errorf("no env: code %d stderr %q", code, errOut.String())
	}
	ok := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer ok.Close()
	errOut.Reset()
	if code := Main([]string{"healthcheck", ok.URL}, envOf(nil), &out, &errOut); code != 0 {
		t.Errorf("healthcheck: code %d stderr %q", code, errOut.String())
	}
	if code := Main([]string{"healthcheck"}, envOf(nil), &out, &errOut); code != 2 {
		t.Errorf("healthcheck without url: code %d", code)
	}
	if code := Main([]string{"healthcheck", "http://127.0.0.1:1/"}, envOf(nil), &out, &errOut); code != 1 {
		t.Errorf("failing healthcheck: code %d", code)
	}
	if code := Main([]string{"bogus"}, envOf(nil), &out, &errOut); code != 2 {
		t.Errorf("unknown command: code %d", code)
	}
}
