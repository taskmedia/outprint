package packages

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/taskmedia/outprint/internal/config"
)

const fixtureManifest = `manifest_version: 1
fields:
  name:
    type: string
    description: Organisation name.
  accent_color:
    type: color
    pattern: '^[0-9A-Fa-f]{6}$'
  logo:
    type: asset
    pattern: '^[A-Za-z0-9_-]+\.svg$'
  subject:
    type: string
document_types:
  letter:
    template: letter.latex
    fields: [name, accent_color, logo, subject]
`

const fixtureLetter = `\definecolor{accent}{HTML}{$accent_color$}
\input{shared/base.tex}
\includegraphics{$logo$}
$name$ $subject$ $body$
`

const fixtureConfiguration = `manifest_version: 1
defaults:
  document_type: letter
  subject: Hello
fields:
  name: Sample Club
  accent_color: "2F6F5E"
  logo: logo.svg
`

type fixture struct {
	t    *testing.T
	root string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{t: t, root: t.TempDir()}
	f.write("templates/plain/manifest.yaml", fixtureManifest)
	f.write("templates/plain/letter.latex", fixtureLetter)
	f.write("templates/plain/shared/base.tex", `\newcommand{\sample}{x}`)
	f.write("configurations/sample/configuration.yaml", fixtureConfiguration)
	f.write("configurations/sample/logo.svg", "<svg/>")
	return f
}

func (f *fixture) path(rel string) string { return filepath.Join(f.root, rel) }

func (f *fixture) write(rel, content string) {
	f.t.Helper()
	if err := os.MkdirAll(filepath.Dir(f.path(rel)), 0o755); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(f.path(rel), []byte(content), 0o644); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) roots() Roots {
	return Roots{Templates: f.path("templates"), Configurations: f.path("configurations")}
}

func (f *fixture) service(bindings ...config.Binding) config.Service {
	if len(bindings) == 0 {
		bindings = []config.Binding{{Template: "plain", Configuration: "sample"}}
	}
	return config.Service{
		TemplatesRoot:      f.path("templates"),
		ConfigurationsRoot: f.path("configurations"),
		Bindings:           bindings,
	}
}

func (f *fixture) load() (*Registry, []Finding) { return Load(f.service()) }

// mentions reports whether some finding has the severity and a message
// containing every part.
func mentions(findings []Finding, s Severity, parts ...string) bool {
	for _, fd := range findings {
		text := fd.String()
		all := fd.Severity == s
		for _, p := range parts {
			all = all && strings.Contains(text, p)
		}
		if all {
			return true
		}
	}
	return false
}

func dump(findings []Finding) string {
	var b strings.Builder
	for _, f := range findings {
		b.WriteString(f.String() + "\n")
	}
	return b.String()
}
