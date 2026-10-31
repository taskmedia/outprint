package binding

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/taskmedia/outline-latex-pdf/internal/config"
	"github.com/taskmedia/outline-latex-pdf/internal/metadata"
)

const (
	keyTemplate      = "template"
	keyConfiguration = "configuration"
	yamlStringTag    = "!!str"
)

var yamlLineRE = regexp.MustCompile(`^yaml: (?:line (\d+): )?(.*)$`)

// parseBlock reads the block strictly: a mapping with exactly the string
// keys template and configuration, both with a non-empty string value.
func parseBlock(blk codeBlock) (config.Binding, []metadata.Error) {
	if !blk.closed {
		return config.Binding{}, []metadata.Error{newError("", CodeMalformed, blk.openLine,
			"the binding block is not closed (missing closing fence)")}
	}
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(strings.Join(blk.body, "\n")), &doc); err != nil {
		return config.Binding{}, []metadata.Error{yamlError(err, blk.openLine)}
	}
	values, errs := readMapping(&doc, blk)
	if len(errs) > 0 {
		return config.Binding{}, errs
	}
	return config.Binding{Template: values[keyTemplate], Configuration: values[keyConfiguration]}, nil
}

func readMapping(doc *yaml.Node, blk codeBlock) (map[string]string, []metadata.Error) {
	var root *yaml.Node
	if len(doc.Content) > 0 {
		root = doc.Content[0]
	}
	if root != nil && root.Kind != yaml.MappingNode {
		return nil, []metadata.Error{newError("", CodeMalformed, blk.openLine+root.Line,
			"the binding block must be a mapping with the keys template and configuration")}
	}
	values := map[string]string{}
	var errs []metadata.Error
	if root != nil {
		for i := 0; i+1 < len(root.Content); i += 2 {
			errs = append(errs, readEntry(root.Content[i], root.Content[i+1], blk.openLine, values)...)
		}
	}
	for _, key := range []string{keyTemplate, keyConfiguration} {
		if _, ok := values[key]; !ok && !hasError(errs, key) {
			errs = append(errs, newError(key, CodeMalformed, blk.openLine, fmt.Sprintf("missing key %q", key)))
		}
	}
	return values, errs
}

func readEntry(k, v *yaml.Node, offset int, values map[string]string) []metadata.Error {
	line := offset + k.Line
	if k.Kind != yaml.ScalarNode {
		return []metadata.Error{newError("", CodeMalformed, line, "keys must be plain names")}
	}
	if k.Value != keyTemplate && k.Value != keyConfiguration {
		return []metadata.Error{newError(k.Value, CodeMalformed, line,
			fmt.Sprintf("unknown key %q: the binding block has exactly the keys template and configuration", k.Value))}
	}
	if _, dup := values[k.Value]; dup {
		return []metadata.Error{newError(k.Value, CodeMalformed, line, fmt.Sprintf("key %q appears twice", k.Value))}
	}
	if v.Kind != yaml.ScalarNode || v.Tag != yamlStringTag || strings.TrimSpace(v.Value) == "" {
		return []metadata.Error{newError(k.Value, CodeMalformed, line,
			fmt.Sprintf("%q must be the name of an installed package, as a string", k.Value))}
	}
	values[k.Value] = v.Value
	return nil
}

func hasError(errs []metadata.Error, field string) bool {
	for _, e := range errs {
		if e.Field == field {
			return true
		}
	}
	return false
}

// keyLine is the overview line of a key in a block that parsed, or the
// fence line if it cannot be found.
func keyLine(blk codeBlock, key string) int {
	for i, l := range blk.body {
		if strings.HasPrefix(strings.TrimSpace(l), key+":") {
			return blk.openLine + i + 1
		}
	}
	return blk.openLine
}

func yamlError(err error, openLine int) metadata.Error {
	msg, line := err.Error(), openLine+1
	if m := yamlLineRE.FindStringSubmatch(msg); m != nil {
		if m[1] != "" {
			n, _ := strconv.Atoi(m[1])
			line = openLine + n
		}
		msg = m[2]
	}
	return newError("", CodeMalformed, line, "invalid YAML in the binding block: "+strings.TrimSpace(msg))
}
