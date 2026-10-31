package worker

import (
	"slices"
	"strings"
	"testing"
)

func TestBuildEnvIsRebuiltFromScratch(t *testing.T) {
	host := map[string]string{
		"PATH":              "/home/someone/bin",
		"HOME":              "/home/someone",
		"SECRET_TOKEN":      "hunter2",
		"TEXMFHOME":         "/elsewhere",
		"SOURCE_DATE_EPOCH": "1700000000",
		"FORCE_SOURCE_DATE": "1",
	}
	lookup := func(k string) (string, bool) { v, ok := host[k]; return v, ok }

	env := buildEnv("/job", lookup)

	joined := strings.Join(env, "\n")
	for _, forbidden := range []string{"hunter2", "/home/someone", "/elsewhere"} {
		if strings.Contains(joined, forbidden) {
			t.Errorf("environment leaks %q", forbidden)
		}
	}
	for _, want := range []string{
		"HOME=/job/home",
		"TEXMFVAR=/job/texmf-var",
		"shell_escape=f",
		"SOURCE_DATE_EPOCH=1700000000",
		"FORCE_SOURCE_DATE=1",
	} {
		if !slices.Contains(env, want) {
			t.Errorf("environment lacks %q", want)
		}
	}
}

func TestBuildEnvOmitsUnsetSourceDate(t *testing.T) {
	env := buildEnv("/job", func(string) (string, bool) { return "", false })
	for _, e := range env {
		if strings.HasPrefix(e, "SOURCE_DATE_EPOCH") || strings.HasPrefix(e, "FORCE_SOURCE_DATE") {
			t.Errorf("unexpected %q", e)
		}
	}
}

func TestPandocArgsAreSandboxedAndCarryNoValues(t *testing.T) {
	args := pandocArgs("/tpl/letter.latex", "/job/meta.yaml", "/job/input.md", "/job/doc.tex")

	if !slices.Contains(args, "--sandbox") {
		t.Error("missing --sandbox")
	}
	reader := ""
	for i, a := range args {
		if a == "-f" {
			reader = args[i+1]
		}
		if a == "-V" || a == "--variable" || a == "-M" || a == "--metadata" {
			t.Errorf("document values must not travel as %s", a)
		}
	}
	for _, ext := range []string{"-raw_tex", "-raw_attribute", "-tex_math_dollars", "-yaml_metadata_block"} {
		if !strings.Contains(reader, ext) {
			t.Errorf("reader %q lacks %s", reader, ext)
		}
	}
}

func TestLualatexArgsNeverEnableShellEscape(t *testing.T) {
	args := lualatexArgs("doc.tex")
	if slices.Contains(args, "--shell-escape") || slices.Contains(args, "-shell-escape") {
		t.Fatal("shell escape must never be enabled")
	}
	if !slices.Contains(args, "-no-shell-escape") {
		t.Fatal("expected explicit -no-shell-escape")
	}
}

func TestNeedsRerun(t *testing.T) {
	if !needsRerun("LaTeX Warning: Label(s) may have changed. Rerun to get cross-references right.") {
		t.Error("expected rerun")
	}
	if needsRerun("Output written on doc.pdf") {
		t.Error("unexpected rerun")
	}
}

func TestErrorNamesPhaseAndTailsOutput(t *testing.T) {
	long := strings.Repeat("x", 5000) + "THE END"
	err := &Error{Phase: "lualatex", Err: errString("exit status 1"), Output: long}

	msg := err.Error()
	if !strings.Contains(msg, "lualatex") || !strings.Contains(msg, "THE END") {
		t.Fatalf("unexpected message %q", msg)
	}
	if len(msg) > 3000 {
		t.Fatalf("message not truncated: %d bytes", len(msg))
	}
}

type errString string

func (e errString) Error() string { return string(e) }
