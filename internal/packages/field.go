package packages

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// FieldType is the type of a field value.
type FieldType string

const (
	TypeString FieldType = "string"
	TypeText   FieldType = "text"
	TypeList   FieldType = "list"
	TypeDate   FieldType = "date"
	TypeBool   FieldType = "bool"
	TypeEnum   FieldType = "enum"
	TypeColor  FieldType = "color"
	TypeAsset  FieldType = "asset"
)

var fieldTypes = []FieldType{TypeString, TypeText, TypeList, TypeDate, TypeBool, TypeEnum, TypeColor, TypeAsset}

const (
	maxPatternLength = 512
	fieldNameMaxLen  = 64
)

var (
	fieldNameRE = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	enumValueRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 ._-]*$`)
	// a package-relative path made of plain segments
	relativePathRE = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9._-]*(/[A-Za-z0-9_][A-Za-z0-9._-]*)*$`)
)

// Field is a declared field of a Template. The manifest is its single
// source of truth: type, whether it is required, its default and, for
// non-text positions, a typed pattern or the allowed enum values.
type Field struct {
	Name        string
	Type        FieldType
	Required    bool
	Description string
	Default     any
	HasDefault  bool
	Pattern     string
	Values      []string

	matcher *regexp.Regexp
}

// IsNonText reports whether the type is one that lands in non-text
// positions and therefore always needs a strict pattern or value list.
func (t FieldType) IsNonText() bool {
	return t == TypeColor || t == TypeAsset || t == TypeEnum
}

func (t FieldType) valid() bool { return slices.Contains(fieldTypes, t) }

// rawField mirrors the manifest. Decoding goes through a yaml.Node because
// strictness and default presence cannot be expressed with struct tags.
type rawField struct {
	Type        string   `yaml:"type"`
	Required    bool     `yaml:"required"`
	Description string   `yaml:"description"`
	Default     any      `yaml:"default"`
	Pattern     string   `yaml:"pattern"`
	Values      []string `yaml:"values"`
}

var fieldKeys = []string{"type", "required", "description", "default", "pattern", "values"}

// parseFieldNode decodes and validates one field definition. It returns
// every problem found; the Field is usable only if there are none.
func parseFieldNode(name string, node *yaml.Node) (Field, []string) {
	f := Field{Name: name}
	var problems []string
	if len(name) > fieldNameMaxLen || !fieldNameRE.MatchString(name) {
		problems = append(problems, fmt.Sprintf("invalid field name %q (must match %s)", name, fieldNameRE))
	}
	if node.Kind != yaml.MappingNode {
		return f, append(problems, "field definition must be a mapping")
	}
	problems = append(problems, unknownKeys(node, fieldKeys)...)
	var raw rawField
	if err := node.Decode(&raw); err != nil {
		return f, append(problems, fmt.Sprintf("field definition: %v", err))
	}
	f.Type, f.Required, f.Description = FieldType(raw.Type), raw.Required, raw.Description
	f.Default, f.Pattern, f.Values = raw.Default, raw.Pattern, raw.Values
	f.HasDefault = hasKey(node, "default")
	return f, append(problems, f.validateDefinition()...)
}

func unknownKeys(node *yaml.Node, allowed []string) []string {
	var problems []string
	for i := 0; i+1 < len(node.Content); i += 2 {
		if key := node.Content[i].Value; !slices.Contains(allowed, key) {
			problems = append(problems, fmt.Sprintf("unknown key %q", key))
		}
	}
	return problems
}

func hasKey(node *yaml.Node, key string) bool {
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return true
		}
	}
	return false
}

func (f *Field) validateDefinition() []string {
	if !f.Type.valid() {
		return []string{fmt.Sprintf("type %q is not one of %s", f.Type, joinTypes())}
	}
	var problems []string
	problems = append(problems, f.validateValues()...)
	problems = append(problems, f.validatePattern()...)
	if len(problems) == 0 && f.HasDefault {
		if err := f.Check(f.Default); err != nil {
			problems = append(problems, fmt.Sprintf("default: %v", err))
		}
	}
	return problems
}

func (f *Field) validateValues() []string {
	if f.Type != TypeEnum {
		if len(f.Values) > 0 {
			return []string{fmt.Sprintf("values are only allowed for type enum, not %s", f.Type)}
		}
		return nil
	}
	var problems []string
	if len(f.Values) == 0 {
		problems = append(problems, "type enum requires a list of values")
	}
	for _, v := range f.Values {
		if !enumValueRE.MatchString(v) {
			problems = append(problems, fmt.Sprintf("enum value %q must match %s", sanitizeForMessage(v), enumValueRE))
		}
	}
	if f.Pattern != "" {
		problems = append(problems, "type enum is constrained by values, not by a pattern")
	}
	return problems
}

func (f *Field) validatePattern() []string {
	if f.Pattern == "" {
		if f.Type == TypeColor || f.Type == TypeAsset {
			return []string{fmt.Sprintf("type %s requires a typed pattern", f.Type)}
		}
		return nil
	}
	if len(f.Pattern) > maxPatternLength {
		return []string{fmt.Sprintf("pattern is too long (max %d characters)", maxPatternLength)}
	}
	re, err := regexp.Compile(`\A(?:` + f.Pattern + `)\z`)
	if err != nil {
		return []string{fmt.Sprintf("pattern does not compile: %v", err)}
	}
	f.matcher = re
	if f.Type.IsNonText() {
		if p := f.strictPatternProblem(); p != "" {
			return []string{p}
		}
	}
	return nil
}

// hostileProbes are strings no legitimate colour, length or file name
// contains. A pattern that matches any of them could let TeX syntax or a
// path escape through.
func hostileProbes() []string {
	specials := []string{`\`, `{`, `}`, `%`, `$`, `&`, `#`, `~`, `^`, `|`, "`", `"`, "\n", "\r", "\x00"}
	var probes []string
	for _, c := range specials {
		probes = append(probes, c, "a"+c+"b", "aaaaaa"+c, "a"+c+"b.svg", "1"+c+"1mm")
	}
	return append(probes, "..", "../a", "a/..", "a/../b", "/a", "/etc/passwd", "~/a", "a b\\c")
}

// strictPatternProblem returns a description if the pattern is missing or
// matches a hostile probe, or "" if it is strict enough.
func (f *Field) strictPatternProblem() string {
	if f.matcher == nil {
		return "a strict typed pattern is required"
	}
	for _, probe := range hostileProbes() {
		if f.matcher.MatchString(probe) {
			return fmt.Sprintf("pattern is too permissive: it matches %q", sanitizeForMessage(probe))
		}
	}
	return ""
}

// Check reports whether v is a valid value for the field: its type, its
// pattern or value list, and for assets a clean package-relative path.
func (f Field) Check(v any) error {
	switch f.Type {
	case TypeList:
		return f.checkList(v)
	case TypeBool:
		if _, ok := v.(bool); !ok {
			return fmt.Errorf("must be a bool, got %T", v)
		}
		return nil
	case TypeDate:
		if _, ok := v.(time.Time); ok {
			return nil
		}
	}
	s, ok := v.(string)
	if !ok {
		return fmt.Errorf("must be a string, got %T", v)
	}
	return f.checkString(s)
}

func (f Field) checkList(v any) error {
	items, ok := v.([]any)
	if !ok {
		return fmt.Errorf("must be a list, got %T", v)
	}
	for i, item := range items {
		s, ok := item.(string)
		if !ok {
			return fmt.Errorf("item %d must be a string, got %T", i, item)
		}
		if err := f.checkString(s); err != nil {
			return fmt.Errorf("item %d: %w", i, err)
		}
	}
	return nil
}

func (f Field) checkString(s string) error {
	if f.Type == TypeEnum && !slices.Contains(f.Values, s) {
		return fmt.Errorf("%q is not one of %s", sanitizeForMessage(s), strings.Join(f.Values, ", "))
	}
	if f.matcher != nil && !f.matcher.MatchString(s) {
		return fmt.Errorf("%q does not match the field pattern %s", sanitizeForMessage(s), f.Pattern)
	}
	if f.Type == TypeAsset && !relativePathRE.MatchString(s) {
		return fmt.Errorf("%q is not a clean package-relative path", sanitizeForMessage(s))
	}
	return nil
}

func joinTypes() string {
	names := make([]string, len(fieldTypes))
	for i, t := range fieldTypes {
		names[i] = string(t)
	}
	return strings.Join(names, ", ")
}
