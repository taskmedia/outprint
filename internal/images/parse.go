// Package images is the Gateway's image pipeline. It finds the images of a
// Document in its Markdown, fetches each one from a Source, canonicalises it
// (format by magic bytes, pixel limit, decode and re-encode, EXIF
// orientation, metadata dropped) and rewrites the Markdown so that every
// image refers to a generated bundle name with an explicit size. All
// problems of a Document are collected, never just the first.
package images

import (
	"bytes"
	"unicode/utf8"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

const imageOpen = "!["

// Ref is one image reference in the Markdown body.
type Ref struct {
	// Start and End are the byte range of the image in the body.
	Start, End int
	// Line and Col are the 1-based position of Start in the body.
	Line, Col int
	// Alt is the alt text as plain text.
	Alt   string
	Dest  string
	Title string
	// Unsupported is set when the image's source range could not be
	// determined reliably, for example because it spans lines.
	Unsupported string
}

// ParseRefs returns the images of body in document order: outermost images
// only, inside links, lists, quotes and tables, never inside code. It reads
// the CommonMark dialect of goldmark with pipe tables and strikethrough,
// which is close to what Pandoc reads. Where the two disagree, the Worker's
// filter still rejects any image that is not a bundle file, so a
// disagreement fails a render and never lets something unchecked through.
func ParseRefs(body []byte) []Ref {
	tr := newTracker(body)
	md := goldmark.New(
		goldmark.WithParser(parser.NewParser(
			parser.WithBlockParsers(parser.DefaultBlockParsers()...),
			parser.WithInlineParsers(inlineParsers(tr)...),
			parser.WithParagraphTransformers(parser.DefaultParagraphTransformers()...),
		)),
		goldmark.WithExtensions(extension.Table, extension.Strikethrough),
	)
	doc := md.Parser().Parse(text.NewReader(body))
	return tr.collect(doc)
}

// inlineParsers are goldmark's defaults with the link parser replaced by a
// tracker around it. goldmark records no source range for a link or image,
// and the rewrite needs one.
func inlineParsers(tr *tracker) []util.PrioritizedValue {
	var out []util.PrioritizedValue
	for _, p := range parser.DefaultInlineParsers() {
		ip := p.Value.(parser.InlineParser)
		if bytes.IndexByte(ip.Trigger(), ']') >= 0 {
			tr.link = ip
			out = append(out, util.Prioritized(tr, p.Priority))
			continue
		}
		out = append(out, p)
	}
	return out
}

type span struct{ start, end int }

// tracker wraps goldmark's link parser. It remembers where every "![" and
// "[" began and, when a "]" closes an image, the range from the "!" to the
// end of the image syntax.
type tracker struct {
	link   parser.InlineParser
	src    []byte
	starts map[ast.Node]int
	ranges map[ast.Node]span
	// unsupported holds images whose range could not be fixed.
	unsupported map[ast.Node]string
}

func newTracker(src []byte) *tracker {
	return &tracker{
		src:         src,
		starts:      map[ast.Node]int{},
		ranges:      map[ast.Node]span{},
		unsupported: map[ast.Node]string{},
	}
}

func (t *tracker) Trigger() []byte { return t.link.Trigger() }

// CloseBlock hands over to the link parser, which turns unclosed brackets
// into text.
func (t *tracker) CloseBlock(parent ast.Node, block text.Reader, pc parser.Context) {
	if c, ok := t.link.(parser.CloseBlocker); ok {
		c.CloseBlock(parent, block, pc)
	}
}

func (t *tracker) Parse(parent ast.Node, block text.Reader, pc parser.Context) ast.Node {
	line, before := block.Position()
	first, _ := block.PeekLine()
	var open node
	if len(first) > 0 && first[0] == ']' {
		open = t.nearestOpener(parent)
	}
	n := t.link.Parse(parent, block, pc)
	if n == nil {
		return nil
	}
	if len(first) > 0 && (first[0] == '!' || first[0] == '[') {
		t.starts[n] = before.Start
		return n
	}
	if img, ok := n.(*ast.Image); ok && open.found {
		endLine, after := block.Position()
		t.record(img, open, line, endLine, after.Start)
	}
	if open.found {
		delete(t.starts, open.n)
	}
	return n
}

type node struct {
	n     ast.Node
	start int
	found bool
}

func (t *tracker) nearestOpener(parent ast.Node) node {
	for c := parent.LastChild(); c != nil; c = c.PreviousSibling() {
		if s, ok := t.starts[c]; ok {
			return node{n: c, start: s, found: true}
		}
	}
	return node{}
}

func (t *tracker) record(img *ast.Image, open node, line, endLine, end int) {
	if endLine != line || end > len(t.src) || end <= open.start+len(imageOpen) {
		t.unsupported[img] = "the image syntax spans lines"
		return
	}
	r := t.src[open.start:end]
	last := r[len(r)-1]
	if !bytes.HasPrefix(r, []byte(imageOpen)) || bytes.IndexByte(r, '\n') >= 0 || (last != ')' && last != ']') {
		t.unsupported[img] = "the image syntax is not supported here"
		return
	}
	t.ranges[img] = span{open.start, end}
}

// collect walks the AST and returns every outermost image with a position.
func (t *tracker) collect(doc ast.Node) []Ref {
	idx := newLineIndex(t.src)
	var refs []Ref
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		img, ok := n.(*ast.Image)
		if !entering || !ok {
			return ast.WalkContinue, nil
		}
		ref := Ref{
			Alt:   plainText(img, t.src),
			Dest:  string(img.Destination),
			Title: string(img.Title),
		}
		if sp, ok := t.ranges[img]; ok {
			ref.Start, ref.End = sp.start, sp.end
			ref.Line, ref.Col = idx.position(sp.start)
		} else {
			ref.Unsupported = firstNonEmpty(t.unsupported[img], "the image syntax is not supported here")
			ref.Line, ref.Col = idx.position(firstChildOffset(img))
		}
		refs = append(refs, ref)
		return ast.WalkSkipChildren, nil
	})
	return refs
}

func firstChildOffset(img *ast.Image) int {
	if tx, ok := img.FirstChild().(*ast.Text); ok {
		return tx.Segment.Start
	}
	return 0
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// plainText is the text of a node without markup, as Outline stores an
// alt text: line breaks become spaces, escapes and entities are resolved.
func plainText(n ast.Node, src []byte) string {
	var b bytes.Buffer
	appendText(&b, n, src)
	return b.String()
}

func appendText(b *bytes.Buffer, n ast.Node, src []byte) {
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		switch v := c.(type) {
		case *ast.Text:
			b.Write(util.ResolveEntityNames(util.UnescapePunctuations(v.Segment.Value(src))))
			if v.SoftLineBreak() || v.HardLineBreak() {
				b.WriteByte(' ')
			}
		case *ast.String:
			b.Write(v.Value)
		case *ast.RawHTML:
			// Dropped by the Worker's filter as well.
		default:
			appendText(b, c, src)
		}
	}
}

// lineIndex maps byte offsets to line and column.
type lineIndex struct {
	src    []byte
	starts []int
}

func newLineIndex(src []byte) lineIndex {
	starts := []int{0}
	for i, c := range src {
		if c == '\n' {
			starts = append(starts, i+1)
		}
	}
	return lineIndex{src: src, starts: starts}
}

// position returns the 1-based line and the 1-based column in characters.
func (l lineIndex) position(offset int) (int, int) {
	if offset > len(l.src) {
		offset = len(l.src)
	}
	lo, hi := 0, len(l.starts)-1
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if l.starts[mid] <= offset {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return lo + 1, utf8.RuneCount(l.src[l.starts[lo]:offset]) + 1
}
