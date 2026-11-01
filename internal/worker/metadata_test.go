package worker

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/taskmedia/outprint/internal/packages"
)

const metaManifest = `manifest_version: 1
fields:
  title:
    type: string
  date:
    type: date
  language:
    type: string
    pattern: '^(en|de)$'
  recipient:
    type: list
  abstract:
    type: text
  toc:
    type: bool
document_types:
  report:
    template: report.latex
    fields: [title, date, language, recipient, abstract, toc]
`

func metaTemplate(t *testing.T) packages.Template {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "sample")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{"manifest.yaml": metaManifest, "report.latex": "$body$\n"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	tpl, err := packages.LoadTemplate(root, "sample")
	if err != nil {
		t.Fatal(err)
	}
	return tpl
}

func decodeMeta(t *testing.T, data []byte) map[string]any {
	t.Helper()
	var out map[string]any
	if err := yaml.Unmarshal(data, &out); err != nil {
		t.Fatalf("metadata is not valid YAML: %v\n%s", err, data)
	}
	return out
}

func TestPandocMetadataEscapesPlainValuesButNotText(t *testing.T) {
	tpl := metaTemplate(t)
	values := map[string]any{
		"title":     `Costs $x$ and \input{foo} *flat*`,
		"recipient": []string{"1. Ada", "R&D"},
		"abstract":  "Some *emphasis*",
		"toc":       true,
		"language":  "en",
	}

	data, err := pandocMetadata(tpl, values)
	if err != nil {
		t.Fatal(err)
	}

	got := decodeMeta(t, data)
	want := map[string]any{
		"title":     `Costs \$x\$ and \\input\{foo\} \*flat\*`,
		"recipient": []any{`1\. Ada`, `R\&D`},
		"abstract":  "Some *emphasis*",
		"toc":       true,
		"language":  "en",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got  %#v\nwant %#v", got, want)
	}
}

func TestPandocMetadataKeepsNonAsciiAndIsDeterministic(t *testing.T) {
	tpl := metaTemplate(t)
	values := map[string]any{"title": "Grüße aus Köln", "language": "de"}

	first, err := pandocMetadata(tpl, values)
	if err != nil {
		t.Fatal(err)
	}
	second, _ := pandocMetadata(tpl, values)
	if string(first) != string(second) {
		t.Fatal("output is not deterministic")
	}
	if !strings.Contains(string(first), "Grüße aus Köln") {
		t.Fatalf("non-ASCII text mangled:\n%s", first)
	}
	if strings.Index(string(first), `"language"`) > strings.Index(string(first), `"title"`) {
		t.Fatalf("keys not sorted:\n%s", first)
	}
}

func TestPandocMetadataTrimsIndentationOfPlainValues(t *testing.T) {
	tpl := metaTemplate(t)
	data, err := pandocMetadata(tpl, map[string]any{"title": "    indented"})
	if err != nil {
		t.Fatal(err)
	}
	if got := decodeMeta(t, data)["title"]; got != "indented" {
		t.Fatalf("title = %q", got)
	}
}

func TestPandocMetadataRejectsUndeclaredFields(t *testing.T) {
	if _, err := pandocMetadata(metaTemplate(t), map[string]any{"mystery": "x"}); err == nil {
		t.Fatal("expected an error for a field the Template does not declare")
	}
}

func TestPandocMetadataRejectsUnexpectedValueTypes(t *testing.T) {
	if _, err := pandocMetadata(metaTemplate(t), map[string]any{"title": 5}); err == nil {
		t.Fatal("expected an error for a non-string value")
	}
}

func TestFormatDatesByLanguage(t *testing.T) {
	tpl := metaTemplate(t)
	cases := []struct {
		language, date, want string
	}{
		{"en", "2026-03-02", "2 March 2026"},
		{"de", "2026-03-02", "2. März 2026"},
		{"de", "2026-12-24", "24. Dezember 2026"},
		{"en", "14 March 2026", "14 March 2026"}, // free text stays as written
	}
	for _, tc := range cases {
		got, err := formatDates(tpl, map[string]any{"language": tc.language, "date": tc.date})
		if err != nil {
			t.Fatal(err)
		}
		if got["date"] != tc.want {
			t.Errorf("%s %s -> %q, want %q", tc.language, tc.date, got["date"], tc.want)
		}
	}
}

func TestFormatDatesDoesNotChangeItsInput(t *testing.T) {
	in := map[string]any{"language": "en", "date": "2026-03-02"}
	if _, err := formatDates(metaTemplate(t), in); err != nil {
		t.Fatal(err)
	}
	if in["date"] != "2026-03-02" {
		t.Fatal("input was modified")
	}
}

func TestFormatDatesRefusesAnUnsupportedLanguage(t *testing.T) {
	_, err := formatDates(metaTemplate(t), map[string]any{"language": "xx", "date": "2026-03-02"})
	if err == nil || !strings.Contains(err.Error(), `"xx"`) {
		t.Fatalf("err = %v", err)
	}
}

func TestFormatDatesDefaultsToEnglishWithoutLanguage(t *testing.T) {
	got, err := formatDates(metaTemplate(t), map[string]any{"date": "2026-03-02"})
	if err != nil || got["date"] != "2 March 2026" {
		t.Fatalf("got %v, %v", got, err)
	}
}
