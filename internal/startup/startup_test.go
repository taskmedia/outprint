package startup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/taskmedia/outprint/internal/config"
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

const manifest = `manifest_version: 1
fields:
  name: {type: string}
document_types:
  letter:
    template: letter.latex
    fields: [name]
`

// setup writes a valid installation and returns the service config path.
func setup(t *testing.T) (dir, servicePath string) {
	t.Helper()
	dir = t.TempDir()
	write(t, filepath.Join(dir, "templates/plain/manifest.yaml"), manifest)
	write(t, filepath.Join(dir, "templates/plain/letter.latex"), "$name$ $body$\n")
	write(t, filepath.Join(dir, "configurations/sample/configuration.yaml"), "manifest_version: 1\ndefaults:\n  document_type: letter\nfields:\n  name: Sample\n")
	servicePath = filepath.Join(dir, "service.yaml")
	write(t, servicePath, "templatesRoot: "+filepath.Join(dir, "templates")+"\nconfigurationsRoot: "+filepath.Join(dir, "configurations")+"\nbindings:\n  - {template: plain, configuration: sample}\ndefaultBinding: {template: plain, configuration: sample}\n")
	return dir, servicePath
}

func noEnv(string) string { return "" }

func TestRunStartsWithAValidInstallation(t *testing.T) {
	_, path := setup(t)
	res, err := Run(path, noEnv)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := res.Registry.Template("plain"); !ok {
		t.Fatal("template not loaded")
	}
	if b, ok := res.Registry.DefaultBinding(); !ok || b.Configuration != "sample" {
		t.Fatalf("default binding %+v", b)
	}
}

func TestRunReadsPathFromEnvironment(t *testing.T) {
	_, path := setup(t)
	env := func(k string) string {
		if k == config.EnvServiceConfig {
			return path
		}
		return ""
	}
	if _, err := Run("", env); err != nil {
		t.Fatal(err)
	}
}

func TestRunRefusesWithoutConfiguration(t *testing.T) {
	_, err := Run("", noEnv)
	if !IsRefused(err) || !strings.Contains(err.Error(), config.EnvServiceConfig) {
		t.Fatalf("err = %v", err)
	}
	if _, err := Run(filepath.Join(t.TempDir(), "absent.yaml"), noEnv); !IsRefused(err) {
		t.Fatalf("err = %v", err)
	}
}

func TestRunRefusesAndNamesEveryCause(t *testing.T) {
	dir, path := setup(t)
	write(t, filepath.Join(dir, "templates/plain/letter.latex"), "$name$ $ghost$ \\directlua{x}\n")
	write(t, filepath.Join(dir, "configurations/sample/configuration.yaml"), "manifest_version: 1\nextends: base\n")
	_, err := Run(path, noEnv)
	if !IsRefused(err) {
		t.Fatalf("err = %v", err)
	}
	for _, want := range []string{"ghost", "directlua", "not supported in v1"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("cause %q missing from:\n%v", want, err)
		}
	}
}

func TestRunRefusesOnUnknownServiceKey(t *testing.T) {
	_, path := setup(t)
	write(t, path, "bogus: 1\n")
	if _, err := Run(path, noEnv); !IsRefused(err) || !strings.Contains(err.Error(), "bogus") {
		t.Fatalf("err = %v", err)
	}
}

func TestRunRefusesOnMissingAllowListedPackage(t *testing.T) {
	dir, path := setup(t)
	write(t, path, "templatesRoot: "+filepath.Join(dir, "templates")+"\nconfigurationsRoot: "+filepath.Join(dir, "configurations")+"\nbindings:\n  - {template: plain, configuration: nope}\n")
	if _, err := Run(path, noEnv); !IsRefused(err) || !strings.Contains(err.Error(), `missing configuration "nope"`) {
		t.Fatalf("err = %v", err)
	}
}

func TestEnvironmentRelocatesRoots(t *testing.T) {
	dir, path := setup(t)
	moved := filepath.Join(dir, "moved")
	if err := os.Rename(filepath.Join(dir, "templates"), moved); err != nil {
		t.Fatal(err)
	}
	env := func(k string) string {
		if k == config.EnvTemplatesRoot {
			return moved
		}
		return ""
	}
	if _, err := Run(path, env); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(path, noEnv); !IsRefused(err) {
		t.Fatal("without the override the old root is gone")
	}
}

func TestWarningsDoNotStopStartup(t *testing.T) {
	dir, path := setup(t)
	write(t, filepath.Join(dir, "templates/other/manifest.yaml"), manifest)
	write(t, filepath.Join(dir, "templates/other/letter.latex"), "$name$\n")
	res, err := Run(path, noEnv)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Warnings) == 0 {
		t.Fatal("expected an allow-list warning")
	}
}
