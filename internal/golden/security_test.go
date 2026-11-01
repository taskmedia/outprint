//go:build golden

// Security layer 1: a corpus of hostile Documents rendered through the real
// outprint binary. Whatever the payload, the render must not read a file the
// Document names, must not run Lua or a shell, and must not hand raw TeX to
// the engine. Each payload class has one deterministic outcome:
//
//	literal  the payload is typeset as visible text
//	dropped  the payload is removed; the text around it stays
//	rejected the render fails with a structured error naming phase and kind
//
// There is no skipping in this file: a missing tool is a failure.
package golden

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

type outcome int

const (
	literal outcome = iota
	dropped
	rejected
)

func (o outcome) String() string { return [...]string{"literal", "dropped", "rejected"}[o] }

const (
	canaryPlaceholder = "{{CANARY}}"
	markerPlaceholder = "{{MARKER}}"
)

type hostileCase struct {
	name     string
	template string // Document Type: "letter" or "report"
	// markdown is the whole Document, Metadata Block included. Placeholders
	// are replaced with a fresh canary path and marker path.
	markdown string
	want     outcome
	// visible must appear in the PDF text (whitespace collapsed) for literal
	// and dropped outcomes.
	visible []string
	// absent must not appear in the PDF text.
	absent []string
	// phase and kind are expected for rejected outcomes.
	phase, kind string
	// imageKind, for a rejected outcome, is the image problem the render
	// command reports before the Worker runs: the pipeline stops these
	// payloads ahead of the Worker's own filter, which the Worker tests
	// cover with the same payloads.
	imageKind string
	// noLinkTo lists link targets that must not exist in the PDF.
	noLinkTo []string
}

// The TeX payloads, used in the body and in metadata values.
const (
	payloadInput     = `\input{{{CANARY}}}`
	payloadDirectlua = `\directlua{local f = io.open("{{MARKER}}", "w") if f then f:write("x") f:close() end}`
	payloadWrite18   = `\immediate\write18{touch {{MARKER}}}`
)

func hostileCases() []hostileCase {
	cases := []hostileCase{
		{name: "body raw input", template: "letter", want: literal,
			markdown: "Before " + payloadInput + " after\n", visible: []string{`\input`, "Before", "after"}},
		{name: "body raw directlua", template: "letter", want: literal,
			markdown: "Before " + payloadDirectlua + " after\n", visible: []string{`\directlua`, "after"}},
		{name: "body raw write18", template: "letter", want: literal,
			markdown: "Before " + payloadWrite18 + " after\n", visible: []string{`\write18`, "after"}},
		{name: "body raw environment", template: "letter", want: literal,
			markdown: "\\begin{verbatim}\n" + payloadInput + "\n\\end{verbatim}\n", visible: []string{`\begin`, `\input`}},
		{name: "math input inline", template: "letter", want: literal,
			markdown: "Before $" + payloadInput + "$ after\n", visible: []string{`\input`, "after"}},
		{name: "math directlua inline", template: "letter", want: literal,
			markdown: "Before $" + payloadDirectlua + "$ after\n", visible: []string{`\directlua`, "after"}},
		{name: "math display", template: "letter", want: literal,
			markdown: "Before\n\n$$\n" + payloadInput + "\n$$\n\nafter\n", visible: []string{`\input`, "after"}},
		{name: "raw attribute block", template: "letter", want: literal,
			markdown: "```{=latex}\n" + payloadInput + "\n" + payloadDirectlua + "\n```\n\nafter\n", visible: []string{`\input`, "after"}},
		{name: "raw attribute inline", template: "letter", want: literal,
			markdown: "Before `" + payloadInput + "`{=latex} after\n", visible: []string{`\input`, "after"}},
		{name: "raw attribute tex", template: "letter", want: literal,
			markdown: "```{=tex}\n" + payloadWrite18 + "\n```\n\nafter\n", visible: []string{`\write18`, "after"}},
		{name: "fenced code with TeX", template: "letter", want: literal,
			markdown: "```tex\n" + payloadInput + "\n" + payloadWrite18 + "\n```\n\nafter\n", visible: []string{`\input`, `\write18`, "after"}},
		{name: "code span with TeX", template: "letter", want: literal,
			markdown: "Before `" + payloadDirectlua + "` after\n", visible: []string{`\directlua`, "after"}},
		{name: "html script", template: "letter", want: dropped,
			markdown: "Before\n\n<script>document.write(\"SCRIPT-BODY\")</script>\n\nafter\n",
			visible:  []string{"Before", "after"}, absent: []string{"SCRIPT-BODY", "document.write"}},
		{name: "html comment with TeX", template: "letter", want: dropped,
			markdown: "Before <!-- " + payloadInput + " --> after\n", visible: []string{"Before", "after"}, absent: []string{`\input`, "CANARY-"}},
		{name: "html iframe and object", template: "letter", want: dropped,
			markdown: "Before\n\n<iframe src=\"file://{{CANARY}}\"></iframe>\n\n<object data=\"file://{{CANARY}}\"></object>\n\nafter\n",
			visible:  []string{"Before", "after"}, absent: []string{"iframe", "object", "CANARY-"}},
		{name: "html inline tags", template: "letter", want: dropped,
			markdown: "Before <b onclick=\"x()\">bold</b> and <img src=\"file://{{CANARY}}\"> after\n",
			visible:  []string{"Before bold and", "after"}, absent: []string{"onclick", "<img", "CANARY-"}},
		{name: "html div keeps its text", template: "letter", want: dropped,
			markdown: "<div style=\"x\">Text in a div</div>\n", visible: []string{"Text in a div"}, absent: []string{"style"}},
		{name: "link javascript", template: "letter", want: literal,
			markdown: "Click [the link text](javascript:alert(1)) now\n", visible: []string{"Click the link text now"},
			noLinkTo: []string{"javascript:"}},
		{name: "link file", template: "letter", want: literal,
			markdown: "Open [the file](file://{{CANARY}}) now\n", visible: []string{"Open the file now"},
			noLinkTo: []string{"file:", "CANARY"}},
		{name: "link data and vbscript", template: "letter", want: literal,
			markdown: "[one](data:text/html;base64,PHNjcmlwdD4=) and [two](VBScript:x) and [three](/doc/internal-path)\n",
			visible:  []string{"one and two and three"}, noLinkTo: []string{"data:", "vbscript:", "internal-path"}},
		{name: "image parent path", template: "letter", want: rejected, imageKind: "forbidden",
			markdown: "![](../../etc/passwd)\n"},
		{name: "image absolute canary path", template: "letter", want: rejected, imageKind: "forbidden",
			markdown: "![alt]({{CANARY}})\n"},
		{name: "image file url", template: "letter", want: rejected, imageKind: "forbidden",
			markdown: "![alt](file://{{CANARY}})\n"},
		{name: "image remote url", template: "letter", want: rejected, imageKind: "forbidden",
			markdown: "![alt](https://example.org/x.png)\n"},
		{name: "image not in the bundle", template: "letter", want: rejected, imageKind: "missing",
			markdown: "![alt](images/never-uploaded.png)\n"},
		{name: "image reference style", template: "letter", want: rejected, imageKind: "forbidden",
			markdown: "![alt][r]\n\n[r]: ../../../../etc/hostname\n"},
	}
	return append(cases, metadataCases()...)
}

// metadataCases put the payloads into Metadata Block values: plain-text
// fields are escaped and shown literally; a text field is Markdown, so raw
// HTML in it is dropped like it is in the body.
func metadataCases() []hostileCase {
	payloads := []struct {
		name, payload, shown string
	}{
		{"input", payloadInput, `\input`},
		{"math input", "$" + payloadInput + "$", `\input`},
		{"directlua", payloadDirectlua, `\directlua`},
		{"write18", payloadWrite18, `\write18`},
		{"html", `<script>alert("META-SCRIPT")</script>`, "META-SCRIPT"},
	}
	var cases []hostileCase
	for _, p := range payloads {
		for _, field := range []string{"subject", "opening", "author"} {
			cases = append(cases, hostileCase{
				name: "letter " + field + " " + p.name, template: "letter", want: literal,
				markdown: "```yaml\n" + field + ": " + yamlQuote(p.payload) + "\n```\nBody text.\n",
				visible:  []string{p.shown, "Body text."},
			})
		}
		abstract := hostileCase{
			name: "report abstract text " + p.name, template: "report", want: literal,
			markdown: "```yaml\ntype: report\ntitle: Report\nabstract: " + yamlQuote("Summary "+p.payload+" end") + "\n```\nBody text.\n",
			visible:  []string{"Summary", "end", "Body text."},
		}
		if p.name == "html" {
			abstract.want, abstract.absent = dropped, []string{"META-SCRIPT"}
		} else {
			abstract.visible = append(abstract.visible, p.shown)
		}
		cases = append(cases, hostileCase{
			name: "letter recipient list " + p.name, template: "letter", want: literal,
			markdown: "```yaml\nrecipient:\n  - " + yamlQuote("Line one") + "\n  - " + yamlQuote(p.payload) + "\n```\nBody text.\n",
			visible:  []string{"Line one", p.shown, "Body text."},
		}, hostileCase{
			name: "report title " + p.name, template: "report", want: literal,
			markdown: "```yaml\ntype: report\ntitle: " + yamlQuote(p.payload) + "\n```\nBody text.\n",
			visible:  []string{p.shown, "Body text."},
		}, abstract)
	}
	return cases
}

func yamlQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }

func TestSecurityHostileDocuments(t *testing.T) {
	root := repoRoot(t)
	for _, tool := range []string{"pdftotext", "pdftohtml"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Fatalf("%s not found: the security suite must run in the outprint image (make test-golden)", tool)
		}
	}
	for _, tc := range hostileCases() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runHostileCase(t, root, tc)
		})
	}
}

type hostileRun struct {
	canaryPath, canarySecret, markerPath string
	pdf                                  string
	stdout, stderr                       string
	exitErr                              error
}

func runHostileCase(t *testing.T, root string, tc hostileCase) {
	t.Helper()
	run := renderHostile(t, root, tc)

	if _, err := os.Stat(run.markerPath); err == nil {
		t.Fatalf("a payload ran Lua or a shell: %s was created", run.markerPath)
	}
	for name, text := range map[string]string{"stdout": run.stdout, "stderr": run.stderr} {
		if strings.Contains(text, run.canarySecret) {
			t.Errorf("%s contains the canary content", name)
		}
	}
	switch tc.want {
	case rejected:
		assertRejected(t, run, tc)
	default:
		assertRendered(t, run, tc)
	}
}

func renderHostile(t *testing.T, root string, tc hostileCase) hostileRun {
	t.Helper()
	dir := t.TempDir()
	run := hostileRun{
		canaryPath:   filepath.Join(dir, "canary", "canary.txt"),
		canarySecret: "CANARY-" + randomToken(t),
		markerPath:   filepath.Join(dir, "marker-"+randomToken(t)),
	}
	if err := os.MkdirAll(filepath.Dir(run.canaryPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(run.canaryPath, []byte(run.canarySecret+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	markdown := strings.NewReplacer(canaryPlaceholder, run.canaryPath, markerPlaceholder, run.markerPath).Replace(tc.markdown)
	input := filepath.Join(dir, "input.md")
	if err := os.WriteFile(input, []byte(markdown), 0o644); err != nil {
		t.Fatal(err)
	}
	run.pdf = filepath.Join(dir, "out.pdf")
	bin := os.Getenv("OUTPRINT_BIN")
	if bin == "" {
		bin = "outprint"
	}
	cmd := exec.Command(bin, "render", "--template", "neutral", "--config", "example", "--date", fixedDate, "-o", run.pdf, input)
	var stdout, stderr strings.Builder
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	run.exitErr = cmd.Run()
	run.stdout, run.stderr = stdout.String(), stderr.String()
	return run
}

func assertRejected(t *testing.T, run hostileRun, tc hostileCase) {
	t.Helper()
	if run.exitErr == nil {
		t.Fatalf("the render succeeded; want a structured rejection (phase %s, kind %s)", tc.phase, tc.kind)
	}
	want := fmt.Sprintf("render failed in phase %s (%s), job ", tc.phase, tc.kind)
	if tc.imageKind != "" {
		want = "(" + tc.imageKind + ")"
	}
	if !strings.Contains(run.stderr, want) {
		t.Errorf("stderr lacks %q:\n%s", want, run.stderr)
	}
	if _, err := os.Stat(run.pdf); err == nil {
		t.Error("a PDF was written for a rejected render")
	}
	for _, leak := range []string{"\\input", "Undefined control sequence", "LuaHBTeX", "This is "} {
		if strings.Contains(run.stderr, leak) {
			t.Errorf("stderr looks like a TeX log (%q):\n%s", leak, run.stderr)
		}
	}
}

var whitespace = regexp.MustCompile(`\s+`)

func assertRendered(t *testing.T, run hostileRun, tc hostileCase) {
	t.Helper()
	if run.exitErr != nil {
		t.Fatalf("the render failed (%s expected): %v\n%s", tc.want, run.exitErr, run.stderr)
	}
	text := whitespace.ReplaceAllString(tool(t, "pdftotext", "-layout", run.pdf, "-"), " ")
	if strings.Contains(text, run.canarySecret) {
		t.Fatalf("the PDF text contains the canary content")
	}
	for _, v := range tc.visible {
		if !strings.Contains(text, v) {
			t.Errorf("PDF text lacks %q:\n%s", v, text)
		}
	}
	for _, a := range tc.absent {
		if strings.Contains(text, a) {
			t.Errorf("PDF text contains %q", a)
		}
	}
	if len(tc.noLinkTo) > 0 {
		links := tool(t, "pdftohtml", "-xml", "-i", "-stdout", run.pdf)
		for _, target := range tc.noLinkTo {
			if strings.Contains(strings.ToLower(links), strings.ToLower(target)) {
				t.Errorf("the PDF links to %q", target)
			}
		}
	}
}

func randomToken(t *testing.T) string {
	t.Helper()
	f, err := os.Open("/dev/urandom")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	b := make([]byte, 8)
	if _, err := f.Read(b); err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("%x", b)
}

// TestSecurityLinkControl proves that the link check above can see links: an
// https link must show up in the pdftohtml output.
func TestSecurityLinkControl(t *testing.T) {
	root := repoRoot(t)
	run := renderHostile(t, root, hostileCase{template: "letter", want: literal,
		markdown: "A [good link](https://example.org/page) here\n"})
	if run.exitErr != nil {
		t.Fatalf("render failed: %v\n%s", run.exitErr, run.stderr)
	}
	if links := tool(t, "pdftohtml", "-xml", "-i", "-stdout", run.pdf); !strings.Contains(links, "https://example.org/page") {
		t.Fatalf("pdftohtml does not report the https link, so the negative link checks prove nothing:\n%s", links)
	}
}
