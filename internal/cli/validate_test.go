package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

const testManifest = `manifest_version: 1
fields:
  name: {type: string}
  orphan: {type: string}
document_types:
  letter:
    template: letter.latex
    fields: [name, orphan]
`

func installation(t *testing.T) (templates, configs string) {
	t.Helper()
	dir := t.TempDir()
	write(t, filepath.Join(dir, "t/plain/manifest.yaml"), testManifest)
	write(t, filepath.Join(dir, "t/plain/letter.latex"), "$name$ $body$\n")
	write(t, filepath.Join(dir, "c/sample/configuration.yaml"), "manifest_version: 1\nfields:\n  name: Sample\n")
	return filepath.Join(dir, "t"), filepath.Join(dir, "c")
}

func validate(t *testing.T, env map[string]string, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errOut strings.Builder
	code = runValidate(args, func(k string) string { return env[k] }, &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestValidateWarningsOnlyExitsZero(t *testing.T) {
	tpl, cfg := installation(t)
	code, out, errOut := validate(t, nil, "--templates-root", tpl, "--configs-root", cfg)
	if code != 0 {
		t.Fatalf("code %d\n%s%s", code, out, errOut)
	}
	if !strings.Contains(out, "warning") || !strings.Contains(out, "orphan") || !strings.Contains(out, "template/plain") {
		t.Fatalf("output:\n%s", out)
	}
}

func TestValidateErrorsExitNonZero(t *testing.T) {
	tpl, cfg := installation(t)
	write(t, filepath.Join(tpl, "plain/letter.latex"), "$name$ $ghost$\n\\directlua{x}\n")
	code, out, _ := validate(t, nil, "--templates-root", tpl, "--configs-root", cfg)
	if code != 1 {
		t.Fatalf("code %d\n%s", code, out)
	}
	for _, want := range []string{"error", "ghost", "letter.latex:1", "directlua", "letter.latex:2"} {
		if !strings.Contains(out, want) {
			t.Errorf("%q missing from:\n%s", want, out)
		}
	}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if strings.HasPrefix(line, "error") && strings.Count(line, "\n") != 0 {
			t.Errorf("finding spans lines: %q", line)
		}
	}
}

func TestValidateOneFindingPerLine(t *testing.T) {
	tpl, cfg := installation(t)
	_, out, _ := validate(t, nil, "--templates-root", tpl, "--configs-root", cfg)
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) < 2 {
		t.Fatalf("output:\n%s", out)
	}
	if !strings.HasPrefix(lines[len(lines)-1], "validate:") {
		t.Fatalf("last line should be a summary: %q", lines[len(lines)-1])
	}
}

func TestValidateRootsFromEnvironment(t *testing.T) {
	tpl, cfg := installation(t)
	code, _, _ := validate(t, map[string]string{
		"OUTPRINT_TEMPLATES_ROOT": tpl, "OUTPRINT_CONFIGURATIONS_ROOT": cfg,
	})
	if code != 0 {
		t.Fatalf("code %d", code)
	}
}

func TestValidatePairCheck(t *testing.T) {
	tpl, cfg := installation(t)
	write(t, filepath.Join(cfg, "sample/configuration.yaml"), "manifest_version: 1\nfields:\n  name: [a]\n")
	code, out, _ := validate(t, nil, "--templates-root", tpl, "--configs-root", cfg, "--template", "plain", "--config", "sample")
	if code != 1 || !strings.Contains(out, "binding/plain/sample") {
		t.Fatalf("code %d\n%s", code, out)
	}
}

func TestValidateServiceConfig(t *testing.T) {
	tpl, cfg := installation(t)
	dir := t.TempDir()
	svc := filepath.Join(dir, "service.yaml")
	write(t, svc, "templatesRoot: "+tpl+"\nconfigurationsRoot: "+cfg+"\nbindings:\n  - {template: plain, configuration: sample}\n  - {template: plain, configuration: nope}\n")
	code, out, _ := validate(t, nil, "--service-config", svc)
	if code != 1 || !strings.Contains(out, `missing configuration "nope"`) {
		t.Fatalf("code %d\n%s", code, out)
	}

	write(t, svc, "templatesRoot: "+tpl+"\nconfigurationsRoot: "+cfg+"\nbindings:\n  - {template: plain, configuration: sample}\n")
	write(t, filepath.Join(tpl, "extra/manifest.yaml"), testManifest)
	write(t, filepath.Join(tpl, "extra/letter.latex"), "$name$ $orphan$\n")
	code, out, _ = validate(t, map[string]string{"OUTPRINT_SERVICE_CONFIG": svc})
	if code != 0 || !strings.Contains(out, "not on the allow-list") {
		t.Fatalf("code %d\n%s", code, out)
	}
}

func TestValidateServiceConfigProblemsAreFindings(t *testing.T) {
	svc := filepath.Join(t.TempDir(), "service.yaml")
	write(t, svc, "bogus: 1\n")
	code, out, _ := validate(t, nil, "--service-config", svc)
	if code != 1 || !strings.Contains(out, "error") || !strings.Contains(out, "bogus") {
		t.Fatalf("code %d\n%s", code, out)
	}
}

func TestValidateUsageErrors(t *testing.T) {
	tpl, cfg := installation(t)
	svc := filepath.Join(t.TempDir(), "service.yaml")
	cases := map[string][]string{
		"unknown flag":              {"--bogus"},
		"positional":                {"extra"},
		"service config with roots": {"--service-config", svc, "--templates-root", tpl},
		"service config with names": {"--service-config", svc, "--template", "plain"},
		"unreadable flags combo":    {"--templates-root", tpl, "--configs-root", cfg, "--bogus"},
	}
	for name, args := range cases {
		if code, _, _ := validate(t, nil, args...); code != 2 {
			t.Errorf("%s: code %d, want 2", name, code)
		}
	}
}

func TestExplicitSelectionBeatsServiceConfigFromEnvironment(t *testing.T) {
	tpl, cfg := installation(t)
	env := map[string]string{"OUTPRINT_SERVICE_CONFIG": filepath.Join(t.TempDir(), "absent.yaml")}
	if code, out, _ := validate(t, env); code != 1 {
		t.Fatalf("the environment service configuration should be used (and is absent): code %d\n%s", code, out)
	}
	if code, out, errOut := validate(t, env, "--templates-root", tpl, "--configs-root", cfg); code != 0 {
		t.Fatalf("explicit roots should win: code %d\n%s%s", code, out, errOut)
	}
}
