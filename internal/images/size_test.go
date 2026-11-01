package images

import (
	"math"
	"strings"
	"testing"
)

func TestParseSize(t *testing.T) {
	tests := []struct {
		title string
		want  Size
		ok    bool
	}{
		{" =400x300", Size{400, 300}, true},
		{"=400x300", Size{400, 300}, true},
		{"=400x", Size{400, 0}, true},
		{"=x300", Size{0, 300}, true},
		{"=400", Size{400, 0}, true},
		{"Caption text =400x300", Size{400, 300}, true},
		{"  =12x34  ", Size{12, 34}, true},
		{"", Size{}, false},
		{"=", Size{}, false},
		{"=x", Size{}, false},
		{"plain title", Size{}, false},
		{"a=400x300", Size{}, false},
		{"=400x300 more", Size{}, false},
		{"=1234567x1", Size{}, false},
		{"=-4x3", Size{}, false},
	}
	for _, tt := range tests {
		got, ok := ParseSize(tt.title)
		if got != tt.want || ok != tt.ok {
			t.Errorf("%q: %v %v, want %v %v", tt.title, got, ok, tt.want, tt.ok)
		}
	}
}

func near(a, b float64) bool { return math.Abs(a-b) < 0.01 }

func TestPoints(t *testing.T) {
	tests := []struct {
		name   string
		w, h   float64
		req    Size
		pw, ph float64
	}{
		{"natural size at 96 dpi", 400, 200, Size{}, 300, 150},
		{"both sides", 400, 200, Size{200, 200}, 150, 150},
		{"width only derives height", 400, 200, Size{200, 0}, 150, 75},
		{"height only derives width", 400, 200, Size{0, 100}, 150, 75},
		{"huge request is capped with the aspect kept", 400, 200, Size{99999, 0}, 10000, 5000},
		{"tiny request has a floor", 400, 200, Size{1, 0}, 1, 1},
		{"a tall natural image is capped, width floored", 10, 90000, Size{}, 1.11, 10000},
	}
	for _, tt := range tests {
		pw, ph := Points(tt.w, tt.h, tt.req)
		if !near(pw, tt.pw) || !near(ph, tt.ph) {
			t.Errorf("%s: %.2f x %.2f, want %.2f x %.2f", tt.name, pw, ph, tt.pw, tt.ph)
		}
	}
	if got := formatPoints(7.5); got != "7.50pt" {
		t.Errorf("format = %s", got)
	}
}

func TestEscapeAlt(t *testing.T) {
	tests := []struct{ in, want string }{
		{"plain words", "plain words"},
		{"Fig. 1: it's a \"test\" - ok?", `Fig. 1: it's a "test" - ok?`},
		{"under_score *star* [br] {br} `tick` ~ ^ | < > $ # @ ! \\ &", "under\\_score \\*star\\* \\[br\\] \\{br\\} \\`tick\\` \\~ \\^ \\| \\< \\> \\$ \\# \\@ \\! \\\\ \\&"},
		{"  lots \n of \t\t space  ", "lots of space"},
		{"ctrl\x00\x1b\x7f gone", "ctrl gone"},
		{"umlaut äöü ß and 日本", "umlaut äöü ß and 日本"},
		{"sep arated", "sep arated"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := escapeAlt(tt.in); got != tt.want {
			t.Errorf("%q: got %q want %q", tt.in, got, tt.want)
		}
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("a\n b", 10); got != "a b" {
		t.Error(got)
	}
	got := truncate(strings.Repeat("ä", 50), 40)
	if len([]rune(got)) != 40 || !strings.HasSuffix(got, "…") {
		t.Errorf("%q", got)
	}
}

func TestAttributeBlockLen(t *testing.T) {
	for in, want := range map[string]int{
		`{width=10pt}`:             12,
		`{#id .cls width="5cm"} x`: 22,
		`{curly}`:                  0,
		`{}`:                       0,
		`{width=10pt`:              0,
		` {width=10pt}`:            0,
		`{a=1}{b=2}`:               5,
		"{a=1\n}":                  0,
	} {
		if got := attributeBlockLen([]byte(in)); got != want {
			t.Errorf("%q: %d, want %d", in, got, want)
		}
	}
}
