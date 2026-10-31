package binding

import (
	"strings"
	"testing"

	"github.com/taskmedia/outline-latex-pdf/internal/config"
	"github.com/taskmedia/outline-latex-pdf/internal/metadata"
	"github.com/taskmedia/outline-latex-pdf/internal/startup"
)

var neutralExample = config.Binding{Template: "neutral", Configuration: "example"}

func registry(t *testing.T, withDefault bool) Registry {
	t.Helper()
	svc := config.Service{
		TemplatesRoot: "../../templates", ConfigurationsRoot: "../../configurations",
		Bindings: []config.Binding{neutralExample},
	}
	if withDefault {
		svc.DefaultBinding = &neutralExample
	}
	started, err := startup.FromService(svc)
	if err != nil {
		t.Fatal(err)
	}
	return started.Registry
}

func block(body string) string { return "```yaml binding\n" + body + "\n```\n" }

const validBody = "template: neutral\nconfiguration: example"

func TestValidBlock(t *testing.T) {
	out, errs := Resolve(block(validBody), registry(t, false))
	if errs != nil || out.Kind != FromBlock || out.Binding != neutralExample {
		t.Errorf("out = %+v, errs = %v", out, errs)
	}
}

func TestBlockAnywhereInTheOverview(t *testing.T) {
	tests := map[string]string{
		"middle":         "# Team\n\nIntro text.\n\n" + block(validBody) + "\nMore text.\n",
		"end":            "# Team\n\nIntro text.\n\n" + block(validBody),
		"tilde":          "~~~yaml binding\n" + validBody + "\n~~~\n",
		"long fence":     "````yaml binding\n" + validBody + "\n````\n",
		"indented":       "   ```yaml binding\n" + validBody + "\n   ```\n",
		"crlf":           strings.ReplaceAll(block(validBody), "\n", "\r\n"),
		"spaced":         "```  yaml   binding  \n" + validBody + "\n```\n",
		"quoted":         block("template: \"neutral\"\nconfiguration: 'example'"),
		"closing longer": "```yaml binding\n" + validBody + "\n`````\n",
	}
	for name, overview := range tests {
		t.Run(name, func(t *testing.T) {
			out, errs := Resolve(overview, registry(t, false))
			if errs != nil || out.Kind != FromBlock || out.Binding != neutralExample {
				t.Errorf("out = %+v, errs = %v", out, errs)
			}
		})
	}
}

func TestBindingTextThatIsNotABlock(t *testing.T) {
	tests := map[string]string{
		"inline code":        "Use `yaml binding` blocks.\n",
		"inline triple":      "```yaml binding``` is how it starts.\n",
		"other language":     "```markdown\n```yaml binding\n" + validBody + "\n```\n```\n",
		"inside longer":      "````text\n```yaml binding\n" + validBody + "\n```\n````\n",
		"inside tilde":       "~~~\n```yaml binding\n" + validBody + "\n```\n~~~\n",
		"indented code":      "    ```yaml binding\n    " + validBody + "\n    ```\n",
		"different info":     "```yaml\n" + validBody + "\n```\n",
		"binding only":       "```binding\n" + validBody + "\n```\n",
		"yaml metadata only": "```yaml metadata\n" + validBody + "\n```\n",
	}
	for name, overview := range tests {
		t.Run(name, func(t *testing.T) {
			out, errs := Resolve(overview, registry(t, true))
			if errs != nil || out.Kind != UseDefault {
				t.Errorf("out = %+v, errs = %v", out, errs)
			}
		})
	}
}

func TestFirstBlockWinsAndSecondIsAnError(t *testing.T) {
	overview := "Intro\n\n" + block(validBody) + "\nText\n\n" + block("template: other\nconfiguration: x")
	out, errs := Resolve(overview, registry(t, true))
	wantErrors(t, errs, wantError{code: CodeDuplicate, line: 10})
	if out.Kind != FromBlock {
		t.Errorf("kind = %v, want the block to count", out.Kind)
	}
}

func TestSecondBlockInsideOtherFenceIsNotCounted(t *testing.T) {
	overview := block(validBody) + "\n````text\n" + block(validBody) + "````\n"
	if _, errs := Resolve(overview, registry(t, false)); errs != nil {
		t.Errorf("errs = %v", errs)
	}
}

func TestStructuralErrors(t *testing.T) {
	tests := []struct {
		name string
		body string
		want []wantError
	}{
		{"extra key", validBody + "\nfont: serif", []wantError{{CodeMalformed, "font", 6}}},
		{"missing configuration", "template: neutral", []wantError{{CodeMalformed, "configuration", 3}}},
		{"missing both", "# nothing", []wantError{{CodeMalformed, "template", 3}, {CodeMalformed, "configuration", 3}}},
		{"empty block", "", []wantError{{CodeMalformed, "template", 3}, {CodeMalformed, "configuration", 3}}},
		{"number", "template: 42\nconfiguration: example", []wantError{{CodeMalformed, "template", 4}}},
		{"null", "template:\nconfiguration: example", []wantError{{CodeMalformed, "template", 4}}},
		{"empty string", "template: \"\"\nconfiguration: example", []wantError{{CodeMalformed, "template", 4}}},
		{"list", "template: [neutral]\nconfiguration: example", []wantError{{CodeMalformed, "template", 4}}},
		{"duplicate key", "template: neutral\ntemplate: neutral\nconfiguration: example", []wantError{{CodeMalformed, "template", 5}}},
		{"not a mapping", "- neutral\n- example", []wantError{{CodeMalformed, "", 4}}},
		{"broken yaml", "template: [neutral\nconfiguration: example", []wantError{{CodeMalformed, "", 0}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, errs := Resolve("Intro\n\n```yaml binding\n"+tt.body+"\n```\n", registry(t, true))
			if out.Kind != FromBlock {
				t.Errorf("kind = %v: a broken block must never fall back", out.Kind)
			}
			wantErrors(t, errs, tt.want...)
		})
	}
}

func TestUnclosedBlock(t *testing.T) {
	_, errs := Resolve("Intro\n```yaml binding\n"+validBody+"\n", registry(t, true))
	wantErrors(t, errs, wantError{code: CodeMalformed, line: 2})
}

func TestUnknownNames(t *testing.T) {
	_, errs := Resolve(block("template: nope\nconfiguration: example"), registry(t, true))
	wantErrors(t, errs, wantError{CodeUnknown, "template", 2})
	_, errs = Resolve(block("template: neutral\nconfiguration: nope"), registry(t, true))
	wantErrors(t, errs, wantError{CodeUnknown, "configuration", 3})
	_, errs = Resolve(block("template: nope\nconfiguration: nada"), registry(t, true))
	wantErrors(t, errs, wantError{CodeUnknown, "template", 2}, wantError{CodeUnknown, "configuration", 3})
}

func TestPairNotOnAllowList(t *testing.T) {
	svc := config.Service{
		TemplatesRoot: "../../templates", ConfigurationsRoot: "../../configurations",
		Bindings: []config.Binding{neutralExample},
	}
	started, err := startup.FromService(svc)
	if err != nil {
		t.Fatal(err)
	}
	reg := fakeRegistry{Registry: started.Registry, deny: true}
	out, errs := Resolve(block(validBody), reg)
	wantErrors(t, errs, wantError{code: CodeNotAllowed, line: 2})
	if out.Kind != FromBlock {
		t.Errorf("kind = %v", out.Kind)
	}
}

// fakeRegistry knows every package but allows no pairing.
type fakeRegistry struct {
	Registry
	deny bool
}

func (f fakeRegistry) Allowed(config.Binding) bool { return !f.deny }

func TestNoBlockUsesDefault(t *testing.T) {
	out, errs := Resolve("# Team\n\nJust text.\n", registry(t, true))
	if errs != nil || out.Kind != UseDefault || out.Binding != neutralExample {
		t.Errorf("out = %+v, errs = %v", out, errs)
	}
}

func TestNoBlockNoDefault(t *testing.T) {
	out, errs := Resolve("", registry(t, false))
	if errs != nil || out.Kind != NoBinding {
		t.Errorf("out = %+v, errs = %v", out, errs)
	}
}

func TestBrokenBlockNeverFallsBackToDefault(t *testing.T) {
	out, errs := Resolve(block("template: neutral"), registry(t, true))
	if errs == nil || out.Kind == UseDefault || out.Binding != (config.Binding{}) {
		t.Errorf("out = %+v, errs = %v", out, errs)
	}
}

type wantError struct {
	code  string
	field string
	line  int // 0 = do not check
}

func wantErrors(t *testing.T, got *metadata.ErrorList, want ...wantError) {
	t.Helper()
	if got == nil {
		t.Fatalf("no errors, want %+v", want)
	}
	if len(got.Errors) != len(want) {
		t.Fatalf("errors = %v, want %d: %+v", got.Errors, len(want), want)
	}
	for i, w := range want {
		e := got.Errors[i]
		if e.Source != metadata.SourceCollection || e.Code != w.code || e.Field != w.field || e.Line == nil || e.Message == "" {
			t.Errorf("error %d = %+v, want source collection, %+v", i, e, w)
		}
		if w.line != 0 && e.Line != nil && *e.Line != w.line {
			t.Errorf("error %d line = %d, want %d (%s)", i, *e.Line, w.line, e.Message)
		}
	}
}
