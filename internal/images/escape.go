package images

import (
	"regexp"
	"strings"
	"unicode"
)

// plainPunctuation is ASCII punctuation that has no meaning inside the alt
// text of a Pandoc image and is left alone, so that smart quotes, dashes and
// ellipses still work in a caption. Every other ASCII punctuation character
// is backslash-escaped.
const plainPunctuation = `'".,:;-?%/+=`

// escapeAlt turns plain text into an alt text that Pandoc reads back as
// exactly that text, whatever it holds: no bracket, brace, backtick or other
// markup character survives unescaped, and line breaks and control
// characters become spaces or vanish.
func escapeAlt(s string) string {
	var b strings.Builder
	space := false
	for _, r := range s {
		switch {
		case unicode.IsSpace(r) || r == ' ' || r == ' ':
			space = b.Len() > 0
		case unicode.IsControl(r):
		default:
			if space {
				b.WriteByte(' ')
				space = false
			}
			if isASCIIMarkup(r) {
				b.WriteByte('\\')
			}
			b.WriteRune(r)
		}
	}
	return b.String()
}

func isASCIIMarkup(r rune) bool {
	return r < unicode.MaxASCII && (unicode.IsPunct(r) || unicode.IsSymbol(r)) && !strings.ContainsRune(plainPunctuation, r)
}

// attributeBlockRE is a Pandoc attribute block directly after an image:
// identifier, classes and key=value pairs.
var attributeBlockRE = regexp.MustCompile(
	`^\{(?:[ \t]*(?:#[^\s{}"]+|\.[^\s{}"]+|[A-Za-z_:][-\w:.]*=(?:"[^"{}\n]*"|[^\s{}"]*)))+[ \t]*\}`)

// attributeBlockLen is the length of an attribute block at the start of s,
// or 0. A block written by the author is swallowed, so that only the
// attributes this package generates reach Pandoc.
func attributeBlockLen(s []byte) int {
	loc := attributeBlockRE.Find(s)
	return len(loc)
}

// truncate shortens text to at most n characters for an error message.
func truncate(s string, n int) string {
	runes := []rune(strings.Join(strings.Fields(s), " "))
	if len(runes) <= n {
		return string(runes)
	}
	return string(runes[:n-1]) + "…"
}
