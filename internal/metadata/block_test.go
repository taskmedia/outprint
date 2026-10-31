package metadata

import (
	"strings"
	"testing"
)

func TestNoBlockIsValidWhenNothingIsRequired(t *testing.T) {
	env := newEnv(t, testConfiguration)

	res := resolveOK(t, env.input("Just a body.\n"))

	if string(res.Body) != "Just a body.\n" {
		t.Errorf("body = %q", res.Body)
	}
	if res.DocumentType != "letter" {
		t.Errorf("document type = %q", res.DocumentType)
	}
	wantValues(t, res.Values, map[string]any{"language": "en", "closing": "Kind regards", "name": "Sample Club"})
}

func TestBlockIsParsedAndRemovedFromBody(t *testing.T) {
	env := newEnv(t, testConfiguration)
	doc := "\n\n" + block("subject: Spring sale", "recipient:", "  - Ada Example", "  - 1 Sample Lane") + "\nFirst paragraph.\n"

	res := resolveOK(t, env.input(doc))

	if strings.Contains(string(res.Body), "metadata") || strings.Contains(string(res.Body), "Spring sale") {
		t.Errorf("block leaked into body: %q", res.Body)
	}
	if string(res.Body) != "First paragraph.\n" {
		t.Errorf("body = %q", res.Body)
	}
	if res.Values["subject"] != "Spring sale" {
		t.Errorf("subject = %v", res.Values["subject"])
	}
	wantList := []string{"Ada Example", "1 Sample Lane"}
	if got, _ := res.Values["recipient"].([]string); strings.Join(got, "|") != strings.Join(wantList, "|") {
		t.Errorf("recipient = %#v", res.Values["recipient"])
	}
}

func TestEmptyBlockIsAllowed(t *testing.T) {
	env := newEnv(t, testConfiguration)
	res := resolveOK(t, env.input("```yaml metadata\n```\nBody\n"))
	if string(res.Body) != "Body\n" {
		t.Errorf("body = %q", res.Body)
	}
}

func TestSecondBlockIsAnError(t *testing.T) {
	env := newEnv(t, testConfiguration)
	doc := block("subject: One") + "\nText\n\n" + block("subject: Two")

	errs := resolveErrors(t, env.input(doc))

	e := find(t, errs, "")
	wantError(t, e, SourceDocument, CodeBlock, 7, "second", "metadata block")
}

func TestMisplacedBlockIsAnError(t *testing.T) {
	env := newEnv(t, testConfiguration)
	doc := "Some text first.\n\n" + block("subject: Late")

	errs := resolveErrors(t, env.input(doc))

	wantError(t, find(t, errs, ""), SourceDocument, CodeBlock, 3, "start of the document")
}

func TestBlockInsideAnotherFenceIsExampleText(t *testing.T) {
	env := newEnv(t, testConfiguration)
	doc := block("subject: Real") + "\n````markdown\n" + block("subject: Shown") + "````\n"

	res := resolveOK(t, env.input(doc))

	if res.Values["subject"] != "Real" {
		t.Errorf("subject = %v", res.Values["subject"])
	}
	if !strings.Contains(string(res.Body), "subject: Shown") {
		t.Errorf("example block missing from body: %q", res.Body)
	}
}

func TestOtherYamlFencesAreBody(t *testing.T) {
	env := newEnv(t, testConfiguration)
	res := resolveOK(t, env.input("```yaml\nkey: value\n```\n"))
	if !strings.Contains(string(res.Body), "key: value") {
		t.Errorf("body = %q", res.Body)
	}
}

func TestUnclosedBlockIsAnError(t *testing.T) {
	env := newEnv(t, testConfiguration)
	errs := resolveErrors(t, env.input("```yaml metadata\nsubject: x\n"))
	wantError(t, find(t, errs, ""), SourceDocument, CodeBlock, 1, "not closed")
}

func TestYamlSyntaxErrorCarriesDocumentLine(t *testing.T) {
	env := newEnv(t, testConfiguration)
	doc := "\n" + block("subject: ok", "recipient: [unclosed")

	errs := resolveErrors(t, env.input(doc))

	e := find(t, errs, "")
	if e.Code != CodeBlock || e.Line == nil || *e.Line < 3 {
		t.Errorf("unexpected %+v", e)
	}
}

func TestBlockMustBeAMapping(t *testing.T) {
	env := newEnv(t, testConfiguration)
	errs := resolveErrors(t, env.input(block("- a", "- b")))
	wantError(t, find(t, errs, ""), SourceDocument, CodeBlock, 2, "mapping")
}

func TestDuplicateKeyIsAnError(t *testing.T) {
	env := newEnv(t, testConfiguration)
	errs := resolveErrors(t, env.input(block("subject: a", "subject: b")))
	wantError(t, find(t, errs, "subject"), SourceDocument, CodeBlock, 3, "twice")
}

func TestAliasesAreRejected(t *testing.T) {
	env := newEnv(t, testConfiguration)
	errs := resolveErrors(t, env.input(block("subject: &a x", "closing: *a")))
	wantError(t, find(t, errs, "closing"), SourceDocument, CodeType, 3, "alias")
}
