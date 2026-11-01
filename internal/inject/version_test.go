package inject

import (
	"bytes"
	"testing"

	"github.com/taskmedia/outprint/internal/version"
)

func TestMainVersionPrintsVersionLine(t *testing.T) {
	oldV, oldC := version.Version, version.Commit
	t.Cleanup(func() { version.Version, version.Commit = oldV, oldC })
	version.Version, version.Commit = "9.8.7", ""
	for _, arg := range []string{"version", "--version"} {
		var stdout, stderr bytes.Buffer
		if code := Main([]string{arg}, func(string) string { return "" }, &stdout, &stderr); code != 0 {
			t.Fatalf("%s: code %d, stderr %q", arg, code, stderr.String())
		}
		if got, want := stdout.String(), "outprint-injector 9.8.7\n"; got != want {
			t.Errorf("%s: stdout = %q, want %q", arg, got, want)
		}
	}
}
