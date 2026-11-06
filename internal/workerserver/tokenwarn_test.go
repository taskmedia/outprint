package workerserver

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestWarnIfTokenMissing(t *testing.T) {
	cases := []struct {
		name     string
		token    string
		wantWarn bool
	}{
		{"empty token warns", "", true},
		{"set token is silent", "s3cret", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var buf bytes.Buffer
			WarnIfTokenMissing(slog.New(slog.NewTextHandler(&buf, nil)), c.token)
			got := strings.Contains(buf.String(), "worker token is not set")
			if got != c.wantWarn {
				t.Fatalf("warned=%v want %v; log=%q", got, c.wantWarn, buf.String())
			}
			if strings.Contains(buf.String(), c.token) && c.token != "" {
				t.Fatal("token value leaked into log")
			}
		})
	}
}
