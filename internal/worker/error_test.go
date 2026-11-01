package worker

import (
	"errors"
	"strings"
	"testing"
)

func TestErrorNamesPhaseKindAndJobOnly(t *testing.T) {
	err := &Error{Phase: PhaseTeX, Kind: KindTeXError, JobID: "abc123",
		Err: errors.New("lualatex exited with code 1"), FirstTeXError: "! Secret text in the log."}

	msg := err.Error()
	for _, want := range []string{"tex", "tex_error", "abc123", "exited with code 1"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message %q lacks %q", msg, want)
		}
	}
	if strings.Contains(msg, "Secret text") {
		t.Errorf("message %q carries the TeX error line", msg)
	}
}

func TestFilterKindReadsOnlyTheKind(t *testing.T) {
	cases := []struct {
		output string
		want   Kind
		ok     bool
	}{
		{"Error running filter x.lua:\noutprint-filter:raw_tex: raw TeX node reached the filter stage\nstack", KindRawTeX, true},
		{"outprint-filter:math: math node found", KindMath, true},
		{"outprint-filter:image_path: image does not refer", KindImagePath, true},
		{"outprint-filter:surprise: ?", KindInternal, true},
		{"pandoc: something else failed", "", false},
	}
	for _, c := range cases {
		got, ok := filterKind(c.output)
		if got != c.want || ok != c.ok {
			t.Errorf("filterKind(%q) = %q, %v; want %q, %v", c.output, got, ok, c.want, c.ok)
		}
	}
}
