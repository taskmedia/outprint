package gateway

import (
	"bytes"
	"log/slog"
	"testing"
)

func TestWarnCleartextOutline(t *testing.T) {
	cases := map[string]bool{
		"http://outline.example.com":  true,
		"http://10.0.0.5:3000":        true,
		"https://outline.example.com": false,
		"http://localhost:3000":       false,
		"http://127.0.0.1:3000":       false,
		"http://[::1]:3000":           false,
	}
	for u, want := range cases {
		var buf bytes.Buffer
		warnCleartextOutline(slog.New(slog.NewTextHandler(&buf, nil)), u)
		if got := buf.Len() > 0; got != want {
			t.Errorf("%s: warned=%v, want %v", u, got, want)
		}
	}
}
