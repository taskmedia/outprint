package metadata

import (
	"regexp"
	"strings"
)

// metadataInfo is the info string of the Metadata Block. Outline keeps only the
// first word of a fence's info string, so the block is marked by its place, not
// by a second word.
const metadataInfo = "yaml"

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

// splitBlock finds the Metadata Block: the first thing in the Document (blank
// lines before it are tolerated), if it is a fenced `yaml` block. Any other
// fence, including a later `yaml` one, is body text.
func splitBlock(markdown string) blockSplit {
	text := strings.ReplaceAll(markdown, "\r\n", "\n")
	lines := strings.Split(text, "\n")
	start := firstNonBlank(lines)

	out := blockSplit{body: text}
	if start >= len(lines) {
		return out
	}
	open, info, ok := fenceOpen(lines[start])
	if !ok || strings.Join(strings.Fields(info), " ") != metadataInfo {
		return out
	}
	closeAt := findClose(lines, start+1, open)
	if closeAt >= len(lines) {
		out.errs = append(out.errs, blockError(start+1, "the metadata block is not closed (missing closing fence)"))
		return out
	}
	out.hasBlock = true
	out.yaml = strings.Join(lines[start+1:closeAt], "\n")
	out.yamlOffset = start + 1
	out.body = trimLeadingBlankLines(strings.Join(lines[closeAt+1:], "\n"))
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

// Body returns the Document without its Metadata Block, as Resolve does, and
// the number of lines that were cut off before it. Callers that work on the
// body even when the block has problems use it to report Document lines.
func Body(markdown []byte) ([]byte, int) {
	split := splitBlock(string(markdown))
	text := strings.ReplaceAll(string(markdown), "\r\n", "\n")
	if !strings.HasSuffix(text, split.body) {
		return []byte(split.body), 0
	}
	return []byte(split.body), strings.Count(text[:len(text)-len(split.body)], "\n")
}
