package images

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/taskmedia/outline-latex-pdf/internal/limits"
	"github.com/taskmedia/outline-latex-pdf/internal/metadata"
)

const (
	altTruncate  = 40
	bundleDir    = "images/"
	imageNameFmt = "img-%d.%s"
)

// Result is a Document whose images are all in the bundle.
type Result struct {
	// Markdown refers to every image as images/<name> with a size.
	Markdown []byte
	// Images are the canonical files by bundle name.
	Images map[string][]byte
}

// Problem is one image problem with its place in the Document.
type Problem struct {
	Kind, Message string
	Line, Col     int
	Alt           string
}

// Error converts the problem to an entry of the 422 list. The field is the
// alt text (truncated), or "image" when there is none.
func (p Problem) Error() metadata.Error {
	line := p.Line
	return metadata.Error{
		Field:   firstNonEmpty(truncate(p.Alt, altTruncate), "image"),
		Source:  metadata.SourceDocument,
		Code:    p.Kind,
		Message: fmt.Sprintf("%s (line %d, column %d)", p.Message, p.Line, p.Col),
		Line:    &line,
	}
}

// Process finds, fetches, canonicalises and rewrites the images of body.
// lineOffset is the number of lines before body in the Document, so that
// positions are Document lines. The problems are all of them, in Document
// order; when there are any, Result is empty. The error is a failure of the
// Source that is not the author's doing, such as Outline being down.
func Process(ctx context.Context, body []byte, lineOffset int, src Source) (Result, []Problem, error) {
	p := &processor{src: src, lineOffset: lineOffset, seen: map[string]*entry{}, files: map[string][]byte{}}
	var edits []edit
	for _, ref := range ParseRefs(body) {
		e, err := p.resolve(ctx, ref)
		if err != nil {
			return Result{}, nil, err
		}
		if e != nil {
			edits = append(edits, newEdit(body, ref, e))
		}
	}
	if len(p.problems) > 0 {
		return Result{}, p.problems, nil
	}
	return Result{Markdown: applyEdits(body, edits), Images: p.files}, nil, nil
}

// entry is the outcome for one distinct image address.
type entry struct {
	name string
	img  Canonical
	rej  *Rejection
}

type processor struct {
	src        Source
	lineOffset int
	seen       map[string]*entry
	files      map[string][]byte
	total      int
	problems   []Problem
}

// resolve returns the entry for ref, or nil after recording a problem.
func (p *processor) resolve(ctx context.Context, ref Ref) (*entry, error) {
	if ref.Unsupported != "" {
		p.add(ref, reject(KindSyntax, "%s; put the image on a single line", ref.Unsupported))
		return nil, nil
	}
	key, rej := p.src.Locate(ref.Dest)
	if rej != nil {
		p.add(ref, rej)
		return nil, nil
	}
	e, ok := p.seen[key]
	if !ok {
		var err error
		if e, err = p.load(ctx, key); err != nil {
			return nil, err
		}
		p.seen[key] = e
	}
	if e.rej != nil {
		p.add(ref, e.rej)
		return nil, nil
	}
	return e, nil
}

func (p *processor) load(ctx context.Context, key string) (*entry, error) {
	if len(p.seen) >= limits.MaxImages {
		return &entry{rej: reject(KindTooLarge, "the document has more than %d images", limits.MaxImages)}, nil
	}
	data, err := p.src.Fetch(ctx, key)
	var blocked *BlockedError
	switch {
	case errors.As(err, &blocked):
		return &entry{rej: reject(KindForbidden, "the image cannot be fetched: %s", blocked.Reason)}, nil
	case errors.Is(err, ErrNotFound):
		return &entry{rej: reject(KindMissing, "the image was not found; it may have been deleted")}, nil
	case errors.Is(err, ErrForbidden):
		return &entry{rej: reject(KindForbidden, "you are not allowed to read this image")}, nil
	case errors.Is(err, ErrTooLarge):
		return &entry{rej: reject(KindTooLarge, "the image is larger than %s", mib(limits.MaxImageBytes))}, nil
	case err != nil:
		return nil, err
	case len(data) > limits.MaxImageBytes:
		return &entry{rej: reject(KindTooLarge, "the image is larger than %s", mib(limits.MaxImageBytes))}, nil
	}
	img, rej := Canonicalise(data)
	if rej != nil {
		return &entry{rej: rej}, nil
	}
	if p.total+len(img.Data) > limits.MaxImagesTotal {
		return &entry{rej: reject(KindTooLarge, "the images of this document together exceed %s", mib(limits.MaxImagesTotal))}, nil
	}
	p.total += len(img.Data)
	name := fmt.Sprintf(imageNameFmt, len(p.files)+1, img.Ext)
	p.files[name] = img.Data
	return &entry{name: name, img: img}, nil
}

func (p *processor) add(ref Ref, rej *Rejection) {
	p.problems = append(p.problems, Problem{
		Kind: rej.Kind, Message: rej.Message, Line: ref.Line + p.lineOffset, Col: ref.Col, Alt: ref.Alt,
	})
}

// Combine returns the one error list for the problems of the Metadata Block
// (metaErr, an *ErrorList, or nil) and the image problems, metadata first.
// It is nil when there are none.
func Combine(metaList *metadata.ErrorList, problems []Problem) *metadata.ErrorList {
	var all []metadata.Error
	if metaList != nil {
		all = append(all, metaList.Errors...)
	}
	all = append(all, ErrorList(problems)...)
	if len(all) == 0 {
		return nil
	}
	return &metadata.ErrorList{Errors: all}
}

// ErrorList converts problems to the entries of the 422 list.
func ErrorList(problems []Problem) []metadata.Error {
	out := make([]metadata.Error, len(problems))
	for i, p := range problems {
		out[i] = p.Error()
	}
	return out
}

// edit replaces body[start:end] with text.
type edit struct {
	start, end int
	text       string
}

func newEdit(body []byte, ref Ref, e *entry) edit {
	size, _ := ParseSize(ref.Title)
	w, h := Points(e.img.Width, e.img.Height, size)
	end := ref.End + attributeBlockLen(body[ref.End:])
	text := fmt.Sprintf("![%s](%s%s){width=%s height=%s}", escapeAlt(ref.Alt), bundleDir, e.name, formatPoints(w), formatPoints(h))
	return edit{start: ref.Start, end: end, text: text}
}

func applyEdits(body []byte, edits []edit) []byte {
	slices.SortFunc(edits, func(a, b edit) int { return a.start - b.start })
	var out bytes.Buffer
	pos := 0
	for _, e := range edits {
		out.Write(body[pos:e.start])
		out.WriteString(e.text)
		pos = e.end
	}
	out.Write(body[pos:])
	return out.Bytes()
}
