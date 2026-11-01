package inject

import (
	"strings"
	"testing"
)

func envOf(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestConfigFromEnvDefaults(t *testing.T) {
	cfg, err := ConfigFromEnv(envOf(map[string]string{"OUTPRINT_INJECTOR_UPSTREAM": "http://outline.example.invalid:3000"}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Listen != ":8090" || cfg.Prefix != "/pdf" || cfg.ScriptURL != "" || cfg.Gateway != "" || cfg.AllowScriptSelf {
		t.Errorf("defaults = %+v", cfg)
	}
}

func TestConfigFromEnvReadsEverything(t *testing.T) {
	cfg, err := ConfigFromEnv(envOf(map[string]string{
		"OUTPRINT_INJECTOR_UPSTREAM":       "http://outline.example.invalid:3000",
		"OUTPRINT_INJECTOR_LISTEN":         "127.0.0.1:9000",
		"OUTPRINT_INJECTOR_GATEWAY_URL":    "http://gateway.example.invalid:8080",
		"OUTPRINT_INJECTOR_GATEWAY_PREFIX": "/_pdf",
		"OUTPRINT_INJECTOR_SCRIPT_URL":     "/x.js",
		"OUTPRINT_INJECTOR_CSP_ALLOW_SELF": "true",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Listen != "127.0.0.1:9000" || cfg.Gateway != "http://gateway.example.invalid:8080" ||
		cfg.Prefix != "/_pdf" || cfg.ScriptURL != "/x.js" || !cfg.AllowScriptSelf {
		t.Errorf("cfg = %+v", cfg)
	}
}

func TestConfigFromEnvErrors(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want string
	}{
		{"missing upstream", map[string]string{}, "OUTPRINT_INJECTOR_UPSTREAM"},
		{"bad bool", map[string]string{"OUTPRINT_INJECTOR_UPSTREAM": "http://a.invalid", "OUTPRINT_INJECTOR_CSP_ALLOW_SELF": "maybe"}, "OUTPRINT_INJECTOR_CSP_ALLOW_SELF"},
	}
	for _, tc := range tests {
		_, err := ConfigFromEnv(envOf(tc.env))
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want mention of %s", tc.name, err, tc.want)
		}
	}
}

func TestNewRejectsBadConfig(t *testing.T) {
	ok := Config{Upstream: "http://outline.example.invalid"}
	tests := []struct {
		name   string
		mutate func(*Config)
	}{
		{"empty upstream", func(c *Config) { c.Upstream = "" }},
		{"upstream without scheme", func(c *Config) { c.Upstream = "outline.example.invalid" }},
		{"upstream scheme", func(c *Config) { c.Upstream = "ftp://outline.example.invalid" }},
		{"upstream with credentials", func(c *Config) { c.Upstream = "http://u:p@outline.example.invalid" }},
		{"gateway scheme", func(c *Config) { c.Gateway = "file:///etc" }},
		{"prefix without slash", func(c *Config) { c.Prefix = "pdf" }},
		{"prefix trailing slash", func(c *Config) { c.Prefix = "/pdf/" }},
		{"prefix is root", func(c *Config) { c.Prefix = "/" }},
		{"prefix takes the health path", func(c *Config) { c.Prefix = "/_injector" }},
		{"prefix dots", func(c *Config) { c.Prefix = "/a/../b" }},
	}
	for _, tc := range tests {
		cfg := ok
		cfg.Gateway = "http://gateway.example.invalid"
		tc.mutate(&cfg)
		if _, err := New(cfg); err == nil {
			t.Errorf("%s: New accepted %+v", tc.name, cfg)
		}
	}
}
