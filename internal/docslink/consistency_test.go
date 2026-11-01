package docslink

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const repoRoot = "../.."

var headingRE = regexp.MustCompile("(?m)^### `([a-z_][a-z0-9_]*)`")

// sourceCodes asks scripts/docs-error-codes.sh for the codes the product can
// put in an error. The script is the one place that knows which source files
// and constants hold them; this test reuses it instead of deriving the list a
// second time.
func sourceCodes(t *testing.T) []string {
	t.Helper()
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not available; scripts/docs-error-codes.sh cannot run")
	}
	cmd := exec.Command(bash, "scripts/docs-error-codes.sh", "list")
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("scripts/docs-error-codes.sh list: %v", err)
	}
	codes := strings.Fields(string(out))
	if len(codes) < 10 {
		t.Fatalf("the script listed only %d codes (%q); its patterns no longer match the source", len(codes), codes)
	}
	return codes
}

func TestErrorsPageExists(t *testing.T) {
	file := filepath.Join(repoRoot, "docs", filepath.FromSlash(ErrorsPage)+".md")
	page, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("ErrorsPage %q does not lead to a docs page: %v", ErrorsPage, err)
	}
	if regexp.MustCompile(`(?m)^slug:`).Match(page) {
		t.Errorf("%s sets a slug, so its URL is no longer %q", file, ErrorsPage)
	}
}

// The button only renders links to the docs origin; it keeps its own copy of
// the constant so that it needs no URL logic.
func TestButtonTrustsTheSameOriginAndPage(t *testing.T) {
	script, err := os.ReadFile(filepath.Join(repoRoot, "web", "button.js"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"var DOCS_ORIGIN = '" + BaseURL + "';",
		"var DOCS_PAGE_SUFFIX = '/" + ErrorsPage + "/';",
	} {
		if !strings.Contains(string(script), want) {
			t.Errorf("web/button.js lacks %q", want)
		}
	}
}

func TestEveryCodeInTheSourceHasAnAnchorTheLinkPointsAt(t *testing.T) {
	page, err := os.ReadFile(filepath.Join(repoRoot, "docs", filepath.FromSlash(ErrorsPage)+".md"))
	if err != nil {
		t.Fatal(err)
	}
	anchors := map[string]bool{}
	for _, m := range headingRE.FindAllStringSubmatch(string(page), -1) {
		anchors[m[1]] = true
	}
	for _, code := range sourceCodes(t) {
		if !anchors[code] {
			t.Errorf("code %q is in the source but has no heading in %s.md", code, ErrorsPage)
		}
		for _, version := range []string{"0.3.2", "dev", ""} {
			url := ErrorURL(version, code)
			if !strings.HasSuffix(url, "/"+ErrorsPage+"/#"+code) {
				t.Errorf("ErrorURL(%q, %q) = %q, which does not end in the page and the anchor", version, code, url)
			}
		}
	}
}
