package packages

import (
	"reflect"
	"strings"
	"testing"
)

func useNames(p pandocParse, kind useKind) []string {
	var names []string
	for _, u := range p.Uses {
		if u.Kind == kind {
			names = append(names, u.Name)
		}
	}
	return names
}

func TestParsePandocVariables(t *testing.T) {
	src := "\\x{$name$} ${logo} $if(recipient)$$for(recipient)$$recipient$$sep$\\\\ $endfor$$else$\\null$endif$\n" +
		"$-- a comment $hidden$\n" +
		"$body$ $meta.title/uppercase$ costs $$5 $elseif(extra.flag)$$endif$"
	p := parsePandoc(src)
	if len(p.Issues) != 0 {
		t.Fatalf("unexpected issues: %v", p.Issues)
	}
	if got, want := useNames(p, useValue), []string{"name", "logo", "recipient", "body", "meta"}; !reflect.DeepEqual(got, want) {
		t.Errorf("values = %v, want %v", got, want)
	}
	if got, want := useNames(p, useCondition), []string{"recipient", "extra"}; !reflect.DeepEqual(got, want) {
		t.Errorf("conditions = %v, want %v", got, want)
	}
	if got, want := useNames(p, useLoop), []string{"recipient"}; !reflect.DeepEqual(got, want) {
		t.Errorf("loops = %v, want %v", got, want)
	}
	if strings.Contains(p.Skeleton, "hidden") {
		t.Errorf("comment leaked into skeleton: %q", p.Skeleton)
	}
}

func TestParsePandocLines(t *testing.T) {
	p := parsePandoc("a\nb\n$if(x)$\n$y$\n$endif$\n")
	for _, u := range p.Uses {
		if u.Name == "y" && u.Line != 4 {
			t.Errorf("y on line %d, want 4", u.Line)
		}
		if u.Name == "x" && u.Line != 3 {
			t.Errorf("x on line %d, want 3", u.Line)
		}
	}
}

func TestParsePandocIssues(t *testing.T) {
	cases := map[string]string{
		"partial":         "$header()$",
		"applied":         "$items:entry()$",
		"unterminated":    "text $name",
		"unknown syntax":  "$a b c$",
		"in cs name":      "\\foo$name$",
		"after backslash": "\\$name$",
		"braced cs name":  "\\input${name}",
	}
	for name, src := range cases {
		if p := parsePandoc(src); len(p.Issues) == 0 {
			t.Errorf("%s: expected an issue for %q", name, src)
		}
	}
}

func TestParsePandocAllowsControlSymbolBeforeInterpolation(t *testing.T) {
	if p := parsePandoc("line\\\\$name$ and \\sffamily $name$"); len(p.Issues) != 0 {
		t.Fatalf("unexpected issues: %v", p.Issues)
	}
}

func TestNonTextPositions(t *testing.T) {
	cases := []struct {
		src     string
		nonText bool
		cmd     string
	}{
		{`\definecolor{accent}{HTML}{$c$}`, true, "definecolor"},
		{`\setkomavar{fromlogo}{\includegraphics[height=18mm]{$c$}}`, true, "includegraphics"},
		{`\includegraphics[width=$c$]{x.pdf}`, true, "includegraphics"},
		{`\setlength{\parindent}{$c$}`, true, "setlength"},
		{`\textcolor{$c$}{words}`, true, "textcolor"},
		{`\setkomavar{subject}{$c$}`, false, ""},
		{`\parbox{\textwidth}{\color{accent}\rule{1pt}{1pt}\\ \sffamily $c$}`, false, ""},
		{`\begin{letter}{$c$}`, false, ""},
		{`plain $c$ text`, false, ""},
		{`{ {$c$} }`, false, ""},
		{`\usepackage[$c$]{xcolor}`, true, "usepackage"},
	}
	for _, tc := range cases {
		p := parsePandoc(tc.src)
		var marker int = -1
		for _, u := range p.Uses {
			if u.Name == "c" && u.Kind == useValue {
				marker = u.SkeletonPos
			}
		}
		if marker < 0 {
			t.Fatalf("%q: use not found", tc.src)
		}
		cmd, ok := nonTextCommand(p.Skeleton, marker)
		if ok != tc.nonText || (ok && cmd != tc.cmd) {
			t.Errorf("%q: got (%q, %v), want (%q, %v)", tc.src, cmd, ok, tc.cmd, tc.nonText)
		}
	}
}
