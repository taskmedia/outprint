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
	res := resolveOK(t, env.input("```yaml\n```\nBody\n"))
	if string(res.Body) != "Body\n" {
		t.Errorf("body = %q", res.Body)
	}
}

func TestSecondYamlFenceIsBody(t *testing.T) {
	env := newEnv(t, testConfiguration)
	doc := block("subject: One") + "\nText\n\n" + block("subject: Two")

	res := resolveOK(t, env.input(doc))

	if res.Values["subject"] != "One" {
		t.Errorf("subject = %v", res.Values["subject"])
	}
	if !strings.Contains(string(res.Body), "subject: Two") {
		t.Errorf("second fence missing from body: %q", res.Body)
	}
}

func TestYamlFenceAfterTextIsBody(t *testing.T) {
	env := newEnv(t, testConfiguration)
	doc := "Some text first.\n\n" + block("subject: Late")

	res := resolveOK(t, env.input(doc))

	if res.Values["subject"] == "Late" {
		t.Errorf("a fence after the first line was read as the block")
	}
	if !strings.Contains(string(res.Body), "subject: Late") {
		t.Errorf("fence missing from body: %q", res.Body)
	}
}

func TestOldInfoStringIsNotTheBlock(t *testing.T) {
	env := newEnv(t, testConfiguration)
	res := resolveOK(t, env.input("```yaml metadata\nsubject: Old\n```\nBody\n"))
	if res.Values["subject"] == "Old" || !strings.Contains(string(res.Body), "subject: Old") {
		t.Errorf("legacy info string was read: values %v, body %q", res.Values, res.Body)
	}
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

func TestOtherFenceLanguagesAreBody(t *testing.T) {
	env := newEnv(t, testConfiguration)
	res := resolveOK(t, env.input("```json\n{\"key\": 1}\n```\n"))
	if !strings.Contains(string(res.Body), `"key": 1`) {
		t.Errorf("body = %q", res.Body)
	}
}

func TestUnclosedBlockIsAnError(t *testing.T) {
	env := newEnv(t, testConfiguration)
	errs := resolveErrors(t, env.input("```yaml\nsubject: x\n"))
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
