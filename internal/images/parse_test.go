package images

import (
	"strings"
	"testing"
)

func snippets(body string, refs []Ref) []string {
	var out []string
	for _, r := range refs {
		if r.Unsupported != "" {
			out = append(out, "UNSUPPORTED:"+r.Unsupported)
			continue
		}
		out = append(out, body[r.Start:r.End])
	}
	return out
}

func TestParseRefsRanges(t *testing.T) {
	tests := []struct {
		name string
		body string
		want []string
	}{
		{"inline", "Text ![alt](/a.png) more", []string{"![alt](/a.png)"}},
		{"with title", `![alt](/a.png "=40x20") end`, []string{`![alt](/a.png "=40x20")`}},
		{"empty alt", "![](/a.png)", []string{"![](/a.png)"}},
		{"inside link", "[![alt](/a.png)](https://x.example.invalid)", []string{"![alt](/a.png)"}},
		{"in list", "- item\n- ![alt](/a.png)\n", []string{"![alt](/a.png)"}},
		{"in quote", "> quoted ![alt](/a.png) text\n", []string{"![alt](/a.png)"}},
		{"in table", "| a | b |\n|---|---|\n| ![x](/a.png) | y |\n", []string{"![x](/a.png)"}},
		{"reference", "![alt][r]\n\n[r]: /a.png\n", []string{"![alt][r]"}},
		{"collapsed reference", "![alt][]\n\n[alt]: /a.png\n", []string{"![alt][]"}},
		{"shortcut reference", "![alt]\n\n[alt]: /a.png\n", []string{"![alt]"}},
		{"two", "![a](/1.png) and ![b](/2.png)", []string{"![a](/1.png)", "![b](/2.png)"}},
		{"nested label brackets", "![a [b] c](/a.png)", []string{"![a [b] c](/a.png)"}},
		{"link before image", "[text](/x) ![a](/a.png)", []string{"![a](/a.png)"}},
		{"image alt holds image", "![a ![b](/b.png) c](/a.png)", []string{"![a ![b](/b.png) c](/a.png)"}},
		{"fenced code skipped", "```\n![a](/a.png)\n```\n", nil},
		{"indented code skipped", "    ![a](/a.png)\n", nil},
		{"code span skipped", "`![a](/a.png)`", nil},
		{"escaped bang", `\![a](/a.png)`, nil},
		{"angle destination", "![a](</my file.png>)", []string{"![a](</my file.png>)"}},
		{"multi line", "![a](/a.png\n\"title\")", []string{"UNSUPPORTED:the image syntax spans lines"}},
		{"multi line label", "![a\nb](/a.png)", []string{"UNSUPPORTED:the image syntax is not supported here"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := snippets(tt.body, ParseRefs([]byte(tt.body)))
			if strings.Join(got, "|") != strings.Join(tt.want, "|") {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseRefsFields(t *testing.T) {
	body := "# Title\n\nSome ä text ![Alt *with* \\_markup\\_ &amp; more](/api/attachments.redirect?id=x \"t\") end\n"
	refs := ParseRefs([]byte(body))
	if len(refs) != 1 {
		t.Fatalf("got %d refs", len(refs))
	}
	r := refs[0]
	if r.Line != 3 || r.Col != 13 {
		t.Errorf("position = %d:%d, want 3:13", r.Line, r.Col)
	}
	if r.Alt != "Alt with _markup_ & more" {
		t.Errorf("alt = %q", r.Alt)
	}
	if r.Dest != "/api/attachments.redirect?id=x" || r.Title != "t" {
		t.Errorf("dest = %q, title = %q", r.Dest, r.Title)
	}
}
