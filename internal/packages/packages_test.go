package packages

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

const validManifest = `manifest_version: 1
document_types:
  letter:
    template: letter.latex
`

const validConfiguration = `manifest_version: 1
defaults:
  document_type: letter
  closing: Regards
fields:
  name: Sample Club
  logo: logo.svg
`

func fixtureRoots(t *testing.T) Roots {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "templates", "plain", "manifest.yaml"), validManifest)
	writeFile(t, filepath.Join(dir, "templates", "plain", "letter.latex"), "body")
	writeFile(t, filepath.Join(dir, "configurations", "sample", "configuration.yaml"), validConfiguration)
	writeFile(t, filepath.Join(dir, "configurations", "sample", "logo.svg"), "<svg/>")
	return Roots{Templates: filepath.Join(dir, "templates"), Configurations: filepath.Join(dir, "configurations")}
}

func TestLoadTemplate(t *testing.T) {
	roots := fixtureRoots(t)

	tpl, err := LoadTemplate(roots.Templates, "plain")
	if err != nil {
		t.Fatal(err)
	}
	file, err := tpl.DocumentTypeFile("letter")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(file) != "letter.latex" {
		t.Fatalf("unexpected template file %q", file)
	}
	if _, err := tpl.DocumentTypeFile("report"); err == nil {
		t.Fatal("expected error for unknown document type")
	}
}

func TestLoadConfiguration(t *testing.T) {
	roots := fixtureRoots(t)

	cfg, err := LoadConfiguration(roots.Configurations, "sample")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Fixed["name"] != "Sample Club" {
		t.Fatalf("fixed name = %v", cfg.Fixed["name"])
	}
	if cfg.Defaults["closing"] != "Regards" {
		t.Fatalf("default closing = %v", cfg.Defaults["closing"])
	}
	asset, err := cfg.AssetPath("logo.svg")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(asset) != "logo.svg" {
		t.Fatalf("unexpected asset %q", asset)
	}
}

func TestRejectsBadNames(t *testing.T) {
	roots := fixtureRoots(t)
	for _, name := range []string{"", "..", "../plain", "a/b", "Plain", "-x", "/abs", "pl ain"} {
		if _, err := LoadTemplate(roots.Templates, name); err == nil {
			t.Errorf("template %q: expected error", name)
		}
		if _, err := LoadConfiguration(roots.Configurations, name); err == nil {
			t.Errorf("configuration %q: expected error", name)
		}
	}
}

func TestMissingPackage(t *testing.T) {
	roots := fixtureRoots(t)
	if _, err := LoadTemplate(roots.Templates, "absent"); err == nil {
		t.Fatal("expected error for missing template")
	}
	if _, err := LoadConfiguration(roots.Configurations, "absent"); err == nil {
		t.Fatal("expected error for missing configuration")
	}
}

func TestRejectsUnknownManifestVersion(t *testing.T) {
	roots := fixtureRoots(t)
	writeFile(t, filepath.Join(roots.Templates, "plain", "manifest.yaml"), "manifest_version: 2\n")
	if _, err := LoadTemplate(roots.Templates, "plain"); err == nil || !strings.Contains(err.Error(), "manifest_version") {
		t.Fatalf("expected manifest_version error, got %v", err)
	}
	writeFile(t, filepath.Join(roots.Configurations, "sample", "configuration.yaml"), "manifest_version: 2\n")
	if _, err := LoadConfiguration(roots.Configurations, "sample"); err == nil || !strings.Contains(err.Error(), "manifest_version") {
		t.Fatalf("expected manifest_version error, got %v", err)
	}
}

func TestRejectsExtends(t *testing.T) {
	roots := fixtureRoots(t)
	writeFile(t, filepath.Join(roots.Configurations, "sample", "configuration.yaml"), "manifest_version: 1\nextends: other\n")
	if _, err := LoadConfiguration(roots.Configurations, "sample"); err == nil || !strings.Contains(err.Error(), "extends") {
		t.Fatalf("expected extends error, got %v", err)
	}
}

func TestAssetPathContainment(t *testing.T) {
	roots := fixtureRoots(t)
	cfg, err := LoadConfiguration(roots.Configurations, "sample")
	if err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(filepath.Dir(cfg.Dir), "secret.txt")
	writeFile(t, outside, "secret")
	if err := os.Symlink(outside, filepath.Join(cfg.Dir, "link.svg")); err != nil {
		t.Fatal(err)
	}

	for _, rel := range []string{"../secret.txt", "/etc/passwd", "sub/../../secret.txt", "link.svg", "missing.svg", ""} {
		if _, err := cfg.AssetPath(rel); err == nil {
			t.Errorf("asset %q: expected error", rel)
		}
	}
}

func TestDocumentTypeFileContainment(t *testing.T) {
	roots := fixtureRoots(t)
	writeFile(t, filepath.Join(roots.Templates, "plain", "manifest.yaml"), `manifest_version: 1
document_types:
  letter:
    template: ../../outside.latex
`)
	tpl, err := LoadTemplate(roots.Templates, "plain")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tpl.DocumentTypeFile("letter"); err == nil {
		t.Fatal("expected containment error")
	}
}

func TestSharedFilesContained(t *testing.T) {
	roots := fixtureRoots(t)
	tpl, err := LoadTemplate(roots.Templates, "plain")
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(tpl.Dir, "shared", "base.tex"), "x")
	files, err := tpl.SharedFiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0] != "base.tex" {
		t.Fatalf("shared files = %v", files)
	}
}
