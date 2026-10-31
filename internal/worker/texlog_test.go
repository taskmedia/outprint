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
