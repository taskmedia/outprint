package packages

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// texIssue is a problem found in a template file at a source line.
type texIssue struct {
	Line int
	Msg  string
}

// deniedPrimitives are control sequences a Template may not use: Lua and
// shell execution, file writing and reading, and the tricks that build or
// rename control sequences to get around a name check.
//
// \input is handled separately: it is allowed only for files in the
// Template's own shared directory (see checkInput).
var deniedPrimitives = map[string]string{
	"directlua":         "Lua execution",
	"latelua":           "Lua execution",
	"luaexec":           "Lua execution",
	"luacode":           "Lua execution",
	"ShellEscape":       "shell execution",
	"write":             "file writing and shell escape (\\write18)",
	"openout":           "file writing",
	"closeout":          "file writing",
	"immediate":         "immediate file operations",
	"newwrite":          "file writing",
	"openin":            "file reading",
	"read":              "file reading",
	"readline":          "file reading",
	"newread":           "file reading",
	"include":           "file inclusion outside the package",
	"includeonly":       "file inclusion outside the package",
	"InputIfFileExists": "file inclusion outside the package",
	"@@input":           "file inclusion outside the package",
	"@input":            "file inclusion outside the package",
	"@iinput":           "file inclusion outside the package",
	"verbatiminput":     "file reading",
	"VerbatimInput":     "file reading",
	"lstinputlisting":   "file reading",
	"CatchFileDef":      "file reading",
	"CatchFileEdef":     "file reading",
	"pdffiledump":       "file reading",
	"pdfmdfivesum":      "file reading",
	"pdffilesize":       "file reading",
	"catcode":           "category code changes, used to hide other primitives",
	"csname":            "building control sequence names, used to bypass the deny-list",
	"scantokens":        "re-reading text as TeX code",
}

// deniedPackages may not be loaded by a Template.
var deniedPackages = map[string]bool{
	"shellesc": true, "luacode": true, "catchfile": true, "minted": true,
	"pythontex": true, "sagetex": true, "gnuplottex": true, "markdown": true,
}

// deniedEnvironments are environments that run code or read files.
var deniedEnvironments = map[string]bool{"luacode": true, "luacode*": true, "minted": true}

var (
	// the literal path of a permitted \input
	sharedInputRE = regexp.MustCompile(`^shared/([A-Za-z0-9][A-Za-z0-9._-]*)$`)
	// the literal path of a file read through \includegraphics
	plainPathRE = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9._/-]*$`)
)

// scanTeX applies the deny-list to TeX source. It reports every problem.
// sharedHas says whether a file exists in the Template's shared directory.
//
// Rules, applied after comments are removed:
//   - "^^" character notation is rejected: it can spell any primitive.
//   - control sequences in deniedPrimitives are rejected.
//   - \input must be written \input{shared/NAME} with NAME an existing
//     file in the shared directory; anything else is rejected.
//   - \includegraphics literal paths must be relative and free of "..".
//   - denied packages may not be loaded with \usepackage/\RequirePackage.
func scanTeX(src string, sharedHas func(name string) bool) []texIssue {
	text := stripComments(src)
	var issues []texIssue
	add := func(pos int, format string, args ...any) {
		issues = append(issues, texIssue{Line: 1 + strings.Count(text[:pos], "\n"), Msg: fmt.Sprintf(format, args...)})
	}

	for i := strings.Index(text, "^^"); i >= 0; {
		add(i, "character notation \"^^\" is not allowed (it can spell denied primitives)")
		next := strings.Index(text[i+2:], "^^")
		if next < 0 {
			break
		}
		i += 2 + next
	}

	for i := 0; i < len(text); i++ {
		if text[i] != '\\' {
			continue
		}
		name, end := controlSequence(text, i)
		switch name {
		case "input":
			if msg := checkInput(text[end:], sharedHas); msg != "" {
				add(i, "%s", msg)
			}
		case "includegraphics":
			if msg := checkGraphicsPath(text[end:]); msg != "" {
				add(i, "%s", msg)
			}
		case "begin":
			if env, ok := braced(text[end:]); ok && deniedEnvironments[strings.TrimSpace(env)] {
				add(i, "environment %q is not allowed", env)
			}
		case "usepackage", "RequirePackage":
			for _, pkg := range packageNames(text[end:]) {
				if deniedPackages[pkg] {
					add(i, "package %q is not allowed", pkg)
				}
			}
		default:
			if reason, denied := deniedPrimitives[name]; denied {
				add(i, "\\%s is not allowed (%s)", name, reason)
			}
		}
		i = end - 1
	}
	return issues
}

// controlSequence reads the control sequence starting at the backslash at
// i: letters (and @), or a single other character (a control symbol).
func controlSequence(text string, i int) (name string, end int) {
	j := i + 1
	for j < len(text) && isCSLetter(text[j]) {
		j++
	}
	if j == i+1 && j < len(text) {
		return text[j : j+1], j + 1
	}
	return text[i+1 : j], j
}

// stripComments removes TeX comments but keeps newlines so line numbers hold.
func stripComments(src string) string {
	var b strings.Builder
	for i := 0; i < len(src); i++ {
		switch src[i] {
		case '\\':
			b.WriteByte(src[i])
			if i+1 < len(src) {
				i++
				b.WriteByte(src[i])
			}
		case '%':
			for i < len(src) && src[i] != '\n' {
				i++
			}
			if i < len(src) {
				b.WriteByte('\n')
			}
		default:
			b.WriteByte(src[i])
		}
	}
	return b.String()
}

// braced returns the content of a leading "{...}" group (after optional
// spaces) and whether there was one. Groups do not nest here: a nested
// brace makes the argument non-literal and it is rejected by the callers.
func braced(rest string) (string, bool) {
	rest = strings.TrimLeft(rest, " \t")
	if !strings.HasPrefix(rest, "{") {
		return "", false
	}
	end := strings.IndexByte(rest, '}')
	if end < 0 {
		return "", false
	}
	return rest[1:end], true
}

func checkInput(rest string, sharedHas func(string) bool) string {
	arg, ok := braced(rest)
	if !ok {
		return "\\input must be written \\input{shared/NAME}"
	}
	m := sharedInputRE.FindStringSubmatch(arg)
	if m == nil || strings.Contains(arg, "..") {
		return fmt.Sprintf("\\input{%s} is not allowed: only \\input{shared/NAME} of a file in the package's shared directory", sanitizeForMessage(arg))
	}
	if !sharedHas(m[1]) {
		return fmt.Sprintf("\\input{%s}: no such file in the package's shared directory", arg)
	}
	return ""
}

func checkGraphicsPath(rest string) string {
	rest = strings.TrimLeft(rest, " \t")
	if strings.HasPrefix(rest, "[") {
		if end := strings.IndexByte(rest, ']'); end >= 0 {
			rest = rest[end+1:]
		}
	}
	arg, ok := braced(rest)
	if !ok || strings.ContainsRune(arg, useMarker) {
		return "" // dynamic paths are covered by the typed pattern of the field
	}
	if !plainPathRE.MatchString(arg) || strings.Contains(arg, "..") {
		return fmt.Sprintf("\\includegraphics{%s}: path must be relative to the package and free of \"..\"", sanitizeForMessage(arg))
	}
	return ""
}

func packageNames(rest string) []string {
	rest = strings.TrimLeft(rest, " \t")
	if strings.HasPrefix(rest, "[") {
		if end := strings.IndexByte(rest, ']'); end >= 0 {
			rest = rest[end+1:]
		}
	}
	arg, ok := braced(rest)
	if !ok {
		return nil
	}
	names := strings.Split(arg, ",")
	for i := range names {
		names[i] = strings.TrimSpace(names[i])
	}
	sort.Strings(names)
	return names
}

func sanitizeForMessage(s string) string {
	return strings.Map(func(r rune) rune {
		if r == useMarker || r < ' ' {
			return '?'
		}
		return r
	}, s)
}
