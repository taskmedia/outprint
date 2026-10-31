package worker

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
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

	env := buildEnv("/job", "/job/texmf-cnf:/sys/cnf", lookup)

	joined := strings.Join(env, "\n")
	for _, forbidden := range []string{"hunter2", "/home/someone", "/elsewhere"} {
		if strings.Contains(joined, forbidden) {
			t.Errorf("environment leaks %q", forbidden)
		}
	}
	for _, want := range []string{
		"PATH=/usr/local/bin:/usr/bin:/bin",
		"LC_ALL=C.UTF-8",
		"HOME=/job/home",
		"TMPDIR=/job",
		"TEXMFVAR=/job/texmf-var",
		"TEXMFCACHE=/job/texmf-var",
		"TEXMFCONFIG=/job/texmf-config",
		"TEXMFCNF=/job/texmf-cnf:/sys/cnf",
		"SOURCE_DATE_EPOCH=1700000000",
		"FORCE_SOURCE_DATE=1",
	} {
		if !slices.Contains(env, want) {
			t.Errorf("environment lacks %q", want)
		}
	}
	if len(env) != 10 {
		t.Errorf("environment has %d entries, want exactly the 10 listed: %v", len(env), env)
	}
}

func TestBuildEnvOmitsUnsetSourceDate(t *testing.T) {
	env := buildEnv("/job", "/cnf", func(string) (string, bool) { return "", false })
	for _, e := range env {
		if strings.HasPrefix(e, "SOURCE_DATE_EPOCH") || strings.HasPrefix(e, "FORCE_SOURCE_DATE") {
			t.Errorf("unexpected %q", e)
		}
	}
}

func TestPandocArgsAreSandboxedAndCarryNoValues(t *testing.T) {
	filters := []string{"/job/filters/sanitize.lua", "/job/filters/layout.lua"}
	args := pandocArgs("/tpl/letter.latex", "/job/meta.yaml", "/job/input.md", "/job/doc.tex", filters)

	if !slices.Contains(args, "--sandbox") {
		t.Error("missing --sandbox")
	}
	reader := ""
	var luaFilters []string
	for i, a := range args {
		switch a {
		case "-f":
			reader = args[i+1]
		case "--lua-filter":
			luaFilters = append(luaFilters, args[i+1])
		case "-V", "--variable", "-M", "--metadata":
			t.Errorf("document values must not travel as %s", a)
		}
	}
	for _, ext := range []string{"-raw_tex", "-raw_attribute", "-tex_math_dollars", "-yaml_metadata_block"} {
		if !strings.Contains(reader, ext) {
			t.Errorf("reader %q lacks %s", reader, ext)
		}
	}
	if strings.Contains(reader, "-raw_html") {
		t.Errorf("reader %q must keep raw HTML for <br>", reader)
	}
	if !slices.Equal(luaFilters, filters) {
		t.Errorf("filters = %v, want %v in order", luaFilters, filters)
	}
}

func TestLualatexArgsNeverWeakenTheEngine(t *testing.T) {
	args := lualatexArgs("doc.tex")
	for _, forbidden := range []string{"--shell-escape", "-shell-escape", "--socket", "-socket", "--safer", "-safer", "--no-socket"} {
		if slices.Contains(args, forbidden) {
			t.Errorf("lualatex must never get %s", forbidden)
		}
	}
	for _, want := range []string{"-no-shell-escape", "-halt-on-error", "-interaction=nonstopmode"} {
		if !slices.Contains(args, want) {
			t.Errorf("missing %s", want)
		}
	}
}

func TestEmbeddedFiltersExistInRunOrder(t *testing.T) {
	for _, name := range filterFiles {
		data, err := filtersFS.ReadFile("filters/" + name)
		if err != nil || len(data) == 0 {
			t.Errorf("filter %s is not embedded: %v", name, err)
		}
	}
	if filterFiles[0] != "sanitize.lua" {
		t.Errorf("sanitize.lua must run first, got %v", filterFiles)
	}
}

func finishedJob(t *testing.T, job Job, log string, pdfBytes int) *pipeline {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "doc.pdf"), make([]byte, pdfBytes), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "doc.log"), []byte(log), 0o600); err != nil {
		t.Fatal(err)
	}
	return &pipeline{job: job, id: "0123456789abcdef", dir: dir, log: slog.New(slog.NewTextHandler(io.Discard, nil))}
}

func TestCheckOutput(t *testing.T) {
	const log = "Output written on doc.pdf (30 pages, 2000 bytes).\n"
	cases := map[string]struct {
		job  Job
		log  string
		size int
		kind Kind
	}{
		"within limits": {Job{MaxPages: 30, MaxPDFBytes: 2000}, log, 2000, ""},
		"too many":      {Job{MaxPages: 29}, log, 2000, KindPagesExceeded},
		"too big":       {Job{MaxPDFBytes: 1999}, log, 2000, KindPDFTooLarge},
		"no page count": {Job{}, "! Emergency stop.\n", 10, KindNoOutput},
		"defaults":      {Job{}, log, 2000, ""},
	}
	for name, c := range cases {
		err := finishedJob(t, c.job, c.log, c.size).checkOutput()
		var failure *Error
		switch {
		case c.kind == "" && err != nil:
			t.Errorf("%s: unexpected %v", name, err)
		case c.kind != "" && (!errors.As(err, &failure) || failure.Kind != c.kind):
			t.Errorf("%s: err = %v, want kind %s", name, err, c.kind)
		}
	}
}

func TestPagesExceededNamesNoDocumentContent(t *testing.T) {
	err := finishedJob(t, Job{MaxPages: 1}, "Output written on doc.pdf (2 pages, 9 bytes).\n", 9).checkOutput()
	var failure *Error
	if !errors.As(err, &failure) || failure.Phase != PhaseOutput || failure.JobID == "" {
		t.Fatalf("err = %#v", err)
	}
}

func TestFailWithNoSpaceIsAResourceError(t *testing.T) {
	p := &pipeline{id: "x", log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	err := p.fail(PhaseSetup, KindInternal, fmt.Errorf("write job file: %w", syscall.ENOSPC))
	var failure *Error
	if !errors.As(err, &failure) || failure.Kind != KindResource {
		t.Errorf("err = %v, want kind resource", err)
	}
}
