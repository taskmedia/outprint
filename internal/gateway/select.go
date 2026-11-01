package gateway

import (
	"context"

	"github.com/taskmedia/outprint/internal/binding"
	"github.com/taskmedia/outprint/internal/outline"
)

// selection is the Binding outcome of one Document, with the problems of a
// block that does not validate.
type selection struct {
	binding.Outcome
	invalid *failure
}

// resolveBinding reads the Collection overview with the user's cookie and
// resolves it. It reads on every call: a change of the overview takes
// effect on the next render, and nothing outlives the user's permission to
// read the Collection. Nothing but the Document's Collection selects the
// Binding; the request carries no override.
func (g *Gateway) resolveBinding(ctx context.Context, doc outline.Document, s outline.Session) (selection, *failure) {
	overview := ""
	if doc.CollectionID != "" {
		col, err := g.cfg.Outline.Collection(ctx, doc.CollectionID, s)
		if err != nil {
			return selection{}, collectionFailure(err)
		}
		overview = col.Description
	}
	out, errs := binding.Resolve(overview, g.cfg.Registry)
	sel := selection{Outcome: out}
	if errs != nil {
		sel.invalid = bindingFailure(errs)
	}
	return sel, nil
}

// selectBinding is resolveBinding for a render: a broken block or a missing
// Binding ends the request.
func (g *Gateway) selectBinding(ctx context.Context, doc outline.Document, s outline.Session) (selection, *failure) {
	sel, f := g.resolveBinding(ctx, doc, s)
	switch {
	case f != nil:
		return selection{}, f
	case sel.invalid != nil:
		return selection{}, sel.invalid
	case sel.Kind == binding.NoBinding:
		return selection{}, noBindingFailure()
	}
	return sel, nil
}
