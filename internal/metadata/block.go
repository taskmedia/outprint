package metadata

import (
	"regexp"
	"strings"
)

const metadataInfo = "yaml metadata"

var (
	fenceOpenRE = regexp.MustCompile("^ {0,3}(`{3,}|~{3,})[ \t]*(.*?)[ \t]*$")
)

// blockSplit is the Document cut into the Metadata Block and the body.
type blockSplit struct {
	body string
	// yaml is the content of the block; yamlOffset is the Document line
	// that precedes its first line, so YAML line n is Document line
	// yamlOffset+n.
	yaml       string
	yamlOffset int
	hasBlock   bool
	errs       []Error
}

type fence struct {
	char byte
	size int
}

func (f fence) closedBy(line string) bool {
	trimmed := strings.TrimSpace(line)
	return len(trimmed) >= f.size && strings.Trim(trimmed, string(f.char)) == ""
}

// splitBlock finds the Metadata Block. Only the first fenced `yaml
// metadata` block at the very start of the Document (blank lines before it
// are tolerated) is the block; any other one is an error. Fenced code in
// the body is skipped, so a block shown as an example inside another fence
// is plain text.
func splitBlock(markdown string) blockSplit {
	text := strings.ReplaceAll(markdown, "\r\n", "\n")
	lines := strings.Split(text, "\n")
	start := firstNonBlank(lines)

	var out blockSplit
	out.body = text
	seen := 0
	for i := 0; i < len(lines); i++ {
		open, info, ok := fenceOpen(lines[i])
		if !ok {
			continue
		}
		closeAt := findClose(lines, i+1, open)
		if strings.Join(strings.Fields(info), " ") != metadataInfo {
			i = closeAt
			continue
		}
		seen++
		switch {
		case closeAt >= len(lines):
			out.errs = append(out.errs, blockError(i+1, "the metadata block is not closed (missing closing fence)"))
		case seen > 1:
			out.errs = append(out.errs, blockError(i+1, "second metadata block: a Document has at most one"))
		case i != start:
			out.errs = append(out.errs, blockError(i+1, "misplaced metadata block: it must be at the start of the document"))
		default:
			out.hasBlock = true
			out.yaml = strings.Join(lines[i+1:closeAt], "\n")
			out.yamlOffset = i + 1
			out.body = trimLeadingBlankLines(strings.Join(lines[closeAt+1:], "\n"))
		}
		i = closeAt
	}
	return out
}

func blockError(line int, msg string) Error {
	return Error{Source: SourceDocument, Code: CodeBlock, Message: msg, Line: lineRef(line)}
}

func fenceOpen(line string) (fence, string, bool) {
	m := fenceOpenRE.FindStringSubmatch(line)
	if m == nil {
		return fence{}, "", false
	}
	marker, info := m[1], m[2]
	if marker[0] == '`' && strings.Contains(info, "`") {
		return fence{}, "", false // inline code, not a fence
	}
	return fence{char: marker[0], size: len(marker)}, info, true
}

// findClose returns the index of the line closing the fence, or len(lines)
// if it is never closed.
func findClose(lines []string, from int, open fence) int {
	for i := from; i < len(lines); i++ {
		if open.closedBy(lines[i]) {
			return i
		}
	}
	return len(lines)
}

func firstNonBlank(lines []string) int {
	for i, l := range lines {
		if strings.TrimSpace(l) != "" {
			return i
		}
	}
	return len(lines)
}

func trimLeadingBlankLines(s string) string {
	lines := strings.Split(s, "\n")
	return strings.Join(lines[firstNonBlank(lines):], "\n")
}
