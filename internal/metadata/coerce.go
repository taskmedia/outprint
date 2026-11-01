package metadata

import (
	"fmt"
	"strings"
	"time"
	"unicode"

	"gopkg.in/yaml.v3"

	"github.com/taskmedia/outprint/internal/packages"
)

const isoDateLayout = "2006-01-02"

// problem is why a value was rejected, before it is given a source and a
// position.
type problem struct {
	code string
	msg  string
	line int // Document line, 0 if unknown
}

func typeProblem(line int, format string, args ...any) *problem {
	return &problem{code: CodeType, msg: fmt.Sprintf(format, args...), line: line}
}

// coerced is a value brought into the form packages.Field.Check expects:
// a string, a []any of strings, or a bool. empty means the author gave an
// empty value, which suppresses lower layers.
type coerced struct {
	value any
	empty bool
}

// fromNode coerces a YAML value strictly: only scalars become text, a list
// is a YAML list or a multi-line string, a bool must be a YAML bool.
func fromNode(f packages.Field, e entry) (coerced, *problem) {
	n := e.node
	if n.Kind == yaml.AliasNode {
		return coerced{}, typeProblem(e.valueLine, "aliases are not supported in the metadata block")
	}
	if n.Kind == yaml.SequenceNode && len(n.Content) == 0 {
		return coerced{empty: true}, nil
	}
	switch f.Type {
	case packages.TypeList:
		return listFromNode(n, e)
	case packages.TypeBool:
		return boolFromNode(n, e.valueLine)
	}
	return textFromNode(f, n, e.valueLine)
}

func isNull(n *yaml.Node) bool {
	return n.Kind == yaml.ScalarNode && n.ShortTag() == "!!null"
}

func textFromNode(f packages.Field, n *yaml.Node, line int) (coerced, *problem) {
	if isNull(n) {
		return coerced{empty: true}, nil
	}
	if n.Kind != yaml.ScalarNode {
		return coerced{}, typeProblem(line, "must be text, not %s", kindName(n))
	}
	s := strings.TrimRight(n.Value, "\n")
	if strings.TrimSpace(s) == "" {
		return coerced{empty: true}, nil
	}
	if p := checkChars(s, line); p != nil {
		return coerced{}, p
	}
	if f.Type != packages.TypeText && strings.Contains(s, "\n") {
		return coerced{}, typeProblem(line, "must be a single line")
	}
	return coerced{value: s}, nil
}

func boolFromNode(n *yaml.Node, line int) (coerced, *problem) {
	if isNull(n) {
		return coerced{empty: true}, nil
	}
	var b bool
	if n.Kind != yaml.ScalarNode || n.ShortTag() != "!!bool" || n.Decode(&b) != nil {
		return coerced{}, typeProblem(line, "must be true or false")
	}
	return coerced{value: b}, nil
}

func listFromNode(n *yaml.Node, e entry) (coerced, *problem) {
	line := e.valueLine
	switch {
	case isNull(n):
		return coerced{empty: true}, nil
	case n.Kind == yaml.ScalarNode:
		return linesToList(n.Value, line)
	case n.Kind != yaml.SequenceNode:
		return coerced{}, typeProblem(line, "must be a list or a multi-line text, not %s", kindName(n))
	}
	var items []any
	for _, item := range n.Content {
		itemLine := e.base + item.Line
		if item.Kind != yaml.ScalarNode {
			return coerced{}, typeProblem(itemLine, "list items must be text, not %s", kindName(item))
		}
		if isNull(item) || strings.TrimSpace(item.Value) == "" {
			continue
		}
		if p := checkChars(item.Value, itemLine); p != nil {
			return coerced{}, p
		}
		if strings.Contains(strings.TrimRight(item.Value, "\n"), "\n") {
			return coerced{}, typeProblem(itemLine, "list items must be a single line")
		}
		items = append(items, strings.TrimRight(item.Value, "\n"))
	}
	return listResult(items), nil
}

func linesToList(s string, line int) (coerced, *problem) {
	if p := checkChars(s, line); p != nil {
		return coerced{}, p
	}
	var items []any
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimRight(l, " \t"); strings.TrimSpace(l) != "" {
			items = append(items, l)
		}
	}
	return listResult(items), nil
}

func listResult(items []any) coerced {
	if len(items) == 0 {
		return coerced{empty: true}
	}
	return coerced{value: items}
}

func kindName(n *yaml.Node) string {
	switch n.Kind {
	case yaml.MappingNode:
		return "a mapping"
	case yaml.SequenceNode:
		return "a list"
	case yaml.AliasNode:
		return "an alias"
	}
	return "a scalar"
}

// checkChars rejects control characters other than tab and newline.
func checkChars(s string, line int) *problem {
	for _, r := range s {
		if unicode.IsControl(r) && r != '\t' && r != '\n' {
			return typeProblem(line, "contains a control character (U+%04X)", r)
		}
	}
	return nil
}

// fromNative coerces a value that came from YAML decoded by the package
// loader (manifest default, Configuration value). It is strict: nothing but
// the obvious conversions happen, Check rejects the rest.
func fromNative(f packages.Field, raw any) (coerced, *problem) {
	switch v := raw.(type) {
	case time.Time:
		if f.Type == packages.TypeDate {
			return coerced{value: v.Format(isoDateLayout)}, nil
		}
	case string:
		if f.Type == packages.TypeList {
			return linesToList(v, 0)
		}
		if strings.TrimSpace(v) == "" {
			return coerced{empty: true}, nil
		}
		if p := checkChars(v, 0); p != nil {
			return coerced{}, p
		}
	case nil:
		return coerced{empty: true}, nil
	}
	return coerced{value: raw}, nil
}

// finalize checks a coerced value against the field and converts it to the
// output form: string, []string or bool.
func finalize(f packages.Field, c coerced) (any, *problem) {
	if err := f.Check(c.value); err != nil {
		code := CodeType
		if f.Type == packages.TypeEnum {
			code = CodeEnum
		}
		return nil, &problem{code: code, msg: err.Error()}
	}
	items, isList := c.value.([]any)
	if !isList {
		return c.value, nil
	}
	out := make([]string, len(items))
	for i, item := range items {
		out[i] = item.(string)
	}
	return out, nil
}
