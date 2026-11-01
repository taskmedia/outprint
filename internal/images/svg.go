package images

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"strings"

	"github.com/taskmedia/outprint/internal/limits"
)

const (
	svgNamespace = "http://www.w3.org/2000/svg"
	utf8BOM      = "\xef\xbb\xbf"
	extSVG       = "svg"
)

var (
	utf16LittleEndianBOM = []byte{0xff, 0xfe}
	utf16BigEndianBOM    = []byte{0xfe, 0xff}
)

func isUTF16(data []byte) bool {
	return bytes.HasPrefix(data, utf16LittleEndianBOM) || bytes.HasPrefix(data, utf16BigEndianBOM)
}

// canonicaliseSVG is the Gateway's SVG pre-check. It does not rasterise or
// rewrite: the SVG travels in the bundle as it is (without a UTF-8 byte
// order mark) and the Worker converts it to a vector PDF. What it does is
// refuse every SVG that could make the converter read a file, reach a
// network or run for long, so that the Worker's sandbox is a second line of
// defence and not the only one.
//
// Only UTF-8 is accepted. UTF-16 is refused rather than converted because
// the conversion would give the pre-check and the converter two different
// views of the same bytes.
func canonicaliseSVG(data []byte) (Canonical, *Rejection) {
	if len(data) > limits.MaxSVGBytes {
		return Canonical{}, reject(KindTooLarge, "the SVG is %s, at most %s", mib(len(data)), mib(limits.MaxSVGBytes))
	}
	if isUTF16(data) {
		return Canonical{}, reject(KindSVGRejected, "the SVG is encoded as UTF-16; save it as UTF-8")
	}
	data = bytes.TrimPrefix(data, []byte(utf8BOM))
	if rej := rejectDeclarations(data); rej != nil {
		return Canonical{}, rej
	}
	w := &svgWalker{}
	if rej := w.walk(data); rej != nil {
		return Canonical{}, rej
	}
	width, height, rej := w.naturalSize()
	if rej != nil {
		return Canonical{}, rej
	}
	if rej := w.checkExpansion(); rej != nil {
		return Canonical{}, rej
	}
	return Canonical{Data: data, Ext: extSVG, Width: width, Height: height}, nil
}

// rejectDeclarations refuses a DOCTYPE or ENTITY declaration wherever it is,
// comments and CDATA included. The token walk would stop on one as well; the
// plain scan means no parser's quirk can hide one.
func rejectDeclarations(data []byte) *Rejection {
	upper := bytes.ToUpper(data)
	switch {
	case bytes.Contains(upper, []byte("<!DOCTYPE")):
		return reject(KindSVGRejected, "the SVG has a DOCTYPE declaration, which is not allowed; remove it")
	case bytes.Contains(upper, []byte("<!ENTITY")):
		return reject(KindSVGRejected, "the SVG has an ENTITY declaration, which is not allowed")
	}
	return nil
}

// svgNode is what the expansion check needs of an element.
type svgNode struct {
	parent    int
	id        string
	useTarget string
	isStyle   bool
}

type svgWalker struct {
	nodes    []svgNode
	stack    []int
	style    strings.Builder
	rootSeen bool
	rootDone bool
	viewBox  string
	width    string
	height   string
}

func (w *svgWalker) walk(data []byte) *Rejection {
	dec := xml.NewDecoder(bytes.NewReader(data))
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return xmlFailure(err)
		}
		if rej := w.token(tok, dec); rej != nil {
			return rej
		}
	}
	if !w.rootSeen || len(w.stack) != 0 {
		return reject(KindSVGRejected, "the SVG is not well-formed XML: it has no complete root element")
	}
	return nil
}

func xmlFailure(err error) *Rejection {
	var syntax *xml.SyntaxError
	if errors.As(err, &syntax) {
		return reject(KindSVGRejected, "the SVG is not well-formed XML (line %d); only the five predefined entities are supported", syntax.Line)
	}
	if strings.Contains(err.Error(), "encoding") {
		return reject(KindSVGRejected, "the SVG declares an encoding other than UTF-8; save it as UTF-8")
	}
	return reject(KindSVGRejected, "the SVG is not well-formed XML")
}

func (w *svgWalker) token(tok xml.Token, dec *xml.Decoder) *Rejection {
	switch t := tok.(type) {
	case xml.StartElement:
		return w.start(t)
	case xml.EndElement:
		return w.end()
	case xml.CharData:
		return w.text(t)
	case xml.ProcInst:
		if t.Target != "xml" || w.rootSeen {
			return reject(KindSVGRejected, "the SVG has a processing instruction (%s), which is not allowed", shortToken(t.Target))
		}
	case xml.Directive:
		return reject(KindSVGRejected, "the SVG has a DOCTYPE or ENTITY declaration, which is not allowed")
	}
	return nil
}

func (w *svgWalker) text(t xml.CharData) *Rejection {
	switch {
	case len(w.stack) == 0 && len(bytes.TrimSpace(t)) > 0:
		return reject(KindSVGRejected, "the SVG has text outside or after the root element")
	case w.insideStyle():
		if w.style.Len()+len(t) > limits.MaxSVGBytes {
			return reject(KindSVGRejected, "the SVG style sheet is too long")
		}
		w.style.Write(t)
	}
	return nil
}

func (w *svgWalker) insideStyle() bool {
	return len(w.stack) > 0 && w.nodes[w.stack[len(w.stack)-1]].isStyle
}

func (w *svgWalker) start(e xml.StartElement) *Rejection {
	if w.rootDone {
		return reject(KindSVGRejected, "the SVG has more than one root element")
	}
	if !w.rootSeen {
		if rej := w.root(e); rej != nil {
			return rej
		}
	}
	if len(w.nodes) >= limits.MaxSVGElements {
		return reject(KindSVGRejected, "the SVG has more than %d elements", limits.MaxSVGElements)
	}
	if len(w.stack) >= limits.MaxSVGDepth {
		return reject(KindSVGRejected, "the SVG elements are nested more than %d levels deep", limits.MaxSVGDepth)
	}
	name := strings.ToLower(e.Name.Local)
	if rej := checkElementName(name); rej != nil {
		return rej
	}
	node, rej := checkAttributes(name, e.Attr)
	if rej != nil {
		return rej
	}
	node.parent = -1
	if len(w.stack) > 0 {
		node.parent = w.stack[len(w.stack)-1]
	}
	if name == "style" {
		node.isStyle = true
		w.style.Reset()
	}
	w.nodes = append(w.nodes, node)
	w.stack = append(w.stack, len(w.nodes)-1)
	return nil
}

func (w *svgWalker) root(e xml.StartElement) *Rejection {
	if !strings.EqualFold(e.Name.Local, "svg") {
		return reject(KindFormat, "this is XML, but not an SVG (its root element is <%s>); %s", shortToken(e.Name.Local), uploadHint)
	}
	if e.Name.Space != svgNamespace {
		return reject(KindSVGRejected, "the SVG root element is not in the namespace %s; add xmlns=\"%s\"", svgNamespace, svgNamespace)
	}
	w.rootSeen = true
	for _, a := range e.Attr {
		switch a.Name.Local {
		case "width":
			w.width = a.Value
		case "height":
			w.height = a.Value
		case "viewBox":
			w.viewBox = a.Value
		}
	}
	return nil
}

func (w *svgWalker) end() *Rejection {
	top := w.stack[len(w.stack)-1]
	w.stack = w.stack[:len(w.stack)-1]
	if w.nodes[top].isStyle {
		if rej := checkCSS(w.style.String()); rej != nil {
			return rej
		}
		w.style.Reset()
	}
	if len(w.stack) == 0 {
		w.rootDone = true
	}
	return nil
}

func checkElementName(lower string) *Rejection {
	switch lower {
	case "script":
		return reject(KindSVGRejected, "the SVG has a <script> element, which is not allowed")
	case "foreignobject":
		return reject(KindSVGRejected, "the SVG has a <foreignObject> element, which is not allowed")
	}
	return nil
}
