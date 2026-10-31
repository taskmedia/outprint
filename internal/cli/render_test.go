package cli

import (
	"strings"
	"testing"
)

func noEnv(string) string { return "" }

func TestParseRenderOptions(t *testing.T) {
	opts, err := parseRender([]string{
		"--template", "neutral", "--config", "example", "--type", "letter",
		"--templates-root", "/t", "--configs-root", "/c", "--fields", "f.yaml",
		"-o", "out.pdf", "in.md",
	}, noEnv)
	if err != nil {
		t.Fatal(err)
	}
	want := renderOptions{
		Template: "neutral", Config: "example", DocumentType: "letter",
		TemplatesRoot: "/t", ConfigsRoot: "/c", FieldsFile: "f.yaml",
		Output: "out.pdf", Input: "in.md",
	}
	if opts != want {
		t.Fatalf("got %+v, want %+v", opts, want)
	}
}

func TestRootsDefaultAndEnvPrecedence(t *testing.T) {
	args := []string{"--template", "a", "--config", "b", "-o", "o.pdf", "in.md"}

	opts, err := parseRender(args, noEnv)
	if err != nil {
		t.Fatal(err)
	}
	if opts.TemplatesRoot != "./templates" || opts.ConfigsRoot != "./configurations" {
		t.Fatalf("defaults: %+v", opts)
	}

	env := func(k string) string {
		return map[string]string{"RENDERER_TEMPLATES_ROOT": "/env/t", "RENDERER_CONFIGURATIONS_ROOT": "/env/c"}[k]
	}
	opts, err = parseRender(args, env)
	if err != nil {
		t.Fatal(err)
	}
	if opts.TemplatesRoot != "/env/t" || opts.ConfigsRoot != "/env/c" {
		t.Fatalf("env: %+v", opts)
	}

	opts, err = parseRender(append([]string{"--templates-root", "/flag/t"}, args...), env)
	if err != nil {
		t.Fatal(err)
	}
	if opts.TemplatesRoot != "/flag/t" || opts.ConfigsRoot != "/env/c" {
		t.Fatalf("flag over env: %+v", opts)
	}
}

func TestParseRenderRejectsBadUsage(t *testing.T) {
	cases := map[string][]string{
		"missing template": {"--config", "b", "-o", "o.pdf", "in.md"},
		"missing config":   {"--template", "a", "-o", "o.pdf", "in.md"},
		"missing output":   {"--template", "a", "--config", "b", "in.md"},
		"missing input":    {"--template", "a", "--config", "b", "-o", "o.pdf"},
		"extra input":      {"--template", "a", "--config", "b", "-o", "o.pdf", "in.md", "more.md"},
		"unknown flag":     {"--template", "a", "--config", "b", "-o", "o.pdf", "--bogus", "in.md"},
	}
	for name, args := range cases {
		if _, err := parseRender(args, noEnv); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

func TestRunUnknownSubcommand(t *testing.T) {
	var stderr strings.Builder
	if code := Run([]string{"frobnicate"}, &stderr); code != 2 {
		t.Fatalf("exit code %d", code)
	}
	if !strings.Contains(stderr.String(), "usage") {
		t.Fatalf("stderr %q", stderr.String())
	}
	stderr.Reset()
	if code := Run(nil, &stderr); code != 2 {
		t.Fatalf("exit code %d", code)
	}
}

func TestResolveDocumentType(t *testing.T) {
	if got, err := resolveDocumentType("report", map[string]any{"document_type": "letter"}); err != nil || got != "report" {
		t.Fatalf("flag wins: %q %v", got, err)
	}
	if got, err := resolveDocumentType("", map[string]any{"document_type": "letter"}); err != nil || got != "letter" {
		t.Fatalf("default: %q %v", got, err)
	}
	if _, err := resolveDocumentType("", nil); err == nil {
		t.Fatal("expected error without flag or default")
	}
}
