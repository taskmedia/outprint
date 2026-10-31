package packages

import (
	"fmt"
	"regexp"
	"strings"
)

// useMarker stands in for an interpolated variable in the skeleton, the
// template text with every Pandoc directive removed.
const useMarker = '\x01'

type useKind int

const (
	useValue useKind = iota
	useCondition
	useLoop
)

// varUse is one reference to a template variable. Name is the top-level
// variable (the part before the first dot).
type varUse struct {
	Name        string
	Kind        useKind
	Line        int
	SkeletonPos int
}

type pandocParse struct {
	Skeleton string
	Uses     []varUse
	Issues   []texIssue
}

var (
	variableRE    = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]*(\.[A-Za-z0-9_-]+)*$`)
	conditionRE   = regexp.MustCompile(`^(if|elseif|for)\(\s*([^()]*?)\s*\)$`)
	pipeOrApplyRE = regexp.MustCompile(`[/:]`)
)

// pandocBuiltins are variables Pandoc provides itself or that every
// document may carry; the drift guard does not require a manifest
// declaration for them. This is the one place the list lives.
var pandocBuiltins = map[string]bool{
	"body":                true,
	"highlighting-macros": true,
	"header-includes":     true,
	"include-before":      true,
	"include-after":       true,
	"meta-json":           true,
	"pagetitle":           true,
	"titleblock":          true,
	"lang":                true,
	"babel-lang":          true,
	"babel-otherlangs":    true,
	"title":               true,
	"date":                true,
	"it":                  true,
}

// parsePandoc extracts variable references from a Pandoc template and
// returns the template text without directives. It follows the documented
// syntax: $var$, ${var}, $if(var)$, $elseif(var)$, $for(var)$, $else$,
// $endif$, $endfor$, $sep$, $$ (a literal dollar) and $-- comments.
func parsePandoc(src string) pandocParse {
	var p pandocParse
	var skel strings.Builder
	line := 1
	for i := 0; i < len(src); {
		if src[i] != '$' {
			if src[i] == '\n' {
				line++
			}
			skel.WriteByte(src[i])
			i++
			continue
		}
		next, consumed := directiveBounds(src, i)
		if consumed == 0 {
			p.Issues = append(p.Issues, texIssue{Line: line, Msg: "unterminated $ in template"})
			skel.WriteString(src[i:])
			break
		}
		if next == "$" { // "$$" is a literal dollar
			skel.WriteByte('$')
			i += consumed
			continue
		}
		p.handleDirective(next, line, &skel)
		raw := src[i : i+consumed]
		skel.WriteString(strings.Repeat("\n", strings.Count(raw, "\n")))
		line += strings.Count(raw, "\n")
		i += consumed
	}
	p.Skeleton = skel.String()
	return p
}

// directiveBounds returns the directive text and how many bytes it spans,
// or consumed 0 if it never terminates. The literal "$$" yields "$".
func directiveBounds(src string, i int) (content string, consumed int) {
	rest := src[i+1:]
	switch {
	case strings.HasPrefix(rest, "$"):
		return "$", 2
	case strings.HasPrefix(rest, "--"):
		end := strings.IndexByte(rest, '\n')
		if end < 0 {
			return "--", len(src) - i
		}
		return "--", 1 + end // the newline stays
	case strings.HasPrefix(rest, "{"):
		end := strings.IndexByte(rest, '}')
		if end < 0 {
			return "", 0
		}
		return strings.TrimSpace(rest[1:end]), end + 2
	}
	end := strings.IndexByte(rest, '$')
	if end < 0 {
		return "", 0
	}
	return strings.TrimSpace(rest[:end]), end + 2
}

func (p *pandocParse) handleDirective(content string, line int, skel *strings.Builder) {
	switch content {
	case "--", "else", "endif", "endfor", "sep":
		return
	}
	if m := conditionRE.FindStringSubmatch(content); m != nil {
		kind := useCondition
		if m[1] == "for" {
			kind = useLoop
		}
		p.addUse(m[2], kind, line, -1)
		return
	}
	if strings.Contains(content, "(") {
		p.Issues = append(p.Issues, texIssue{Line: line, Msg: fmt.Sprintf("template syntax %q (partials and applied functions) is not supported", "$"+content+"$")})
		return
	}
	name := pipeOrApplyRE.Split(content, 2)[0]
	if strings.Contains(content, ":") {
		p.Issues = append(p.Issues, texIssue{Line: line, Msg: fmt.Sprintf("template syntax %q is not supported", "$"+content+"$")})
		return
	}
	if !variableRE.MatchString(name) {
		p.Issues = append(p.Issues, texIssue{Line: line, Msg: fmt.Sprintf("unrecognised template syntax %q", "$"+content+"$")})
		return
	}
	if endsInOpenControlSequence(skel.String()) {
		p.Issues = append(p.Issues, texIssue{Line: line, Msg: fmt.Sprintf("variable %q is interpolated into a TeX control sequence name", name)})
		return
	}
	p.addUse(name, useValue, line, skel.Len())
	skel.WriteRune(useMarker)
}

func (p *pandocParse) addUse(name string, kind useKind, line, pos int) {
	root, _, _ := strings.Cut(name, ".")
	p.Uses = append(p.Uses, varUse{Name: root, Kind: kind, Line: line, SkeletonPos: pos})
}

// endsInOpenControlSequence reports whether text ends in "\" or "\name",
// where the next character would extend the control sequence name.
func endsInOpenControlSequence(text string) bool {
	end := len(text)
	i := end
	for i > 0 && isCSLetter(text[i-1]) {
		i--
	}
	slashes := 0
	for j := i; j > 0 && text[j-1] == '\\'; j-- {
		slashes++
	}
	if i < end { // letters present: need an unescaped backslash start
		return slashes%2 == 1
	}
	return slashes%2 == 1
}

func isCSLetter(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b == '@'
}

// nonTextCommands take their argument as a colour, length, file name,
// package or similar: a value interpolated there is not typeset text.
var nonTextCommands = map[string]bool{
	"definecolor": true, "color": true, "textcolor": true, "colorbox": true, "fcolorbox": true,
	"pagecolor": true, "includegraphics": true, "includesvg": true, "includepdf": true,
	"input": true, "include": true, "setlength": true, "addtolength": true, "setcounter": true,
	"hspace": true, "vspace": true, "hskip": true, "vskip": true, "rule": true, "raisebox": true,
	"setmainfont": true, "setsansfont": true, "setmonofont": true, "newfontfamily": true,
	"fontspec": true, "usepackage": true, "RequirePackage": true, "documentclass": true,
	"geometry": true, "graphicspath": true, "hypersetup": true, "fontsize": true,
	"linespread": true, "label": true, "ref": true, "pageref": true, "url": true, "href": true,
}

// nonTextCommand reports whether the marker at pos sits in a non-text
// position: inside an option bracket, or inside an argument of a command in
// nonTextCommands. It returns the command (empty for a bare bracket).
func nonTextCommand(skeleton string, pos int) (string, bool) {
	var closers []byte
	for i := pos - 1; i >= 0; i-- {
		if isEscaped(skeleton, i) {
			continue
		}
		switch skeleton[i] {
		case '}', ']':
			closers = append(closers, skeleton[i])
		case '{', '[':
			if n := len(closers); n > 0 && closers[n-1] == closerFor(skeleton[i]) {
				closers = closers[:n-1]
				continue
			}
			if skeleton[i] == '[' {
				return commandBefore(skeleton, i), true
			}
			cmd := commandBefore(skeleton, i)
			if cmd == "" {
				continue // a bare group: look at the enclosing one
			}
			return cmd, nonTextCommands[cmd]
		}
	}
	return "", false
}

func closerFor(open byte) byte {
	if open == '{' {
		return '}'
	}
	return ']'
}

// isEscaped reports whether the character at i is preceded by an odd
// number of backslashes.
func isEscaped(s string, i int) bool {
	n := 0
	for j := i; j > 0 && s[j-1] == '\\'; j-- {
		n++
	}
	return n%2 == 1
}

// commandBefore returns the name of the command whose argument group opens
// at i, skipping earlier argument groups: for "\cmd[a]{b}{" it is "cmd".
func commandBefore(s string, open int) string {
	i := open
	for i > 0 {
		c := s[i-1]
		switch {
		case c == ' ' || c == '\n' || c == '\t':
			i--
		case c == '}' || c == ']':
			i = matchingOpen(s, i-1)
			if i < 0 {
				return ""
			}
		case isCSLetter(c):
			end := i
			for i > 0 && isCSLetter(s[i-1]) {
				i--
			}
			if i > 0 && s[i-1] == '\\' && !isEscaped(s, i-1) {
				return s[i:end]
			}
			return ""
		default:
			return ""
		}
	}
	return ""
}

func matchingOpen(s string, closeIdx int) int {
	depth := 0
	for i := closeIdx; i >= 0; i-- {
		if isEscaped(s, i) {
			continue
		}
		switch s[i] {
		case '}', ']':
			depth++
		case '{', '[':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}
