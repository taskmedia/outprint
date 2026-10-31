package worker

import "testing"

func TestNeedsRerun(t *testing.T) {
	if !needsRerun("LaTeX Warning: Label(s) may have changed. Rerun to get cross-references right.") {
		t.Error("expected rerun")
	}
	if !needsRerun("Package rerunfilecheck Warning: File `doc.out' has changed.\nRerun to get outlines right") {
		t.Error("expected rerun for outlines")
	}
	if needsRerun("Output written on doc.pdf") {
		t.Error("unexpected rerun")
	}
}

func TestFirstTeXErrorLineIsTheFirstBangLine(t *testing.T) {
	log := "This is LuaHBTeX\n(./doc.tex\n! Undefined control sequence.\nl.12 \\foo\n\n! Emergency stop.\n"
	if got := firstTeXErrorLine(log); got != "! Undefined control sequence." {
		t.Fatalf("got %q", got)
	}
	if got := firstTeXErrorLine("all fine\n"); got != "" {
		t.Fatalf("got %q for a clean log", got)
	}
}

func TestPagesFromTeXLog(t *testing.T) {
	cases := map[string]struct {
		log   string
		pages int
		ok    bool
	}{
		"several": {"x\nOutput written on doc.pdf (12 pages, 34567 bytes).\nTranscript", 12, true},
		"single":  {"Output written on doc.pdf (1 page, 999 bytes).", 1, true},
		"none":    {"! Emergency stop.", 0, false},
		"other":   {"Output written on other.pdf (3 pages, 9 bytes).", 0, false},
	}
	for name, c := range cases {
		pages, ok := pagesFromTeXLog(c.log)
		if pages != c.pages || ok != c.ok {
			t.Errorf("%s: got %d, %v; want %d, %v", name, pages, ok, c.pages, c.ok)
		}
	}
}
