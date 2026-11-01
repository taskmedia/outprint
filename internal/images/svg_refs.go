package images

import (
	"bytes"
	"encoding/base64"
	"encoding/xml"
	"image"
	"strings"
	"unicode"

	_ "golang.org/x/image/webp" // registers the WebP decoder for embedded images

	"github.com/taskmedia/outline-latex-pdf/internal/limits"
)

const maxTokenRunes = 24

var embeddedImagePrefixes = []string{
	"data:image/png;base64,", "data:image/jpeg;base64,", "data:image/gif;base64,", "data:image/webp;base64,",
}

// shortToken makes a name from the document safe and short enough to put in
// a message: letters, digits and a few name characters, at most 24 of them.
func shortToken(s string) string {
	var b strings.Builder
	n := 0
	for _, r := range s {
		if n == maxTokenRunes {
			b.WriteString("...")
			break
		}
		if r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune("-_.:#", r)) {
			b.WriteRune(r)
			n++
		}
	}
	return b.String()
}

func checkAttributes(element string, attrs []xml.Attr) (svgNode, *Rejection) {
	var node svgNode
	for _, a := range attrs {
		name := strings.ToLower(a.Name.Local)
		switch {
		case strings.HasPrefix(name, "on"):
			return node, reject(KindSVGRejected, "the SVG has an event handler attribute (%s), which is not allowed", shortToken(name))
		case name == "href":
			target, rej := checkHref(element, a.Value)
			if rej != nil {
				return node, rej
			}
			if element == "use" {
				node.useTarget = target
			}
		case name == "id" && a.Name.Space == "":
			node.id = a.Value
		}
		if rej := checkURLs(a.Value); rej != nil {
			return node, rej
		}
		if name == "style" {
			if rej := checkCSS(a.Value); rej != nil {
				return node, rej
			}
		}
	}
	return node, nil
}

// checkHref allows a #fragment reference anywhere, and an embedded raster
// image on <image> and <feImage>. It returns the fragment, if any.
func checkHref(element, value string) (string, *Rejection) {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "#") {
		if element == "image" || element == "feimage" {
			return "", reject(KindSVGRejected, "an <image> in the SVG must embed its picture as data:image/png, jpeg, gif or webp;base64, not refer to %s", shortToken(value))
		}
		return value[1:], nil
	}
	if element != "image" && element != "feimage" {
		return "", reject(KindSVGRejected, "a reference in the SVG (%s) must be a #fragment of the same document", shortToken(value))
	}
	return "", checkEmbeddedImage(value)
}

func checkEmbeddedImage(value string) *Rejection {
	payload, ok := embeddedPayload(value)
	if !ok {
		return reject(KindSVGRejected, "an <image> in the SVG must embed its picture as data:image/png, jpeg, gif or webp;base64, not refer to %s", shortToken(value))
	}
	raw, err := base64.StdEncoding.DecodeString(stripSpace(payload))
	if err != nil {
		return reject(KindSVGRejected, "an embedded image in the SVG is not valid base64")
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 {
		return reject(KindSVGRejected, "an embedded image in the SVG is not a valid PNG, JPEG, GIF or WebP")
	}
	if int64(cfg.Width)*int64(cfg.Height) > limits.MaxImagePixels {
		return reject(KindSVGRejected, "an embedded image in the SVG has more than %d megapixels", limits.MaxImagePixels/1_000_000)
	}
	return nil
}

func embeddedPayload(value string) (string, bool) {
	lower := strings.ToLower(value)
	for _, prefix := range embeddedImagePrefixes {
		if strings.HasPrefix(lower, prefix) {
			return value[len(prefix):], true
		}
	}
	return "", false
}

func stripSpace(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, s)
}

// checkURLs refuses a url() that does not point at a #fragment, in any
// attribute value (fill, filter, mask, clip-path, marker and so on).
func checkURLs(value string) *Rejection {
	lower := strings.ToLower(value)
	if strings.Contains(lower, "image-set(") {
		return reject(KindSVGRejected, "the SVG uses image-set(), which is not allowed; url() may only point at a #fragment")
	}
	for rest := lower; ; {
		_, after, found := strings.Cut(rest, "url(")
		if !found {
			return nil
		}
		arg, remainder, closed := strings.Cut(after, ")")
		if !closed {
			return reject(KindSVGRejected, "the SVG has a url() that is not closed")
		}
		arg = strings.Trim(strings.TrimSpace(arg), `"'`)
		if !strings.HasPrefix(strings.TrimSpace(arg), "#") {
			return reject(KindSVGRejected, "the SVG has a url() that points outside the document (%s); only url(#fragment) is allowed", shortToken(arg))
		}
		rest = remainder
	}
}

// checkCSS checks the text of a <style> element or a style attribute. CSS
// escapes are refused because they can spell any keyword in a way a plain
// scan does not see.
func checkCSS(css string) *Rejection {
	if strings.Contains(css, `\`) {
		return reject(KindSVGRejected, "the SVG style uses a CSS escape (backslash), which is not allowed")
	}
	if strings.Contains(strings.ToLower(css), "@import") {
		return reject(KindSVGRejected, "the SVG style has an @import, which is not allowed")
	}
	return checkURLs(css)
}
