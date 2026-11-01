package cli

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/taskmedia/outprint/internal/version"
)

func TestRunVersionPrintsVersionLine(t *testing.T) {
	oldV, oldC := version.Version, version.Commit
	t.Cleanup(func() { version.Version, version.Commit = oldV, oldC })
	version.Version, version.Commit = "9.8.7", ""
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"version"}, &stdout, &stderr); code != 0 {
		t.Fatalf("code %d, stderr %q", code, stderr.String())
	}
	if got, want := stdout.String(), "outprint 9.8.7\n"; got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
}

func TestLogStartupRecordsVersion(t *testing.T) {
	oldV := version.Version
	t.Cleanup(func() { version.Version = oldV })
	version.Version = "9.8.7"
	var buf bytes.Buffer
	logStartup(slog.New(slog.NewTextHandler(&buf, nil)), "gateway")
	out := buf.String()
	for _, want := range []string{"level=INFO", "gateway", "version=9.8.7"} {
		if !strings.Contains(out, want) {
			t.Errorf("log %q lacks %q", out, want)
		}
	}
}
