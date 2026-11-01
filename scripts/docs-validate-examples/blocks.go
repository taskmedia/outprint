package main

import (
	"fmt"
	"regexp"
	"strings"
)

// block is one top-level fenced code block of a Markdown file that holds YAML:
// either a ```yaml block, or a ```markdown block that shows a Metadata Block
// or a Binding block (a yaml fence inside it).
type block struct {
	file      string
	line      int    // line of the opening fence, from 1
	lang      string // yaml or markdown
	content   string // without the fence, indentation removed
	directive string // text after "validate:" of the marker line
	hasMarker bool
}

var (
	markerPattern = regexp.MustCompile(`^\s*<!--\s*validate:\s*(.*?)\s*-->\s*$`)
	openPattern   = regexp.MustCompile("^(\\s*)(`{3,}|~{3,})\\s*([A-Za-z0-9_-]*)")
	innerYAML     = regexp.MustCompile("(?m)^\\s*(`{3,}|~{3,})\\s*ya?ml\\b")
)

// scan returns the YAML blocks of one file and the problems with markers that
// do not stand directly before a fenced block.
func scan(file, text string) ([]block, []string) {
	lines := strings.Split(text, "\n")
	var blocks []block
	var problems []string
	markers := map[int]string{} // line number of the fence -> directive
	markerAt := map[int]int{}   // line number of the fence -> line of its marker
	for i := 0; i < len(lines); i++ {
		if m := markerPattern.FindStringSubmatch(lines[i]); m != nil {
			markers[i+2] = m[1]
			markerAt[i+2] = i + 1
			continue
		}
		open := openPattern.FindStringSubmatch(lines[i])
		if open == nil {
			continue
		}
		indent, fence, lang := open[1], open[2], strings.ToLower(open[3])
		end := closingFence(lines, i+1, fence)
		body := dedent(lines[i+1:end], len(indent))
		delete(markerAt, i+1)
		if isYAML(lang, body) {
			d, has := markers[i+1]
			blocks = append(blocks, block{file: file, line: i + 1, lang: normalise(lang), content: body, directive: d, hasMarker: has})
		}
		i = end
	}
	for _, at := range markerAt {
		problems = append(problems, fmt.Sprintf("%s:%d: validate marker is not directly before a fenced block", file, at))
	}
	return blocks, problems
}

// closingFence returns the index of the line that closes a fence opened with
// the given run of characters, or len(lines) when it never closes.
func closingFence(lines []string, from int, fence string) int {
	ch := fence[:1]
	for j := from; j < len(lines); j++ {
		t := strings.TrimSpace(lines[j])
		if len(t) >= len(fence) && strings.Trim(t, ch) == "" {
			return j
		}
	}
	return len(lines)
}

func dedent(lines []string, n int) string {
	var b strings.Builder
	for _, l := range lines {
		cut := 0
		for cut < n && cut < len(l) && l[cut] == ' ' {
			cut++
		}
		b.WriteString(l[cut:])
		b.WriteByte('\n')
	}
	return b.String()
}

func isYAML(lang, body string) bool {
	switch lang {
	case "yaml", "yml":
		return true
	case "markdown", "md":
		return innerYAML.MatchString(body)
	}
	return false
}

func normalise(lang string) string {
	switch lang {
	case "yml":
		return "yaml"
	case "md":
		return "markdown"
	}
	return lang
}
