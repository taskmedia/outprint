package cli

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func envMap(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestParseServeDefaultsAndOverrides(t *testing.T) {
	o, err := parseServe("gateway", ":8080", nil, envMap(nil))
	if err != nil || o.Listen != ":8080" {
		t.Errorf("defaults = %+v, %v", o, err)
	}
	o, err = parseServe("worker", ":8081", []string{"--service-config", "x.yaml"}, envMap(map[string]string{"RENDERER_LISTEN": ":9"}))
	if err != nil || o.Listen != ":9" || o.ServiceConfig != "x.yaml" {
		t.Errorf("overrides = %+v, %v", o, err)
	}
	if _, err := parseServe("worker", ":1", []string{"extra"}, envMap(nil)); err == nil {
		t.Error("extra argument accepted")
	}
}

func TestGatewayNeedsOutlineAndWorkerURL(t *testing.T) {
	var stderr bytes.Buffer
	code := runGateway(nil, envMap(map[string]string{"RENDERER_WORKER_URL": "http://w.example.test"}), &stderr)
	if code != 2 || !strings.Contains(stderr.String(), "OUTLINE_URL") {
		t.Errorf("code %d, stderr %q", code, stderr.String())
	}
}

func TestServicesRefuseToStartOnBrokenPackages(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "service.yaml")
	body := "templatesRoot: /nonexistent/t\nconfigurationsRoot: /nonexistent/c\nbindings:\n  - template: neutral\n    configuration: example\n"
	if err := os.WriteFile(cfg, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	env := envMap(map[string]string{
		"RENDERER_SERVICE_CONFIG": cfg, "OUTLINE_URL": "https://o.example.test", "RENDERER_WORKER_URL": "http://w.example.test",
	})
	for name, run := range map[string]func() (int, string){
		"worker": func() (int, string) {
			var b bytes.Buffer
			return runWorker(nil, env, &b), b.String()
		},
		"gateway": func() (int, string) {
			var b bytes.Buffer
			return runGateway(nil, env, &b), b.String()
		},
	} {
		code, out := run()
		if code != 1 || !strings.Contains(out, "refusing to start") {
			t.Errorf("%s: code %d, output %q", name, code, out)
		}
	}
}

func TestHealthcheck(t *testing.T) {
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	defer ok.Close()
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(500) }))
	defer bad.Close()
	var stderr bytes.Buffer
	if code := runHealthcheck([]string{ok.URL}, &stderr); code != 0 {
		t.Errorf("healthy: code %d", code)
	}
	if code := runHealthcheck([]string{bad.URL}, &stderr); code != 1 {
		t.Errorf("unhealthy: code %d", code)
	}
	if code := runHealthcheck([]string{"http://127.0.0.1:1/"}, &stderr); code != 1 {
		t.Errorf("unreachable: code %d", code)
	}
	if code := runHealthcheck(nil, &stderr); code != 2 {
		t.Errorf("usage: code %d", code)
	}
}

func TestServiceLoggerLevel(t *testing.T) {
	var b bytes.Buffer
	serviceLogger(&b, envMap(nil)).Info("hello")
	if !strings.Contains(b.String(), "hello") {
		t.Error("info is not logged by default")
	}
	b.Reset()
	serviceLogger(&b, envMap(map[string]string{"RENDERER_LOG_LEVEL": "error"})).Info("hello")
	if b.Len() != 0 {
		t.Error("RENDERER_LOG_LEVEL=error still logs info")
	}
}

func TestWorkerRefusesAMalformedLimit(t *testing.T) {
	var b strings.Builder
	env := func(k string) string {
		if k == "RENDERER_MAX_PAGES" {
			return "lots"
		}
		return ""
	}
	if code := runWorker(nil, env, &b); code != 2 || !strings.Contains(b.String(), "RENDERER_MAX_PAGES") {
		t.Errorf("code %d, output %q", code, b.String())
	}
}
