package metadata

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/taskmedia/outline-latex-pdf/internal/packages"
)

const testManifest = `manifest_version: 1
fields:
  title:
    type: string
  date:
    type: date
  language:
    type: string
    pattern: '^(en|de)$'
    default: en
  name:
    type: string
  subject:
    type: string
  recipient:
    type: list
  closing:
    type: string
  side:
    type: enum
    values: [left, right]
  code:
    type: string
    pattern: '^[A-Z]{2}[0-9]{3}$'
  abstract:
    type: text
  toc:
    type: bool
  reference:
    type: string
    required: true
document_types:
  letter:
    template: doc.latex
    fields: [title, date, language, name, subject, recipient, closing, side, code]
  report:
    template: doc.latex
    fields: [title, date, language, abstract, toc]
  strict:
    template: doc.latex
    fields: [title, reference, subject]
`

const testConfiguration = `manifest_version: 1
defaults:
  document_type: letter
  closing: Kind regards
  unrelated_extra: tolerated
fields:
  name: Sample Club
`

type testEnv struct {
	template packages.Template
	config   packages.Configuration
}

func newEnv(t *testing.T, configuration string) testEnv {
	t.Helper()
	root := t.TempDir()
	write := func(rel, content string) {
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("templates/sample/manifest.yaml", testManifest)
	write("templates/sample/doc.latex", "$body$\n")
	write("configurations/club/configuration.yaml", configuration)
	tpl, err := packages.LoadTemplate(filepath.Join(root, "templates"), "sample")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := packages.LoadConfiguration(filepath.Join(root, "configurations"), "club")
	if err != nil {
		t.Fatal(err)
	}
	return testEnv{template: tpl, config: cfg}
}

func (e testEnv) input(markdown string) Input {
	return Input{Markdown: []byte(markdown), Template: e.template, Configuration: e.config}
}

func resolveOK(t *testing.T, in Input) Result {
	t.Helper()
	res, err := Resolve(in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return res
}

// resolveErrors returns the structured errors Resolve reports.
func resolveErrors(t *testing.T, in Input) []Error {
	t.Helper()
	_, err := Resolve(in)
	list, ok := err.(*ErrorList)
	if !ok {
		t.Fatalf("expected *ErrorList, got %T: %v", err, err)
	}
	return list.Errors
}

// find returns the error for a field, failing the test if it is missing.
func find(t *testing.T, errs []Error, field string) Error {
	t.Helper()
	for _, e := range errs {
		if e.Field == field {
			return e
		}
	}
	t.Fatalf("no error for field %q in %v", field, errs)
	return Error{}
}

func wantError(t *testing.T, e Error, source, code string, line int, messageParts ...string) {
	t.Helper()
	if e.Source != source || e.Code != code {
		t.Errorf("got source=%s code=%s, want source=%s code=%s (%+v)", e.Source, e.Code, source, code, e)
	}
	switch {
	case line == 0 && e.Line != nil:
		t.Errorf("line = %d, want none", *e.Line)
	case line != 0 && (e.Line == nil || *e.Line != line):
		t.Errorf("line = %v, want %d", e.Line, line)
	}
	for _, part := range messageParts {
		if !strings.Contains(e.Message, part) {
			t.Errorf("message %q lacks %q", e.Message, part)
		}
	}
}

func block(lines ...string) string {
	return "```yaml metadata\n" + strings.Join(lines, "\n") + "\n```\n"
}

func wantValues(t *testing.T, got, want map[string]any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		gotJSON, _ := json.Marshal(got)
		wantJSON, _ := json.Marshal(want)
		t.Fatalf("values\n got  %s\n want %s", gotJSON, wantJSON)
	}
}
