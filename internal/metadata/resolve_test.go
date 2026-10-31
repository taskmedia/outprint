package metadata

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestLayeringManifestUnderConfigurationUnderDocument(t *testing.T) {
	env := newEnv(t, testConfiguration)

	res := resolveOK(t, env.input(block("closing: Best wishes", "language: de")))

	wantValues(t, res.Values, map[string]any{"language": "de", "closing": "Best wishes", "name": "Sample Club"})
}

func TestConfigurationDefaultOverridesManifestDefault(t *testing.T) {
	env := newEnv(t, testConfiguration+"") // language default comes from the manifest
	if resolveOK(t, env.input("x\n")).Values["language"] != "en" {
		t.Fatal("manifest default not applied")
	}
	env = newEnv(t, "manifest_version: 1\ndefaults:\n  document_type: letter\n  language: de\n")
	if got := resolveOK(t, env.input("x\n")).Values["language"]; got != "de" {
		t.Fatalf("language = %v, want the Configuration's de", got)
	}
}

func TestEmptyDocumentValueSuppressesDefault(t *testing.T) {
	env := newEnv(t, testConfiguration)
	for name, line := range map[string]string{"null": "closing:", "empty string": `closing: ""`, "empty list": "closing: []"} {
		res := resolveOK(t, env.input(block(line)))
		if _, present := res.Values["closing"]; present {
			t.Errorf("%s: closing = %v, want it suppressed", name, res.Values["closing"])
		}
		if res.Values["name"] != "Sample Club" {
			t.Errorf("%s: other values changed: %v", name, res.Values)
		}
	}
}

func TestSuppressedRequiredFieldIsReported(t *testing.T) {
	env := newEnv(t, "manifest_version: 1\ndefaults:\n  document_type: strict\n  reference: R-1\n")
	errs := resolveErrors(t, env.input(block("reference:")))
	wantError(t, find(t, errs, "reference"), SourceDocument, CodeRequired, 2, "empty")
}

func TestDerivedTitleAndDateAreDefaults(t *testing.T) {
	env := newEnv(t, testConfiguration)
	in := env.input("x\n")
	in.Title, in.Modified = "From the wiki", "2026-03-02"

	res := resolveOK(t, in)
	if res.Values["title"] != "From the wiki" || res.Values["date"] != "2026-03-02" {
		t.Errorf("derived values: %v", res.Values)
	}

	in.Markdown = []byte(block("title: Own title", "date: 14 March 2026"))
	res = resolveOK(t, in)
	if res.Values["title"] != "Own title" || res.Values["date"] != "14 March 2026" {
		t.Errorf("document values must win: %v", res.Values)
	}
}

func TestDerivedValuesIgnoredWhenTypeLacksTheField(t *testing.T) {
	env := newEnv(t, "manifest_version: 1\ndefaults:\n  document_type: strict\n  reference: R-1\n")
	in := env.input("x\n")
	in.Modified = "2026-03-02"
	if _, present := resolveOK(t, in).Values["date"]; present {
		t.Error("date is not a field of document type strict")
	}
}

func TestInvalidDerivedDateIsReported(t *testing.T) {
	env := newEnv(t, testConfiguration)
	in := env.input("x\n")
	in.Modified = "yesterday"
	errs := resolveErrors(t, in)
	wantError(t, find(t, errs, "date"), SourceDocument, CodeType, 0, "ISO")
}

func TestConfigurationDefaultsTrumpDerivedValues(t *testing.T) {
	env := newEnv(t, "manifest_version: 1\ndefaults:\n  document_type: letter\n  title: Fixed headline\n")
	in := env.input("x\n")
	in.Title = "From the wiki"
	if got := resolveOK(t, in).Values["title"]; got != "Fixed headline" {
		t.Errorf("title = %v", got)
	}
}

func TestFixedValuesWinAndAreOutput(t *testing.T) {
	env := newEnv(t, testConfiguration)
	res := resolveOK(t, env.input("x\n"))
	if res.Values["name"] != "Sample Club" {
		t.Errorf("name = %v", res.Values["name"])
	}
}

func TestDocumentTypeFromDocumentThenConfiguration(t *testing.T) {
	env := newEnv(t, testConfiguration)
	if got := resolveOK(t, env.input("x\n")).DocumentType; got != "letter" {
		t.Errorf("from configuration: %q", got)
	}
	res := resolveOK(t, env.input(block("type: report", "toc: true", "abstract: Some *emphasis*")))
	if res.DocumentType != "report" {
		t.Errorf("from document: %q", res.DocumentType)
	}
	if res.Values["toc"] != true || res.Values["abstract"] != "Some *emphasis*" {
		t.Errorf("values: %v", res.Values)
	}
	if _, ok := res.Values["closing"]; ok {
		t.Errorf("report does not use closing: %v", res.Values)
	}
}

func TestUnknownTypeIsAnError(t *testing.T) {
	env := newEnv(t, testConfiguration)
	errs := resolveErrors(t, env.input(block("type: memo")))
	wantError(t, find(t, errs, "type"), SourceDocument, CodeUnknown, 2, `"memo"`, "letter", "report")
}

func TestKeysAreStillCheckedWhenTheTypeIsUnknown(t *testing.T) {
	env := newEnv(t, testConfiguration)
	errs := resolveErrors(t, env.input(block("type: memo", "subjct: x", "name: y", "subject: fine")))
	wantError(t, find(t, errs, "type"), SourceDocument, CodeUnknown, 2)
	wantError(t, find(t, errs, "subjct"), SourceDocument, CodeUnknown, 3, "did you mean")
	wantError(t, find(t, errs, "name"), SourceDocument, CodeUnknown, 4, "Fixed Value")
	if len(errs) != 3 {
		t.Errorf("got %d errors: %v", len(errs), errs)
	}
}

func TestUnknownConfigurationTypeIsAConfigurationError(t *testing.T) {
	env := newEnv(t, "manifest_version: 1\ndefaults:\n  document_type: memo\n")
	errs := resolveErrors(t, env.input("x\n"))
	wantError(t, find(t, errs, "type"), SourceConfiguration, CodeUnknown, 0, `"memo"`)
}

func TestMissingTypeIsRequired(t *testing.T) {
	env := newEnv(t, "manifest_version: 1\n")
	errs := resolveErrors(t, env.input("x\n"))
	wantError(t, find(t, errs, "type"), SourceDocument, CodeRequired, 0, "document type")
}

func TestUnknownKeyIsRejectedWithNameLineAndHint(t *testing.T) {
	env := newEnv(t, testConfiguration)
	doc := "\n" + block("subject: ok", "recipent: Ada")

	errs := resolveErrors(t, env.input(doc))

	wantError(t, find(t, errs, "recipent"), SourceDocument, CodeUnknown, 4, `"recipent"`, `did you mean "recipient"?`)
}

func TestUnknownKeyWithoutCloseMatchHasNoHint(t *testing.T) {
	env := newEnv(t, testConfiguration)
	errs := resolveErrors(t, env.input(block("zzzzzzzz: 1")))
	e := find(t, errs, "zzzzzzzz")
	wantError(t, e, SourceDocument, CodeUnknown, 2)
	if strings.Contains(e.Message, "did you mean") {
		t.Errorf("unexpected hint: %q", e.Message)
	}
}

func TestKeyOfAnotherDocumentTypeIsUnknown(t *testing.T) {
	env := newEnv(t, testConfiguration)
	errs := resolveErrors(t, env.input(block("toc: true")))
	wantError(t, find(t, errs, "toc"), SourceDocument, CodeUnknown, 2, "letter")
}

func TestFixedValueKeyIsRejected(t *testing.T) {
	env := newEnv(t, testConfiguration)
	errs := resolveErrors(t, env.input(block("name: Somebody Else")))
	wantError(t, find(t, errs, "name"), SourceDocument, CodeUnknown, 2, "Fixed Value")
}

func TestTypeKeyNeedsAScalar(t *testing.T) {
	env := newEnv(t, testConfiguration)
	errs := resolveErrors(t, env.input(block("type: [letter]")))
	wantError(t, find(t, errs, "type"), SourceDocument, CodeType, 2)
}

func TestWrongTypesAreReported(t *testing.T) {
	env := newEnv(t, testConfiguration)
	cases := map[string]struct{ line, field string }{
		"mapping for string": {"subject:\n  a: b", "subject"},
		"list for string":    {"subject: [a, b]", "subject"},
		"multi-line string":  {"subject: |\n  a\n  b", "subject"},
		"nested list item":   {"recipient:\n  - [a]", "recipient"},
	}
	for name, tc := range cases {
		errs := resolveErrors(t, env.input(block(tc.line)))
		if e := find(t, errs, tc.field); e.Code != CodeType || e.Source != SourceDocument {
			t.Errorf("%s: %+v", name, e)
		}
	}

	res := resolveOK(t, env.input(block("type: report", "toc: true")))
	if res.Values["toc"] != true {
		t.Errorf("toc = %v", res.Values["toc"])
	}
	errs := resolveErrors(t, env.input(block("type: report", "toc: maybe")))
	wantError(t, find(t, errs, "toc"), SourceDocument, CodeType, 3, "true or false")
}

func TestOnlyScalarsCoerceToText(t *testing.T) {
	env := newEnv(t, testConfiguration)
	res := resolveOK(t, env.input(block("subject: 2026", "closing: true")))
	if res.Values["subject"] != "2026" || res.Values["closing"] != "true" {
		t.Errorf("values: %v", res.Values)
	}
}

func TestBadEnumAndPatternAreReported(t *testing.T) {
	env := newEnv(t, testConfiguration)
	errs := resolveErrors(t, env.input(block("side: middle", "code: abc")))
	wantError(t, find(t, errs, "side"), SourceDocument, CodeEnum, 2, "left", "right")
	wantError(t, find(t, errs, "code"), SourceDocument, CodeType, 3, "pattern")
}

func TestMissingRequiredField(t *testing.T) {
	env := newEnv(t, "manifest_version: 1\ndefaults:\n  document_type: strict\n")
	errs := resolveErrors(t, env.input("x\n"))
	wantError(t, find(t, errs, "reference"), SourceDocument, CodeRequired, 0, "required")
}

func TestListFromYamlListOrMultiLineString(t *testing.T) {
	env := newEnv(t, testConfiguration)
	want := []string{"Ada Example", "1 Sample Lane", "12345 Exampleton"}
	docs := map[string]string{
		"yaml list":         block("recipient:", "  - Ada Example", "  - 1 Sample Lane", "  - 12345 Exampleton"),
		"block scalar":      block("recipient: |", "  Ada Example", "  1 Sample Lane", "", "  12345 Exampleton"),
		"flow list":         block("recipient: [Ada Example, 1 Sample Lane, 12345 Exampleton]"),
		"quoted with break": block(`recipient: "Ada Example\n1 Sample Lane\n12345 Exampleton"`),
	}
	for name, doc := range docs {
		got, _ := resolveOK(t, env.input(doc)).Values["recipient"].([]string)
		if len(got) != len(want) {
			t.Errorf("%s: %#v", name, got)
			continue
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("%s: item %d = %q", name, i, got[i])
			}
		}
	}
	single, _ := resolveOK(t, env.input(block("recipient: Ada Example"))).Values["recipient"].([]string)
	if len(single) != 1 || single[0] != "Ada Example" {
		t.Errorf("single line: %#v", single)
	}
}

func TestStringsKeepTextVerbatim(t *testing.T) {
	env := newEnv(t, testConfiguration)
	res := resolveOK(t, env.input(block(`subject: 'Costs $x$ and \input{foo} *not emphasis*'`)))
	if res.Values["subject"] != `Costs $x$ and \input{foo} *not emphasis*` {
		t.Errorf("subject = %v", res.Values["subject"])
	}
}

func TestControlCharactersAreRejected(t *testing.T) {
	env := newEnv(t, testConfiguration)
	errs := resolveErrors(t, env.input(block(`subject: "a\u0000b"`)))
	wantError(t, find(t, errs, "subject"), SourceDocument, CodeType, 2, "control")
}

func TestConfigurationDefaultsMayCarryUndeclaredFields(t *testing.T) {
	env := newEnv(t, testConfiguration)
	res := resolveOK(t, env.input("x\n"))
	if _, present := res.Values["unrelated_extra"]; present {
		t.Error("undeclared Default must be ignored")
	}
}

func TestInvalidConfigurationDefaultIsAConfigurationError(t *testing.T) {
	env := newEnv(t, "manifest_version: 1\ndefaults:\n  document_type: letter\n  side: middle\n")
	errs := resolveErrors(t, env.input("x\n"))
	wantError(t, find(t, errs, "side"), SourceConfiguration, CodeEnum, 0)
	// the Document can repair it
	resolveOK(t, env.input(block("side: left")))
}

func TestAllErrorsAreCollected(t *testing.T) {
	env := newEnv(t, "manifest_version: 1\ndefaults:\n  document_type: strict\n")
	doc := block("subjct: typo", "title: [a]", "reference: ", "name: nope")

	errs := resolveErrors(t, env.input(doc))

	for field, code := range map[string]string{"subjct": CodeUnknown, "title": CodeType, "reference": CodeRequired, "name": CodeUnknown} {
		if e := find(t, errs, field); e.Code != code {
			t.Errorf("%s: %+v", field, e)
		}
	}
	if len(errs) != 4 {
		t.Errorf("got %d errors: %v", len(errs), errs)
	}
}

func TestErrorListJSONShape(t *testing.T) {
	env := newEnv(t, testConfiguration)
	_, err := Resolve(env.input(block("subjct: x", "side: middle")))

	data, jsonErr := json.Marshal(err)
	if jsonErr != nil {
		t.Fatal(jsonErr)
	}
	var body struct {
		Errors []map[string]any `json:"errors"`
	}
	if err := json.Unmarshal(data, &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Errors) != 2 {
		t.Fatalf("body %s", data)
	}
	for _, e := range body.Errors {
		for _, key := range []string{"field", "source", "code", "message", "line"} {
			if _, ok := e[key]; !ok {
				t.Errorf("error %v lacks %q", e, key)
			}
		}
	}
	if body.Errors[0]["line"] != float64(2) {
		t.Errorf("line = %v", body.Errors[0]["line"])
	}
}

func TestErrorWithoutLineSerialisesNull(t *testing.T) {
	data, err := json.Marshal(Error{Field: "f", Source: SourceTemplate, Code: CodeRequired, Message: "m"})
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `{"field":"f","source":"template","code":"required","message":"m","line":null}` {
		t.Errorf("json = %s", data)
	}
}

func TestErrorTextIsReadable(t *testing.T) {
	line := 4
	e := Error{Field: "subjct", Source: SourceDocument, Code: CodeUnknown, Message: "unknown key", Line: &line}
	if got := e.String(); got != "document line 4: subjct: unknown key (unknown)" {
		t.Errorf("String() = %q", got)
	}
}

func TestSuggest(t *testing.T) {
	names := []string{"subject", "recipient", "closing"}
	cases := map[string]string{"subjct": "subject", "recipent": "recipient", "closng": "closing", "zzzzzz": "", "su": ""}
	for typo, want := range cases {
		if got := suggest(typo, names); got != want {
			t.Errorf("suggest(%q) = %q, want %q", typo, got, want)
		}
	}
}
