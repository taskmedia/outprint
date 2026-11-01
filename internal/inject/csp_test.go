package inject

import (
	"net/http"
	"strings"
	"testing"
)

func TestAllowSelfScript(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"adds self to script-src", "default-src 'self'; script-src 'unsafe-inline' https://cdn.example.invalid", "default-src 'self'; script-src 'unsafe-inline' https://cdn.example.invalid 'self'"},
		{"already self", "script-src 'self'", "script-src 'self'"},
		{"wildcard", "script-src *", "script-src *"},
		{"none is replaced", "script-src 'none'", "script-src 'self'"},
		{"script-src-elem too", "script-src 'self'; script-src-elem https://cdn.example.invalid", "script-src 'self'; script-src-elem https://cdn.example.invalid 'self'"},
		{"default-src fallback", "default-src https://cdn.example.invalid; img-src *", "default-src https://cdn.example.invalid; img-src *; script-src https://cdn.example.invalid 'self'"},
		{"default-src none", "default-src 'none'", "default-src 'none'; script-src 'self'"},
		{"default-src self", "default-src 'self'", "default-src 'self'"},
		{"no script restriction", "img-src *", "img-src *"},
		{"case and spaces", " Script-Src  'unsafe-inline' ;img-src *", "Script-Src 'unsafe-inline' 'self'; img-src *"},
		{"empty", "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := allowSelfScript(tc.in); got != tc.want {
				t.Errorf("allowSelfScript(%q)\n got %q\nwant %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestCSPIsRewrittenOnlyWhenAsked(t *testing.T) {
	const policy = "script-src 'unsafe-inline'"
	up := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Header().Add("Content-Security-Policy", policy)
		w.Header().Add("Content-Security-Policy", "default-src 'none'")
		w.Header().Set("Content-Security-Policy-Report-Only", policy)
		_, _ = w.Write([]byte("<body></body>"))
	})

	off := newProxy(t, up, nil)
	resp, _ := get(t, off.URL+"/", nil)
	if got := resp.Header.Values("Content-Security-Policy"); got[0] != policy {
		t.Errorf("without the option: %v", got)
	}

	on := newProxy(t, up, func(c *Config) { c.AllowScriptSelf = true })
	resp, _ = get(t, on.URL+"/", nil)
	got := resp.Header.Values("Content-Security-Policy")
	if len(got) != 2 || got[0] != "script-src 'unsafe-inline' 'self'" || !strings.Contains(got[1], "script-src 'self'") {
		t.Errorf("with the option: %v", got)
	}
	if resp.Header.Get("Content-Security-Policy-Report-Only") != policy {
		t.Errorf("report-only policy changed: %q", resp.Header.Get("Content-Security-Policy-Report-Only"))
	}
}
