package gateway

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	maxFilenameRunes = 100
	fallbackName     = "document"
	pdfSuffix        = ".pdf"
)

// asciiFolds are the few non-ASCII letters worth a readable ASCII fallback;
// every other non-ASCII rune becomes an underscore there. The UTF-8 name in
// filename* keeps them all.
var asciiFolds = map[rune]string{
	'ä': "ae", 'ö': "oe", 'ü': "ue", 'Ä': "Ae", 'Ö': "Oe", 'Ü': "Ue", 'ß': "ss",
}

// contentDisposition returns an attachment header whose file name comes from
// the Document title: an ASCII-safe filename and an RFC 5987 filename* with
// the UTF-8 name. Path separators, quotes, control characters (including CR
// and LF) and other characters that matter to a file system or a header are
// replaced; the length is capped.
func contentDisposition(title string) string {
	name := sanitiseTitle(title)
	return fmt.Sprintf(`attachment; filename="%s%s"; filename*=UTF-8''%s%s`,
		asciiName(name), pdfSuffix, percentEncode(name), percentEncode(pdfSuffix))
}

func sanitiseTitle(title string) string {
	var b strings.Builder
	for _, r := range title {
		switch {
		case unicode.IsControl(r), unicode.Is(unicode.Cf, r), strings.ContainsRune(`/\:*?"<>|;%`+"`", r):
			b.WriteRune(' ')
		case unicode.IsSpace(r):
			b.WriteRune(' ')
		default:
			b.WriteRune(r)
		}
	}
	name := strings.Join(strings.Fields(b.String()), " ")
	name = truncateRunes(name, maxFilenameRunes)
	name = strings.Trim(name, " .")
	if name == "" {
		return fallbackName
	}
	return name
}

func truncateRunes(s string, limit int) string {
	if utf8.RuneCountInString(s) <= limit {
		return s
	}
	return string([]rune(s)[:limit])
}

func asciiName(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r < unicode.MaxASCII && r >= ' ':
			b.WriteRune(r)
		case asciiFolds[r] != "":
			b.WriteString(asciiFolds[r])
		default:
			b.WriteByte('_')
		}
	}
	return b.String()
}

// percentEncode encodes every byte except the attr-char set of RFC 5987.
func percentEncode(s string) string {
	var b strings.Builder
	for _, c := range []byte(s) {
		if isAttrChar(c) {
			b.WriteByte(c)
			continue
		}
		fmt.Fprintf(&b, "%%%02X", c)
	}
	return b.String()
}

func isAttrChar(c byte) bool {
	switch {
	case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		return true
	}
	return strings.IndexByte("!#$&+-.^_`|~", c) >= 0
}
