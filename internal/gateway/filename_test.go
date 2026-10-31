package gateway

import (
	"mime"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestContentDisposition(t *testing.T) {
	tests := []struct {
		name, title string
		wantName    string // decoded filename*
		wantASCII   string
	}{
		{"plain", "Spring plant sale", "Spring plant sale.pdf", "Spring plant sale.pdf"},
		{"non-ascii", "Grüße aus Köln", "Grüße aus Köln.pdf", "Gruesse aus Koeln.pdf"},
		{"other scripts", "Zażółć gęślą", "Zażółć gęślą.pdf", "Za____ g__l_.pdf"},
		{"empty", "", "document.pdf", "document.pdf"},
		{"only hostile", `../..//\`, "document.pdf", "document.pdf"},
		{"dots", "...", "document.pdf", "document.pdf"},
		{"path", "../../etc/passwd", "etc passwd.pdf", "etc passwd.pdf"},
		{"quotes", `Say "hi" now`, "Say hi now.pdf", "Say hi now.pdf"},
		{"crlf", "a\r\nSet-Cookie: x=1\r\nb", "a Set-Cookie x=1 b.pdf", "a Set-Cookie x=1 b.pdf"},
		{"reserved", "a:b*c?d<e>f|g;h%i", "a b c d e f g h i.pdf", "a b c d e f g h i.pdf"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			header := contentDisposition(tt.title)
			if strings.ContainsAny(header, "\r\n") {
				t.Fatalf("header contains CR or LF: %q", header)
			}
			disposition, params, err := mime.ParseMediaType(header)
			if err != nil || disposition != "attachment" {
				t.Fatalf("header %q: %v", header, err)
			}
			if params["filename"] != tt.wantName {
				t.Errorf("filename* = %q, want %q", params["filename"], tt.wantName)
			}
			ascii := between(header, `filename="`, `"`)
			if ascii != tt.wantASCII {
				t.Errorf("ASCII filename = %q, want %q", ascii, tt.wantASCII)
			}
			for _, r := range ascii {
				if r > 126 || r < 32 || strings.ContainsRune(`/\"`, r) {
					t.Errorf("ASCII filename %q has %q", ascii, r)
				}
			}
		})
	}
}

func TestContentDispositionCapsLength(t *testing.T) {
	header := contentDisposition(strings.Repeat("ä", 500))
	_, params, err := mime.ParseMediaType(header)
	if err != nil {
		t.Fatal(err)
	}
	if n := utf8.RuneCountInString(params["filename"]); n != maxFilenameRunes+len(pdfSuffix) {
		t.Errorf("name has %d runes", n)
	}
	if len(header) > 1500 {
		t.Errorf("header is %d bytes", len(header))
	}
}

func between(s, start, end string) string {
	_, rest, _ := strings.Cut(s, start)
	got, _, _ := strings.Cut(rest, end)
	return got
}
