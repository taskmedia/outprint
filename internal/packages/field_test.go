package packages

import (
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func parseField(t *testing.T, name, src string) (Field, []string) {
	t.Helper()
	var node yaml.Node
	if err := yaml.Unmarshal([]byte(src), &node); err != nil {
		t.Fatal(err)
	}
	return parseFieldNode(name, node.Content[0])
}

func TestFieldCheck(t *testing.T) {
	color, _ := parseField(t, "c", "type: color\npattern: '^[0-9A-Fa-f]{6}$'")
	enum, _ := parseField(t, "e", "type: enum\nvalues: [left, right]")
	asset, _ := parseField(t, "a", `{type: asset, pattern: '^([A-Za-z0-9_-]+/)*[A-Za-z0-9_-]+\.svg$'}`)
	list, _ := parseField(t, "l", "type: list")
	boolean, _ := parseField(t, "b", "type: bool")
	date, _ := parseField(t, "d", "type: date")
	str, _ := parseField(t, "s", "type: string\npattern: '[a-z]+'")

	ok := []struct {
		f Field
		v any
	}{
		{color, "2F6F5E"}, {enum, "left"}, {asset, "img/logo.svg"},
		{list, []any{"a", "b"}}, {boolean, true}, {date, "2 March 2026"},
		{date, time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC)}, {str, "abc"},
	}
	for _, tc := range ok {
		if err := tc.f.Check(tc.v); err != nil {
			t.Errorf("%s %v: unexpected error %v", tc.f.Name, tc.v, err)
		}
	}
	bad := []struct {
		f Field
		v any
	}{
		{color, "red}\\directlua{x}"}, {color, "2F6F5"}, {color, 5}, {enum, "up"}, {enum, 3},
		{asset, "../secret.svg"}, {asset, "/etc/passwd"}, {asset, "a.svg\n\\x"},
		{list, "single"}, {list, []any{"a", 3}}, {boolean, "yes"}, {date, 20260302},
		{str, "ABC"}, {str, 1},
	}
	for _, tc := range bad {
		if err := tc.f.Check(tc.v); err == nil {
			t.Errorf("%s %#v: expected error", tc.f.Name, tc.v)
		}
	}
}

func TestListPatternAppliesToEveryItem(t *testing.T) {
	f, problems := parseField(t, "l", "type: list\npattern: '[a-z]+'")
	if len(problems) != 0 {
		t.Fatal(problems)
	}
	if err := f.Check([]any{"ok", "NOT"}); err == nil {
		t.Fatal("second item should fail the pattern")
	}
}

// Security layer 5: hostile field definitions must be load errors.
func TestHostileFieldDefinitionsAreRejected(t *testing.T) {
	cases := map[string]struct{ src, want string }{
		"color without pattern":       {"type: color", "pattern"},
		"asset without pattern":       {"type: asset", "pattern"},
		"enum without values":         {"type: enum", "values"},
		"enum value with brace":       {"type: enum\nvalues: ['a}\\directlua{x}']", "value"},
		"enum with pattern":           {"type: enum\nvalues: [a]\npattern: '^a$'", "values"},
		"values on string":            {"type: string\nvalues: [a]", "values"},
		"color default injects":       {"type: color\npattern: '^[0-9A-Fa-f]{6}$'\ndefault: 'red}\\directlua{os.execute(1)}'", "default"},
		"asset default traversal":     {`{type: asset, pattern: '^[A-Za-z0-9_./-]+$', default: '../../etc/passwd'}`, "pattern"},
		"asset default in pattern":    {`{type: asset, pattern: '^([A-Za-z0-9_-]+/)*[A-Za-z0-9_-]+\.svg$', default: '../x.svg'}`, "default"},
		"enum default not listed":     {"type: enum\nvalues: [a, b]\ndefault: c", "default"},
		"bool default string":         {"type: bool\ndefault: 'true; \\x'", "default"},
		"permissive color":            {"type: color\npattern: '.*'", "permissive"},
		"permissive dotall":           {"type: color\npattern: '^.+$'", "permissive"},
		"allows backslash":            {"type: color\npattern: '^[^ ]+$'", "permissive"},
		"allows braces":               {"type: color\npattern: '^[a-z{}]+$'", "permissive"},
		"allows percent":              {"type: color\npattern: '^[a-z%]+$'", "permissive"},
		"allows dollar":               {"type: color\npattern: '^[a-z$]+$'", "permissive"},
		"allows newline":              {"type: color\npattern: '^[a-z\\n]+$'", "permissive"},
		"asset allows dotdot":         {"type: asset\npattern: '^[a-z./]+$'", "permissive"},
		"asset allows absolute":       {"type: asset\npattern: '^/?[a-z/]+$'", "permissive"},
		"alternation escapes anchors": {"type: color\npattern: '^[0-9a-f]{6}$|.*'", "permissive"},
		"broken regexp":               {"type: color\npattern: '^[0-9'", "pattern"},
		"unknown type":                {"type: length", "type"},
		"missing type":                {"description: x", "type"},
		"unknown key":                 {"type: string\nbogus: 1", "bogus"},
		"pattern not a string":        {"type: string\npattern: [a]", "cannot unmarshal"},
		"huge pattern":                {"type: color\npattern: '^" + strings.Repeat("a", 600) + "$'", "long"},
	}
	for name, tc := range cases {
		_, problems := parseField(t, "field", tc.src)
		if len(problems) == 0 {
			t.Errorf("%s: expected a problem", name)
			continue
		}
		if !strings.Contains(strings.Join(problems, "; "), tc.want) {
			t.Errorf("%s: problems %q do not mention %q", name, problems, tc.want)
		}
	}
}

func TestStrictPatternAcceptsShippedShapes(t *testing.T) {
	for _, src := range []string{
		"type: color\npattern: '^[0-9A-Fa-f]{6}$'",
		"type: color\npattern: '^(red|green|blue)$'",
		`{type: asset, pattern: '^([A-Za-z0-9_-]+/)*[A-Za-z0-9_-]+(\.[A-Za-z0-9_-]+)*\.svg$'}`,
		"type: string\npattern: '^[0-9]+(\\.[0-9]+)?(mm|cm|pt)$'",
	} {
		f, problems := parseField(t, "field", src)
		if len(problems) != 0 {
			t.Errorf("%q: %v", src, problems)
		}
		if p := f.strictPatternProblem(); p != "" {
			t.Errorf("%q: %s", src, p)
		}
	}
}

func TestFieldDefaultPresence(t *testing.T) {
	f, problems := parseField(t, "f", "type: string\ndefault: ''")
	if len(problems) != 0 || !f.HasDefault || f.Default != "" {
		t.Fatalf("%+v %v", f, problems)
	}
	f, _ = parseField(t, "f", "type: string")
	if f.HasDefault {
		t.Fatal("no default expected")
	}
}

func TestFieldMustBeMapping(t *testing.T) {
	if _, problems := parseField(t, "f", "string"); len(problems) == 0 {
		t.Fatal("expected a problem for a scalar field definition")
	}
}
