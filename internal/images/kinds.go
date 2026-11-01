package images

import "fmt"

// Kinds of an image problem. They are the "code" of an error in the 422
// list, next to the codes of the Metadata Block.
const (
	KindMissing   = "missing"
	KindForbidden = "forbidden"
	KindTooLarge  = "too_large"
	KindFormat    = "format"
	KindDecode    = "decode"
	KindPixels    = "pixels"
	// KindSVGRejected is an SVG that the pre-check refuses, for its content
	// or its structure. An SVG over the size limit is too_large instead.
	KindSVGRejected = "svg_rejected"
	// KindSyntax is an image whose Markdown the rewrite cannot place
	// reliably, for example one that spans lines.
	KindSyntax = "syntax"
)

// Rejection is one image problem: a kind and a sentence for the author.
type Rejection struct {
	Kind    string
	Message string
}

func (r *Rejection) Error() string { return r.Kind + ": " + r.Message }

func reject(kind, format string, args ...any) *Rejection {
	return &Rejection{Kind: kind, Message: fmt.Sprintf(format, args...)}
}
