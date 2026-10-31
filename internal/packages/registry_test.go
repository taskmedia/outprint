package packages

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/taskmedia/outline-latex-pdf/internal/config"
)

func TestLoadValidFixture(t *testing.T) {
	fx := newFixture(t)
	reg, findings := fx.load()
	if reg == nil || len(findings) != 0 {
		t.Fatalf("want clean load, got:\n%s", dump(findings))
	}
	tpl, ok := reg.Template("plain")
	if !ok || tpl.Fields["accent_color"].Type != TypeColor {
		t.Fatalf("template: %+v", tpl)
	}
	if _, ok := reg.Configuration("sample"); !ok {
		t.Fatal("configuration missing")
	}
	b := config.Binding{Template: "plain", Configuration: "sample"}
	if !reg.Allowed(b) || reg.Allowed(config.Binding{Template: "plain", Configuration: "other"}) {
		t.Fatal("allow-list wrong")
	}
	if _, ok := reg.DefaultBinding(); ok {
		t.Fatal("no default binding configured")
	}
}

func TestRegistryIsImmutable(t *testing.T) {
	fx := newFixture(t)
	reg, _ := fx.load()
	tpl, _ := reg.Template("plain")
	tpl.Fields["injected"] = Field{Name: "injected"}
	delete(tpl.DocumentTypes, "letter")
	cfg, _ := reg.Configuration("sample")
	cfg.Fixed["name"] = "changed"
	cfg.Defaults["subject"] = "changed"
	bindings := reg.Bindings()
	bindings[0].Template = "changed"

	tpl2, _ := reg.Template("plain")
	cfg2, _ := reg.Configuration("sample")
	if _, bad := tpl2.Fields["injected"]; bad || len(tpl2.DocumentTypes) != 1 {
		t.Error("template was mutated through an accessor")
	}
	if cfg2.Fixed["name"] != "Sample Club" || cfg2.Defaults["subject"] != "Hello" {
		t.Error("configuration was mutated through an accessor")
	}
	if reg.Bindings()[0].Template != "plain" {
		t.Error("bindings were mutated through an accessor")
	}
}

func TestDefaultBindingIsReturned(t *testing.T) {
	fx := newFixture(t)
	svc := fx.service()
	svc.DefaultBinding = &svc.Bindings[0]
	reg, findings := Load(svc)
	if reg == nil {
		t.Fatal(dump(findings))
	}
	if b, ok := reg.DefaultBinding(); !ok || b.Template != "plain" {
		t.Fatalf("default binding = %+v %v", b, ok)
	}
}

func TestAllowListEntryForMissingPackageFails(t *testing.T) {
	fx := newFixture(t)
	svc := fx.service(
		config.Binding{Template: "plain", Configuration: "sample"},
		config.Binding{Template: "ghost", Configuration: "sample"},
		config.Binding{Template: "plain", Configuration: "phantom"},
	)
	reg, findings := Load(svc)
	if reg != nil {
		t.Fatal("registry must be nil on errors")
	}
	if !mentions(findings, SeverityError, "missing template", "ghost") || !mentions(findings, SeverityError, "missing configuration", "phantom") {
		t.Fatalf("findings:\n%s", dump(findings))
	}
}

func TestPackagesOffTheAllowListWarn(t *testing.T) {
	fx := newFixture(t)
	fx.write("templates/extra/manifest.yaml", strings.ReplaceAll(fixtureManifest, "letter.latex", "page.latex"))
	fx.write("templates/extra/page.latex", fixtureLetter)
	fx.write("templates/extra/shared/base.tex", "x")
	fx.write("templates/broken/manifest.yaml", "manifest_version: 9\n")
	fx.write("configurations/unlisted/configuration.yaml", fixtureConfiguration)
	fx.write("configurations/unlisted/logo.svg", "<svg/>")

	reg, findings := fx.load()
	if reg == nil {
		t.Fatalf("unlisted packages must not fail the load:\n%s", dump(findings))
	}
	for _, pkg := range []string{"template/extra", "template/broken", "configuration/unlisted"} {
		if !mentions(findings, SeverityWarning, pkg, "not on the allow-list") {
			t.Errorf("no allow-list warning for %s:\n%s", pkg, dump(findings))
		}
	}
	if len(Errors(findings)) != 0 {
		t.Errorf("errors:\n%s", dump(Errors(findings)))
	}
	if _, ok := reg.Template("broken"); ok {
		t.Error("a broken unlisted package must not be in the registry")
	}
	if _, ok := reg.Template("extra"); !ok {
		t.Error("a valid unlisted package is loaded")
	}
	if reg.Allowed(config.Binding{Template: "extra", Configuration: "sample"}) {
		t.Error("an unlisted package must not be allowed")
	}
}

func TestInvalidDirectoryNamesWarn(t *testing.T) {
	fx := newFixture(t)
	fx.write("templates/Bad_Name/manifest.yaml", "x")
	_, findings := fx.load()
	if !mentions(findings, SeverityWarning, "Bad_Name", "ignored") {
		t.Fatalf("findings:\n%s", dump(findings))
	}
}

func TestMissingRootIsAnError(t *testing.T) {
	fx := newFixture(t)
	svc := fx.service()
	svc.TemplatesRoot = fx.path("nowhere")
	reg, findings := Load(svc)
	if reg != nil || !mentions(findings, SeverityError, "template root", "nowhere") {
		t.Fatalf("findings:\n%s", dump(findings))
	}
}

func TestAllErrorsAreCollected(t *testing.T) {
	fx := newFixture(t)
	fx.write("templates/plain/manifest.yaml", strings.Replace(fixtureManifest, "type: color", "type: colour", 1))
	fx.write("templates/plain/letter.latex", fixtureLetter+`\directlua{x}`+"\n$undeclared$\n")
	fx.write("configurations/sample/configuration.yaml", "manifest_version: 1\nextends: base\nbogus: 1\n")
	_, findings := fx.load()
	for _, want := range []string{"colour", "directlua", "undeclared", "not supported in v1", "bogus"} {
		if !mentions(findings, SeverityError, want) {
			t.Errorf("missing finding about %q:\n%s", want, dump(findings))
		}
	}
}

func TestInspectWithoutServiceConfiguration(t *testing.T) {
	fx := newFixture(t)
	reg, findings := Inspect(fx.roots(), "", "")
	if reg == nil || len(findings) != 0 {
		t.Fatalf("findings:\n%s", dump(findings))
	}
	if got := reg.TemplateNames(); len(got) != 1 || got[0] != "plain" {
		t.Fatalf("templates = %v", got)
	}

	fx.write("configurations/sample/configuration.yaml", strings.Replace(fixtureConfiguration, `"2F6F5E"`, `'red}\directlua{x}'`, 1))
	if _, findings := Inspect(fx.roots(), "plain", "sample"); !mentions(findings, SeverityError, "accent_color") {
		t.Fatalf("pair check missing:\n%s", dump(findings))
	}
	if _, findings := Inspect(fx.roots(), "", "sample"); mentions(findings, SeverityError, "accent_color") {
		t.Fatalf("a Configuration alone cannot be checked against pattern:\n%s", dump(findings))
	}
	if _, findings := Inspect(fx.roots(), "ghost", ""); !mentions(findings, SeverityError, "template/ghost") {
		t.Fatalf("findings:\n%s", dump(findings))
	}
}

func TestLoadTemplateAndConfigurationForRender(t *testing.T) {
	fx := newFixture(t)
	tpl, err := LoadTemplate(fx.roots().Templates, "plain")
	if err != nil {
		t.Fatal(err)
	}
	file, err := tpl.DocumentTypeFile("letter")
	if err != nil || filepath.Base(file) != "letter.latex" {
		t.Fatalf("%q %v", file, err)
	}
	if _, err := tpl.DocumentTypeFile("report"); err == nil {
		t.Fatal("expected error for unknown document type")
	}
	shared, err := tpl.SharedFiles()
	if err != nil || len(shared) != 1 || shared[0] != "base.tex" {
		t.Fatalf("shared = %v %v", shared, err)
	}
	if _, err := tpl.SharedPath("../manifest.yaml"); err == nil {
		t.Fatal("shared path must be confined")
	}

	cfg, err := LoadConfiguration(fx.roots().Configurations, "sample")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Fixed["name"] != "Sample Club" || cfg.Defaults["subject"] != "Hello" {
		t.Fatalf("%+v", cfg)
	}
	if _, err := cfg.AssetPath("logo.svg"); err != nil {
		t.Fatal(err)
	}
}

func TestLoadByNameRejectsBadAndMissingNames(t *testing.T) {
	fx := newFixture(t)
	for _, name := range []string{"", "..", "../plain", "a/b", "Plain", "-x", "/abs", "pl ain", "absent"} {
		if _, err := LoadTemplate(fx.roots().Templates, name); err == nil {
			t.Errorf("template %q: expected error", name)
		}
		if _, err := LoadConfiguration(fx.roots().Configurations, name); err == nil {
			t.Errorf("configuration %q: expected error", name)
		}
	}
}

func TestLoadByNameReportsFindings(t *testing.T) {
	fx := newFixture(t)
	fx.write("configurations/sample/configuration.yaml", "manifest_version: 1\nextends: base\n")
	_, err := LoadConfiguration(fx.roots().Configurations, "sample")
	if err == nil || !strings.Contains(err.Error(), "not supported in v1") {
		t.Fatalf("err = %v", err)
	}
}

func TestAssetPathContainment(t *testing.T) {
	fx := newFixture(t)
	cfg, err := LoadConfiguration(fx.roots().Configurations, "sample")
	if err != nil {
		t.Fatal(err)
	}
	fx.write("configurations/secret.txt", "secret")
	for _, rel := range []string{"../secret.txt", "/etc/passwd", "sub/../../secret.txt", "missing.svg", ""} {
		if _, err := cfg.AssetPath(rel); err == nil {
			t.Errorf("asset %q: expected error", rel)
		}
	}
	if err := os.Symlink(fx.path("configurations/secret.txt"), filepath.Join(cfg.Dir, "link.svg")); err != nil {
		t.Fatal(err)
	}
	if _, err := cfg.AssetPath("link.svg"); err == nil {
		t.Error("symlink escape must be rejected")
	}
}
