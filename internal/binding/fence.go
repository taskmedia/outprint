package binding

import (
	"regexp"
	"strings"
)

const bindingInfo = "yaml binding"

var fenceOpenRE = regexp.MustCompile("^ {0,3}(`{3,}|~{3,})[ \t]*(.*?)[ \t]*$")

// codeBlock is a fenced code block of the overview. Lines are 1-based; a
// block that is never closed runs to the end of the text and has closed
// set to false.
type codeBlock struct {
	info     string
	openLine int
	body     []string
	closed   bool
}

// fencedBlocks returns the top-level fenced code blocks in order. A fence
// opens with three or more backticks or tildes indented at most three
// spaces, and closes with at least as many of the same character. What is
// inside a block, including further fences, is plain text.
func fencedBlocks(text string) []codeBlock {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	var blocks []codeBlock
	for i := 0; i < len(lines); i++ {
		char, size, info, ok := openingFence(lines[i])
		if !ok {
			continue
		}
		end := closingLine(lines, i+1, char, size)
		blocks = append(blocks, codeBlock{
			info: normalizeInfo(info), openLine: i + 1, body: lines[i+1 : end], closed: end < len(lines),
		})
		i = end
	}
	return blocks
}

func openingFence(line string) (char byte, size int, info string, ok bool) {
	m := fenceOpenRE.FindStringSubmatch(line)
	if m == nil {
		return 0, 0, "", false
	}
	marker, info := m[1], m[2]
	if marker[0] == '`' && strings.Contains(info, "`") {
		return 0, 0, "", false // inline code, not a fence
	}
	return marker[0], len(marker), info, true
}

// closingLine returns the index of the line that closes the fence, or
// len(lines) if there is none.
func closingLine(lines []string, from int, char byte, size int) int {
	for i := from; i < len(lines); i++ {
		if isClosingFence(lines[i], char, size) {
			return i
		}
	}
	return len(lines)
}

func isClosingFence(line string, char byte, size int) bool {
	if len(line)-len(strings.TrimLeft(line, " ")) > 3 {
		return false
	}
	trimmed := strings.TrimSpace(line)
	return len(trimmed) >= size && strings.Trim(trimmed, string(char)) == ""
}

func normalizeInfo(info string) string { return strings.Join(strings.Fields(info), " ") }
