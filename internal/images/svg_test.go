package images

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/taskmedia/outline-latex-pdf/internal/limits"
)

const svgOpen = `<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" width="40" height="20">`

func svgDoc(body string) []byte { return []byte(svgOpen + body + `</svg>`) }

func TestSVGAccepted(t *testing.T) {
	png := base64.StdEncoding.EncodeToString(pngBytes(t, 4, 4))
	tests := []struct {
		name string
		data []byte
		w, h float64
	}{
		{"plain", svgDoc(`<rect width="10" height="10"/>`), 40, 20},
		{"with prolog and comment", []byte("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<!-- made by hand -->\n" + string(svgDoc(``))), 40, 20},
		{"byte order mark", append([]byte("\xef\xbb\xbf"), svgDoc(``)...), 40, 20},
		{"viewBox only", []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 200 100"/>`), 200, 100},
		{"viewBox and width", []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 200 100" width="100"/>`), 100, 50},
		{"viewBox and height", []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0,0,200,100" height="50"/>`), 100, 50},
		{"points", []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="72pt" height="36pt"/>`), 96, 48},
		{"millimetres", []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="25.4mm" height="2.54cm"/>`), 96, 96},
		{"inches", []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="1in" height="0.5in"/>`), 96, 48},
		{"percent width falls back to viewBox", []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="100%" height="100%" viewBox="0 0 30 15"/>`), 30, 15},
		{"fragment use and url", svgDoc(`<defs><linearGradient id="g"/><g id="a"><rect fill="url(#g)"/></g></defs><use href="#a"/><use xlink:href="#a"/>`), 40, 20},
		{"embedded png", svgDoc(`<image width="4" height="4" href="data:image/png;base64,` + png + `"/>`), 40, 20},
		{"embedded png via xlink", svgDoc(`<image xlink:href="data:image/png;base64,` + png + `"/>`), 40, 20},
		{"inline style with fragment url", svgDoc(`<style>.a{fill:url(#g)} /* note */</style><rect style="fill:url('#g')"/>`), 40, 20},
		{"prefixed root", []byte(`<s:svg xmlns:s="http://www.w3.org/2000/svg" width="5" height="5"/>`), 5, 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, rej := Canonicalise(tt.data)
			if rej != nil {
				t.Fatalf("rejected: %v", rej)
			}
			if c.Ext != "svg" || c.Width != tt.w || c.Height != tt.h {
				t.Errorf("ext %q size %vx%v, want svg %vx%v", c.Ext, c.Width, c.Height, tt.w, tt.h)
			}
			if bytes.HasPrefix(c.Data, []byte("\xef\xbb\xbf")) {
				t.Error("the byte order mark was passed on")
			}
		})
	}
}

func TestSVGRejected(t *testing.T) {
	tooBig := base64.StdEncoding.EncodeToString(pngBytes(t, 8000, 6000))
	many := strings.Repeat("<g/>", limits.MaxSVGElements+1)
	deep := strings.Repeat("<g>", limits.MaxSVGDepth+1) + strings.Repeat("</g>", limits.MaxSVGDepth+1)
	tests := []struct {
		name string
		data []byte
		kind string
		msg  string
	}{
		{"xxe doctype", []byte(`<?xml version="1.0"?><!DOCTYPE svg [<!ENTITY xxe SYSTEM "file:///etc/passwd">]>` + string(svgDoc(`<text>&xxe;</text>`))), KindSVGRejected, "DOCTYPE"},
		{"billion laughs", []byte(`<!DOCTYPE svg [<!ENTITY a "aaaa"><!ENTITY b "&a;&a;&a;&a;">]>` + string(svgDoc(`<text>&b;</text>`))), KindSVGRejected, "DOCTYPE"},
		{"lowercase doctype", []byte(`<!doctype svg>` + string(svgDoc(``))), KindSVGRejected, "DOCTYPE"},
		{"entity declaration in a comment", svgDoc(`<!-- <!ENTITY x "y"> -->`), KindSVGRejected, "ENTITY"},
		{"undefined entity", svgDoc(`<text>&xxe;</text>`), KindSVGRejected, "well-formed"},
		{"script", svgDoc(`<script>alert(1)</script>`), KindSVGRejected, "<script>"},
		{"script uppercase", svgDoc(`<SCRIPT>alert(1)</SCRIPT>`), KindSVGRejected, "<script>"},
		{"script in a prefixed namespace", svgDoc(`<x:script xmlns:x="http://www.w3.org/2000/svg"/>`), KindSVGRejected, "<script>"},
		{"foreignObject", svgDoc(`<foreignObject><div xmlns="http://www.w3.org/1999/xhtml">x</div></foreignObject>`), KindSVGRejected, "<foreignObject>"},
		{"onload", []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="1" height="1" onload="x()"/>`), KindSVGRejected, "event handler"},
		{"onclick on a child", svgDoc(`<rect onclick="x()"/>`), KindSVGRejected, "event handler"},
		{"image file href", svgDoc(`<image href="file:///etc/passwd"/>`), KindSVGRejected, "data:image"},
		{"image http href", svgDoc(`<image xlink:href="http://example.invalid/x.png"/>`), KindSVGRejected, "data:image"},
		{"image relative href", svgDoc(`<image href="other.png"/>`), KindSVGRejected, "data:image"},
		{"image fragment href", svgDoc(`<image href="#a"/>`), KindSVGRejected, "data:image"},
		{"image data svg", svgDoc(`<image href="data:image/svg+xml;base64,PHN2Zy8+"/>`), KindSVGRejected, "data:image"},
		{"image javascript", svgDoc(`<image href="javascript:alert(1)"/>`), KindSVGRejected, "data:image"},
		{"image obfuscated scheme", svgDoc("<image href=\"java\tscript:alert(1)\"/>"), KindSVGRejected, "data:image"},
		{"image data not base64", svgDoc(`<image href="data:image/png,abc"/>`), KindSVGRejected, "data:image"},
		{"image data broken base64", svgDoc(`<image href="data:image/png;base64,@@@@"/>`), KindSVGRejected, "embedded image"},
		{"image data not an image", svgDoc(`<image href="data:image/png;base64,` + base64.StdEncoding.EncodeToString([]byte("hello")) + `"/>`), KindSVGRejected, "embedded image"},
		{"image data too many pixels", svgDoc(`<image href="data:image/png;base64,` + tooBig + `"/>`), KindSVGRejected, "embedded image"},
		{"use external", svgDoc(`<use href="other.svg#a"/>`), KindSVGRejected, "#fragment"},
		{"use file", svgDoc(`<use xlink:href="file:///etc/passwd"/>`), KindSVGRejected, "#fragment"},
		{"use data", svgDoc(`<use href="data:image/png;base64,AAAA"/>`), KindSVGRejected, "#fragment"},
		{"anchor to the web", svgDoc(`<a href="https://example.invalid/"><rect/></a>`), KindSVGRejected, "#fragment"},
		{"stylesheet import", svgDoc(`<style>@import url(http://example.invalid/a.css);</style>`), KindSVGRejected, "@import"},
		{"stylesheet import uppercase", svgDoc(`<style>@IMPORT "a.css";</style>`), KindSVGRejected, "@import"},
		{"stylesheet url file", svgDoc(`<style>rect{fill:url(file:///etc/passwd)}</style>`), KindSVGRejected, "url()"},
		{"stylesheet url quoted http", svgDoc(`<style>rect{background:url("http://example.invalid/x")}</style>`), KindSVGRejected, "url()"},
		{"stylesheet css escape", svgDoc(`<style>@\69mport "a.css";</style>`), KindSVGRejected, "escape"},
		{"style attribute url", svgDoc(`<rect style="fill:url(http://example.invalid/x)"/>`), KindSVGRejected, "url()"},
		{"presentation attribute url", svgDoc(`<rect fill="url(file:///etc/passwd)"/>`), KindSVGRejected, "url()"},
		{"filter attribute url", svgDoc(`<rect filter="url(other.svg#f)"/>`), KindSVGRejected, "url()"},
		{"image-set", svgDoc(`<rect style="fill:image-set('x.png' 1x)"/>`), KindSVGRejected, "url()"},
		{"stylesheet processing instruction", []byte(`<?xml-stylesheet href="a.css"?>` + string(svgDoc(``))), KindSVGRejected, "processing instruction"},
		{"no namespace", []byte(`<svg width="1" height="1"/>`), KindSVGRejected, "namespace"},
		{"two roots", []byte(string(svgDoc(``)) + string(svgDoc(``))), KindSVGRejected, "more than one"},
		{"text after root", []byte(string(svgDoc(``)) + "tail"), KindSVGRejected, "after the root"},
		{"not well formed", []byte(`<svg xmlns="http://www.w3.org/2000/svg"><g></svg>`), KindSVGRejected, "well-formed"},
		{"empty after prolog", []byte(`<?xml version="1.0"?><!-- <svg> -->`), KindSVGRejected, "well-formed"},
		{"declared latin1", []byte("<?xml version=\"1.0\" encoding=\"ISO-8859-1\"?>" + string(svgDoc(``))), KindSVGRejected, "UTF-8"},
		{"utf16 with byte order mark", utf16Bytes(string(svgDoc(``))), KindSVGRejected, "UTF-16"},
		{"no size", []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`), KindSVGRejected, "size"},
		{"zero size", []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="0" height="10"/>`), KindSVGRejected, "size"},
		{"negative viewBox", []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 -5 5"/>`), KindSVGRejected, "viewBox"},
		{"broken viewBox", []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 5"/>`), KindSVGRejected, "viewBox"},
		{"huge width", []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="1e9" height="10"/>`), KindSVGRejected, "size"},
		{"huge viewBox", []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10" viewBox="0 0 1000000000 1000000000"/>`), KindSVGRejected, "viewBox"},
		{"huge size in inches", []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="5000in" height="1in"/>`), KindSVGRejected, "size"},
		{"nan size is not a number", []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="NaN" height="Inf" viewBox="0 0 5 5"/>`), "", ""},
		{"too many elements", svgDoc(many), KindSVGRejected, "elements"},
		{"too deep", svgDoc(deep), KindSVGRejected, "nested"},
		{"use bomb", svgDoc(useBomb(12, 10)), KindSVGRejected, "expands"},
		{"use cycle", svgDoc(`<g id="a"><use href="#b"/></g><g id="b"><use href="#a"/></g>`), KindSVGRejected, "circular"},
		{"use of itself", svgDoc(`<g id="a"><use href="#a"/></g>`), KindSVGRejected, "circular"},
		{"over 2 MiB", append(svgDoc(`<!-- `), bytes.Repeat([]byte("x"), limits.MaxSVGBytes)...), KindTooLarge, "2.0 MiB"},
		{"xml but not svg", []byte(`<?xml version="1.0"?><note><svg/></note>`), KindFormat, "not an SVG"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, rej := Canonicalise(tt.data)
			if tt.kind == "" {
				if rej != nil {
					t.Fatalf("rejected: %v", rej)
				}
				return
			}
			if rej == nil {
				t.Fatalf("accepted as %+v", c)
			}
			if rej.Kind != tt.kind || !strings.Contains(rej.Message, tt.msg) {
				t.Errorf("got %v, want kind %s with %q", rej, tt.kind, tt.msg)
			}
		})
	}
}

func TestSVGMessagesDoNotEchoTheDocument(t *testing.T) {
	secret := "SECRET-" + strings.Repeat("z", 200)
	for name, data := range map[string][]byte{
		"element":   svgDoc(`<` + secret + ` onload="x"/>`),
		"href":      svgDoc(`<use href="` + secret + `"/>`),
		"url":       svgDoc(`<rect fill="url(` + secret + `)"/>`),
		"handler":   svgDoc(`<rect on` + secret + `="x"/>`),
		"namespace": []byte(`<svg xmlns="` + secret + `"/>`),
		"encoding":  []byte(`<?xml version="1.0" encoding="` + secret + `"?>` + string(svgDoc(``))),
	} {
		_, rej := Canonicalise(data)
		if rej == nil {
			t.Fatalf("%s: accepted", name)
		}
		if strings.Contains(rej.Message, secret) || len(rej.Message) > 300 {
			t.Errorf("%s: message quotes the document: %q", name, rej.Message)
		}
	}
}

func TestSVGDetectionDoesNotTakeHTMLOrXMLForSVG(t *testing.T) {
	for name, data := range map[string]string{
		"html page":      "<!DOCTYPE html><html><body><svg></svg></body></html>",
		"html lowercase": "<html><body><svg></svg></body></html>",
	} {
		_, rej := Canonicalise([]byte(data))
		if rej == nil || rej.Kind != KindFormat || !strings.Contains(rej.Message, "HTML") {
			t.Errorf("%s: %v", name, rej)
		}
	}
}

// useBomb is a chain of depth groups that each use the previous one fanout
// times: depth*fanout elements that expand to fanout^depth.
func useBomb(depth, fanout int) string {
	var b strings.Builder
	b.WriteString(`<g id="l0"><rect width="1" height="1"/></g>`)
	for i := 1; i <= depth; i++ {
		fmt.Fprintf(&b, `<g id="l%d">`, i)
		for range fanout {
			fmt.Fprintf(&b, `<use href="#l%d"/>`, i-1)
		}
		b.WriteString(`</g>`)
	}
	fmt.Fprintf(&b, `<use href="#l%d"/>`, depth)
	return b.String()
}

func utf16Bytes(s string) []byte {
	out := []byte{0xff, 0xfe}
	for _, u := range utf16.Encode([]rune(s)) {
		out = append(out, byte(u), byte(u>>8))
	}
	return out
}
