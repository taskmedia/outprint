package config

import (
	"strings"
	"testing"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

const validService = `
templatesRoot: /srv/templates
configurationsRoot: /srv/configurations
bindings:
  - template: neutral
    configuration: example
  - template: neutral
    configuration: other
defaultBinding:
  template: neutral
  configuration: example
`

func TestParseValid(t *testing.T) {
	s, errs := Parse([]byte(validService), env(nil))
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if s.TemplatesRoot != "/srv/templates" || s.ConfigurationsRoot != "/srv/configurations" {
		t.Fatalf("roots: %+v", s)
	}
	if len(s.Bindings) != 2 || s.DefaultBinding == nil || s.DefaultBinding.Configuration != "example" {
		t.Fatalf("bindings: %+v", s)
	}
}

func TestEnvOverridesRoots(t *testing.T) {
	s, errs := Parse([]byte(validService), env(map[string]string{
		EnvTemplatesRoot:      "/env/t",
		EnvConfigurationsRoot: "/env/c",
	}))
	if len(errs) != 0 {
		t.Fatal(errs)
	}
	if s.TemplatesRoot != "/env/t" || s.ConfigurationsRoot != "/env/c" {
		t.Fatalf("env must win: %+v", s)
	}
}

func TestEnvSuppliesMissingRoots(t *testing.T) {
	_, errs := Parse([]byte("bindings:\n  - {template: a, configuration: b}\n"), env(map[string]string{
		EnvTemplatesRoot: "/t", EnvConfigurationsRoot: "/c",
	}))
	if len(errs) != 0 {
		t.Fatal(errs)
	}
}

func TestParseRejects(t *testing.T) {
	cases := map[string]struct {
		yaml string
		want string
	}{
		"unknown key":       {"templatesRoot: /t\nconfigurationsRoot: /c\nbogus: 1\nbindings:\n  - {template: a, configuration: b}\n", "bogus"},
		"unknown binding":   {"templatesRoot: /t\nconfigurationsRoot: /c\nbindings:\n  - {template: a, configuration: b, extra: 1}\n", "extra"},
		"no templates":      {"configurationsRoot: /c\nbindings:\n  - {template: a, configuration: b}\n", "templatesRoot"},
		"no configs":        {"templatesRoot: /t\nbindings:\n  - {template: a, configuration: b}\n", "configurationsRoot"},
		"no bindings":       {"templatesRoot: /t\nconfigurationsRoot: /c\n", "bindings"},
		"bad template":      {"templatesRoot: /t\nconfigurationsRoot: /c\nbindings:\n  - {template: ../x, configuration: b}\n", "invalid template name"},
		"empty config":      {"templatesRoot: /t\nconfigurationsRoot: /c\nbindings:\n  - {template: a}\n", "configuration"},
		"duplicate":         {"templatesRoot: /t\nconfigurationsRoot: /c\nbindings:\n  - {template: a, configuration: b}\n  - {template: a, configuration: b}\n", "duplicate"},
		"default not bound": {"templatesRoot: /t\nconfigurationsRoot: /c\nbindings:\n  - {template: a, configuration: b}\ndefaultBinding: {template: a, configuration: c}\n", "defaultBinding"},
		"not yaml":          {"templatesRoot: [", "yaml"},
		"empty":             {"", "empty"},
	}
	for name, tc := range cases {
		_, errs := Parse([]byte(tc.yaml), env(nil))
		if len(errs) == 0 {
			t.Errorf("%s: expected error", name)
			continue
		}
		if !strings.Contains(Errors(errs).Error(), tc.want) {
			t.Errorf("%s: errors %q do not mention %q", name, Errors(errs), tc.want)
		}
	}
}

func TestParseCollectsAllProblems(t *testing.T) {
	_, errs := Parse([]byte("bindings:\n  - {template: A, configuration: b}\n"), env(nil))
	if len(errs) < 3 {
		t.Fatalf("want roots and name problems collected, got %v", errs)
	}
}

func TestImageHosts(t *testing.T) {
	base := "templatesRoot: /t\nconfigurationsRoot: /c\nbindings:\n  - {template: a, configuration: b}\n"
	s, errs := Parse([]byte(base+"imageHosts: [files.example.invalid, storage.example.invalid:9000, 10.0.0.5]\n"), env(nil))
	if len(errs) != 0 || len(s.ImageHosts) != 3 {
		t.Fatalf("hosts %v, errors %v", s.ImageHosts, errs)
	}
	for _, bad := range []string{"https://files.example.invalid", "files.example.invalid/path", "*.example.invalid",
		"User@files.example.invalid", "Files.Example.Invalid", "files.example.invalid:", "[::1]", "-x", ""} {
		_, errs := Parse([]byte(base+"imageHosts: ['"+bad+"']\n"), env(nil))
		if len(errs) == 0 {
			t.Errorf("%q accepted", bad)
		}
	}
	if _, errs := Parse([]byte(base+"imageHost: [x]\n"), env(nil)); len(errs) == 0 {
		t.Error("a misspelled key must be rejected")
	}
}
