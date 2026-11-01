package binding

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/taskmedia/outprint/internal/config"
	"github.com/taskmedia/outprint/internal/metadata"
)

const (
	keyTemplate      = "template"
	keyConfiguration = "configuration"
	yamlStringTag    = "!!str"
)

var yamlLineRE = regexp.MustCompile(`^yaml: (?:line (\d+): )?(.*)$`)

// parseBlock reads the block strictly: the key binding holding a mapping with
// exactly the string keys template and configuration, both with a non-empty
// string value.
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

// readMapping checks the block is exactly `binding:` with a mapping below it,
// then reads that mapping's entries.
func readMapping(doc *yaml.Node, blk codeBlock) (map[string]string, []metadata.Error) {
	var root *yaml.Node
	if len(doc.Content) > 0 {
		root = doc.Content[0]
	}
	if root == nil || root.Kind != yaml.MappingNode {
		return nil, []metadata.Error{newError(bindingKey, CodeMalformed, blk.openLine+1,
			"the block must be a mapping with the key binding")}
	}
	var inner *yaml.Node
	bindingLine := blk.openLine + 1
	var errs []metadata.Error
	for i := 0; i+1 < len(root.Content); i += 2 {
		k, v := root.Content[i], root.Content[i+1]
		line := blk.openLine + k.Line
		switch {
		case k.Kind != yaml.ScalarNode || k.Value != bindingKey:
			errs = append(errs, newError(k.Value, CodeMalformed, line,
				fmt.Sprintf("unknown key %q: the block has exactly the key %s", k.Value, bindingKey)))
		case inner != nil:
			errs = append(errs, newError(bindingKey, CodeMalformed, line, "key \"binding\" appears twice"))
		default:
			inner, bindingLine = v, line
		}
	}
	if inner == nil || inner.Kind != yaml.MappingNode {
		if inner != nil || len(errs) == 0 {
			errs = append(errs, newError(bindingKey, CodeMalformed, bindingLine,
				"binding must be a mapping with the keys template and configuration"))
		}
		return nil, errs
	}
	values := map[string]string{}
	for i := 0; i+1 < len(inner.Content); i += 2 {
		errs = append(errs, readEntry(inner.Content[i], inner.Content[i+1], blk.openLine, values)...)
	}
	for _, key := range []string{keyTemplate, keyConfiguration} {
		if _, ok := values[key]; !ok && !hasError(errs, key) {
			errs = append(errs, newError(key, CodeMalformed, bindingLine, fmt.Sprintf("missing key %q", key)))
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
			fmt.Sprintf("unknown key %q: binding has exactly the keys template and configuration", k.Value))}
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
