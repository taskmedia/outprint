package main

import (
	"errors"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/taskmedia/outprint/internal/metadata"
	"github.com/taskmedia/outprint/internal/packages"
)

// sampleValues are example values for the Standard Fields. A field that is not
// listed gets a value from its type; a field that has neither fails the run, so
// a Template author must extend this table instead of shipping a bad example.
var sampleValues = map[string]any{
	"title":      sampleTitle,
	"date":       sampleDate,
	"language":   "en",
	"author":     "Robin Sample",
	"recipient":  []string{"Ada Example", "1 Sample Lane", "12345 Exampleton"},
	"sender":     []string{"2 Orchard Road", "54321 Exampleton"},
	"subject":    "Spring plant sale",
	"opening":    "Dear Ms Example,",
	"closing":    "With best wishes",
	"reference":  "PS-2026-03",
	"enclosures": []string{"Price list", "Map of the greenhouse"},
	"cc":         []string{"Treasurer", "Sale committee"},
	"subtitle":   "Results and plans",
	"version":    "1.0",
	"abstract":   "A short summary of the report, in *one* or two sentences.",
	"toc":        true,
}

var typeSamples = map[packages.FieldType]any{
	packages.TypeString: "Example text",
	packages.TypeText:   "Example text with *emphasis*.",
	packages.TypeList:   []string{"First item", "Second item"},
	packages.TypeDate:   sampleDate,
	packages.TypeBool:   true,
}

func sampleFor(f packages.Field) (any, error) {
	candidates := []any{}
	if f.Type == packages.TypeEnum && len(f.Values) > 0 {
		candidates = append(candidates, f.Values[0])
	}
	if v, ok := sampleValues[f.Name]; ok {
		candidates = append(candidates, v)
	}
	if v, ok := typeSamples[f.Type]; ok {
		candidates = append(candidates, v)
	}
	for _, v := range candidates {
		if items, ok := v.([]string); ok {
			v = toAny(items)
		}
		if f.Check(v) == nil {
			return v, nil
		}
	}
	return nil, fmt.Errorf("no example value for field %q (type %s): add one to sampleValues in scripts/docs-catalogue/example.go", f.Name, f.Type)
}

// exampleBlock builds the ready-to-copy Metadata Block of a Document Type and
// proves that the real resolver accepts it.
func (c *catalogue) exampleBlock(t packages.Template, typeName string, fields []packages.Field) (string, error) {
	var b strings.Builder
	b.WriteString("type: " + typeName + "\n")
	for _, f := range fields {
		if _, fixed := c.cfg.Fixed[f.Name]; fixed {
			continue
		}
		v, err := sampleFor(f)
		if err != nil {
			return "", err
		}
		line, err := yamlEntry(f.Name, v)
		if err != nil {
			return "", err
		}
		if optionalByDefault(f) {
			line = commentOut(line)
		}
		b.WriteString(line)
	}
	block := b.String()
	if err := c.accepts(t, block); err != nil {
		return "", fmt.Errorf("the example block is not valid: %w", err)
	}
	return block, nil
}

// optionalByDefault marks the fields that Outprint fills in from the Outline
// document itself, so the author can leave them out.
func optionalByDefault(f packages.Field) bool {
	return (f.Name == "title" || f.Name == "date") && !f.Required
}

func commentOut(entry string) string {
	lines := strings.Split(strings.TrimRight(entry, "\n"), "\n")
	for i, l := range lines {
		lines[i] = "# " + l
	}
	return strings.Join(lines, "\n") + "\n"
}

func yamlEntry(name string, v any) (string, error) {
	if s, ok := v.(string); ok && name == "date" && s == sampleDate {
		return name + ": " + s + "\n", nil // an unquoted ISO date is what authors write
	}
	out, err := yaml.Marshal(map[string]any{name: v})
	if err != nil {
		return "", err
	}
	return indentSequences(string(out)), nil
}

// indentSequences turns yaml.v3's two-space list items into the form the
// documentation uses everywhere.
func indentSequences(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "\n    - ", "\n  - "), "\n- ", "\n  - ")
}

func (c *catalogue) accepts(t packages.Template, block string) error {
	_, err := c.resolve(t, "```yaml\n"+block+"```\nBody.\n")
	return err
}

func (c *catalogue) resolve(t packages.Template, markdown string) (metadata.Result, error) {
	return metadata.Resolve(metadata.Input{
		Markdown:      []byte(markdown),
		Title:         "Untitled",
		Modified:      sampleDate,
		Template:      t,
		Configuration: c.cfg,
	})
}

// errorCase is one row of the "Errors you can expect" table.
type errorCase struct {
	code, field, message, cause, fix string
}

// shown is the message as the command line prints it: field, then text.
func (e errorCase) shown() string {
	if e.field == "" {
		return e.message
	}
	return e.field + ": " + e.message
}

// errorCases provokes the typical mistakes against the real resolver and
// records its own messages.
func (c *catalogue) errorCases(t packages.Template, typeName string, fields []packages.Field) ([]errorCase, error) {
	var cases []errorCase
	add := func(markdown, wantCode, cause, fix string) error {
		_, err := c.resolve(t, markdown)
		var list *metadata.ErrorList
		if !errors.As(err, &list) {
			return fmt.Errorf("expected a %q error for %q, got none", wantCode, markdown)
		}
		for _, e := range list.Errors {
			if e.Code == wantCode {
				cases = append(cases, errorCase{code: e.Code, field: e.Field, message: e.Message, cause: cause, fix: fix})
				return nil
			}
		}
		return fmt.Errorf("expected a %q error for %q, got %v", wantCode, markdown, list.Errors)
	}
	block := func(lines ...string) string {
		return "```yaml\ntype: " + typeName + "\n" + strings.Join(lines, "\n") + "\n```\nBody.\n"
	}
	open := func(f packages.Field) bool {
		_, fixed := c.cfg.Fixed[f.Name]
		return !fixed && !optionalByDefault(f)
	}

	if f, ok := first(fields, func(f packages.Field) bool { return open(f) && len(f.Name) >= 6 && f.Pattern == "" }); ok {
		typo := f.Name[:len(f.Name)/2] + f.Name[len(f.Name)/2+1:]
		if err := add(block(typo+": text"), metadata.CodeUnknown,
			"A field name is misspelled, or the field does not exist in this document type.",
			"Use the name the message suggests, or one from the field table."); err != nil {
			return nil, err
		}
	}
	if f, ok := first(fields, func(f packages.Field) bool { return f.Required && open(f) || f.Required && f.Name == "title" }); ok {
		if err := add(block(f.Name+":"), metadata.CodeRequired,
			"A required field is empty or missing.",
			"Give the field a value."); err != nil {
			return nil, err
		}
	}
	if f, ok := first(fields, func(f packages.Field) bool { return open(f) && f.Pattern != "" && f.Type == packages.TypeString }); ok {
		bad, found := failingValue(f)
		if !found {
			return nil, fmt.Errorf("no failing value for pattern of %q", f.Name)
		}
		if err := add(block(f.Name+": "+bad), metadata.CodeType,
			"The value does not match the allowed format of the field.",
			"Use a value that matches the format in the field table."); err != nil {
			return nil, err
		}
	}
	if f, ok := first(fields, func(f packages.Field) bool { return open(f) && f.Type == packages.TypeString && f.Pattern == "" }); ok {
		if err := add(block(f.Name+":\n  - first\n  - second"), metadata.CodeType,
			"A list was written where one line of text is expected.",
			"Write a single line of text."); err != nil {
			return nil, err
		}
	}
	if f, ok := first(fields, func(f packages.Field) bool { return open(f) && f.Type == packages.TypeEnum }); ok {
		if err := add(block(f.Name+": zz"), metadata.CodeEnum,
			"The value is not one of the allowed choices.",
			"Pick one of the values in the field table."); err != nil {
			return nil, err
		}
	}
	if err := add("```yaml\ntype: memo\n```\nBody.\n", metadata.CodeUnknown,
		"The type is not a document type of this Template.",
		"Use one of the document types listed at the top of this page."); err != nil {
		return nil, err
	}
	if f, ok := first(fields, func(f packages.Field) bool { _, fixed := c.cfg.Fixed[f.Name]; return fixed }); ok {
		if err := add(block(f.Name+": Other"), metadata.CodeUnknown,
			"The field is fixed by the configuration of the Collection.",
			"Remove the line. To change the value, ask the Collection admin to change the configuration."); err != nil {
			return nil, err
		}
	}
	if err := add("```yaml\ntype: "+typeName+"\nsubject: Hello\n", metadata.CodeBlock,
		"The block has no closing line of three backticks.",
		"Add a line with three backticks after the last field."); err != nil {
		return nil, err
	}
	return dedupeCases(cases), nil
}

func dedupeCases(in []errorCase) []errorCase {
	seen := map[string]bool{}
	var out []errorCase
	for _, ec := range in {
		if key := ec.code + "|" + ec.message; !seen[key] {
			seen[key] = true
			out = append(out, ec)
		}
	}
	return out
}

func first(fields []packages.Field, ok func(packages.Field) bool) (packages.Field, bool) {
	for _, f := range fields {
		if ok(f) {
			return f, true
		}
	}
	return packages.Field{}, false
}

func failingValue(f packages.Field) (string, bool) {
	for _, v := range []string{"fr", "not valid", "ZZZ", "000", "x"} {
		if f.Check(v) != nil {
			return v, true
		}
	}
	return "", false
}

func toAny(items []string) []any {
	out := make([]any, len(items))
	for i, s := range items {
		out[i] = s
	}
	return out
}
