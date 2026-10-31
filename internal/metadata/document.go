package metadata

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

var yamlLineRE = regexp.MustCompile(`^yaml: (?:line (\d+): )?(.*)$`)

// entry is one key of the Metadata Block with its position in the Document.
type entry struct {
	key  string
	line int
	node *yaml.Node
	// valueLine is the Document line of the value; base is the offset to
	// add to the line of any node inside it.
	valueLine int
	base      int
}

// parseEntries reads the block's YAML strictly: it must be a mapping with
// plain keys, each appearing once. offset is added to YAML line numbers to
// get Document lines.
func parseEntries(src string, offset int) ([]entry, []Error) {
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(src), &doc); err != nil {
		return nil, []Error{yamlError(err, offset)}
	}
	if len(doc.Content) == 0 {
		return nil, nil
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, []Error{blockError(offset+root.Line, "the metadata block must be a mapping of field names to values")}
	}
	var entries []entry
	var errs []Error
	seen := map[string]bool{}
	for i := 0; i+1 < len(root.Content); i += 2 {
		k, v := root.Content[i], root.Content[i+1]
		line := offset + k.Line
		switch {
		case k.Kind != yaml.ScalarNode:
			errs = append(errs, blockError(line, "keys must be plain field names"))
		case seen[k.Value]:
			errs = append(errs, Error{Field: k.Value, Source: SourceDocument, Code: CodeBlock, Line: lineRef(line),
				Message: fmt.Sprintf("key %q appears twice", k.Value)})
		default:
			seen[k.Value] = true
			entries = append(entries, entry{key: k.Value, line: line, node: v, valueLine: offset + v.Line, base: offset})
		}
	}
	return entries, errs
}

func yamlError(err error, offset int) Error {
	msg, line := err.Error(), offset+1
	if m := yamlLineRE.FindStringSubmatch(msg); m != nil {
		if m[1] != "" {
			n, _ := strconv.Atoi(m[1])
			line = offset + n
		}
		msg = m[2]
	}
	return blockError(line, "invalid YAML in the metadata block: "+strings.TrimSpace(msg))
}

func indexEntries(entries []entry) map[string]entry {
	m := make(map[string]entry, len(entries))
	for _, e := range entries {
		m[e.key] = e
	}
	return m
}
