package packages

import (
	"strings"
	"testing"
)

// Security layer 5: hostile Template snippets must be rejected by the TeX
// deny-list. There is no skip path in this suite.
var hostileSnippets = map[string]string{
	"directlua":                     `\directlua{os.execute("id")}`,
	"latelua":                       `\latelua{os.execute("id")}`,
	"luaexec":                       `\luaexec{os.execute("id")}`,
	"luacode environment":           "\\begin{luacode}\nos.execute('id')\n\\end{luacode}",
	"write18":                       `\immediate\write18{id}`,
	"write18 without immediate":     `\write18{id}`,
	"write to file":                 `\newwrite\f \write\f{x}`,
	"openout":                       `\openout3=owned.txt`,
	"closeout":                      `\closeout3`,
	"openin":                        `\openin3=/etc/passwd`,
	"read":                          `\read3 to \line`,
	"readline":                      `\readline3 to \line`,
	"input absolute":                `\input{/etc/passwd}`,
	"input absolute no braces":      `\input /etc/passwd`,
	"input parent":                  `\input{../other/secret.tex}`,
	"input shared escape":           `\input{shared/../../secret.tex}`,
	"input missing shared":          `\input{shared/missing.tex}`,
	"input pipe":                    `\input{|"id"}`,
	"input subdir":                  `\input{shared/sub/x.tex}`,
	"input unrelated":               `\input{other.tex}`,
	"input with macro":              `\input{shared/\jobname}`,
	"input via marker":              "\\input{shared/\x01}",
	"include":                       `\include{other}`,
	"InputIfFileExists":             `\InputIfFileExists{/etc/passwd}{}{}`,
	"internal input":                `\@@input /etc/passwd`,
	"internal input 2":              `\@input{/etc/passwd}`,
	"catcode":                       `\catcode` + "`\\@=11",
	"csname":                        `\csname write\endcsname`,
	"scantokens":                    `\scantokens{\directlua{}}`,
	"caret notation":                `\^^64irectlua{os.execute("id")}`,
	"caret notation write":          `\^^77rite18{id}`,
	"shellesc package":              `\usepackage{shellesc}`,
	"package in list":               `\usepackage[draft]{xcolor,catchfile}`,
	"require package":               `\RequirePackage{luacode}`,
	"ShellEscape":                   `\ShellEscape{id}`,
	"verbatiminput":                 `\verbatiminput{/etc/passwd}`,
	"lstinputlisting":               `\lstinputlisting{/etc/passwd}`,
	"catchfile":                     `\CatchFileDef\x{/etc/passwd}{}`,
	"pdffiledump":                   `\pdffiledump offset 0 length 100{/etc/passwd}`,
	"includegraphics absolute":      `\includegraphics{/etc/passwd}`,
	"includegraphics parent":        `\includegraphics[width=1cm]{../../secret.png}`,
	"escaped percent hides nothing": `100\%\directlua{x}`,
	"comment ends at newline":       "% harmless\n\\directlua{x}",
}

func sharedHas(name string) bool { return name == "neutral.tex" }

func TestScanTeXRejectsHostileSnippets(t *testing.T) {
	for name, snippet := range hostileSnippets {
		if issues := scanTeX(snippet, sharedHas); len(issues) == 0 {
			t.Errorf("%s: %q was not rejected", name, snippet)
		}
	}
}

func TestScanTeXAcceptsBenignSnippets(t *testing.T) {
	benign := map[string]string{
		"input shared":            `\input{shared/neutral.tex}`,
		"comment":                 "% \\directlua{x} is commented out\ntext",
		"similar names":           `\writeup \inputs \directluas \readme`,
		"includegraphics":         `\includegraphics[height=18mm]{logo.pdf}`,
		"includegraphics marker":  "\\includegraphics{\x01}",
		"escaped percent":         `100\% sure`,
		"usepackage":              `\usepackage[hidelinks]{hyperref}`,
		"hook":                    `\AddToHook{shipout/background}{\put(0,0){x}}`,
		"input shared whitespace": `\input {shared/neutral.tex}`,
	}
	for name, snippet := range benign {
		if issues := scanTeX(snippet, sharedHas); len(issues) != 0 {
			t.Errorf("%s: unexpected issues %v", name, issues)
		}
	}
}

func TestScanTeXReportsLine(t *testing.T) {
	issues := scanTeX("ok\nstill ok\n\\directlua{x}\n", sharedHas)
	if len(issues) != 1 || issues[0].Line != 3 {
		t.Fatalf("issues = %+v, want one on line 3", issues)
	}
	if !strings.Contains(issues[0].Msg, `\directlua`) {
		t.Fatalf("message should name the primitive: %q", issues[0].Msg)
	}
}

func TestScanTeXCollectsAllIssues(t *testing.T) {
	issues := scanTeX("\\directlua{a}\n\\openout1=x\n\\catcode1=2\n", sharedHas)
	if len(issues) != 3 {
		t.Fatalf("issues = %+v, want 3", issues)
	}
}
