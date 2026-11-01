package svgcorpus

import (
	"testing"

	"github.com/taskmedia/outprint/internal/images"
)

// TestPrecheckAnswersAsTheCorpusSays is the Gateway half of the SVG attack
// corpus: every hostile file the pre-check should refuse is refused with the
// recorded kind, and the ones it lets through are exactly the ones the
// Worker sandbox tests have to contain.
func TestPrecheckAnswersAsTheCorpusSays(t *testing.T) {
	attacks := Attacks(Target{CanaryPath: "/nonexistent/canary.txt", URL: "http://127.0.0.1:9"})
	if len(attacks) < 30 {
		t.Fatalf("the corpus has %d attacks, expected at least 30", len(attacks))
	}
	accepted := 0
	for _, a := range attacks {
		t.Run(a.Name, func(t *testing.T) {
			c, rej := images.Canonicalise(a.SVG)
			switch {
			case a.Precheck == Accepted && rej != nil:
				t.Fatalf("the pre-check rejected it: %v", rej)
			case a.Precheck == Accepted:
				accepted++
				if c.Ext != "svg" {
					t.Errorf("ext = %q", c.Ext)
				}
			case rej == nil:
				t.Fatalf("the pre-check accepted an attack that it must reject (%s)", a.Precheck)
			case rej.Kind != a.Precheck:
				t.Errorf("kind = %s, want %s: %v", rej.Kind, a.Precheck, rej)
			}
		})
	}
	if accepted < 5 {
		t.Errorf("only %d attacks pass the pre-check; the Worker half of the corpus needs some", accepted)
	}
}
