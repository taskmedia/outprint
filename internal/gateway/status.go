package gateway

import (
	"encoding/json"
	"net/http"

	"github.com/taskmedia/outprint/internal/binding"
	"github.com/taskmedia/outprint/internal/button"
	"github.com/taskmedia/outprint/internal/outline"
	"github.com/taskmedia/outprint/internal/workerapi"
)

// Reasons of the status endpoint.
const (
	reasonCollectionBinding = "collection_binding"
	reasonInvalidBinding    = "collection_binding_invalid"
	reasonDefaultBinding    = "default_binding"
	reasonNoBinding         = "no_binding"
)

type statusResponse struct {
	Available bool   `json:"available"`
	Reason    string `json:"reason"`
	// Language is the user's Outline language (for example "de_DE"), empty
	// when unknown. The script picks its labels from it.
	Language string `json:"language,omitempty"`
	// Button holds the installation's non-default button options.
	Button *button.Options `json:"button,omitempty"`
}

// status answers whether the button belongs on this Document: the
// Collection has a Binding block, or the installation has a default Binding.
// A block that does not validate counts as available so that the author
// sees the error on click. It is for button visibility only and renders
// nothing.
func (g *Gateway) status(w http.ResponseWriter, r *http.Request) {
	resp, f := g.statusOf(r)
	if f != nil {
		f.write(w)
		return
	}
	w.Header().Set("Content-Type", workerapi.ContentTypeJSON)
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(resp)
}

func (g *Gateway) statusOf(r *http.Request) (statusResponse, *failure) {
	docID := r.PathValue("documentId")
	if !documentIDRE.MatchString(docID) {
		return statusResponse{}, fail(http.StatusBadRequest, PhaseRequest, "bad_document_id", "invalid document id", nil)
	}
	session := outline.Session{Cookie: r.Header.Get("Cookie")}
	if session.Cookie == "" {
		return statusResponse{}, outlineFailure(outline.ErrUnauthorized)
	}
	doc, err := g.cfg.Outline.Document(r.Context(), docID, session)
	if err != nil {
		return statusResponse{}, outlineFailure(err)
	}
	sel, f := g.resolveBinding(r.Context(), doc, session)
	if f != nil {
		return statusResponse{}, f
	}
	resp := statusFor(sel)
	resp.Language = g.userLanguage(r, session)
	if !g.cfg.Button.IsZero() {
		resp.Button = &g.cfg.Button
	}
	return resp, nil
}

// userLanguage is best effort: a label in the wrong language is better than
// no button, so a failed lookup yields an empty language.
func (g *Gateway) userLanguage(r *http.Request, session outline.Session) string {
	lang, err := g.cfg.Outline.UserLanguage(r.Context(), session)
	if err != nil {
		g.cfg.Logger.Debug("user language not available", "err", err)
		return ""
	}
	return lang
}

func statusFor(sel selection) statusResponse {
	switch {
	case sel.invalid != nil:
		return statusResponse{Available: true, Reason: reasonInvalidBinding}
	case sel.Kind == binding.FromBlock:
		return statusResponse{Available: true, Reason: reasonCollectionBinding}
	case sel.Kind == binding.UseDefault:
		return statusResponse{Available: true, Reason: reasonDefaultBinding}
	}
	return statusResponse{Available: false, Reason: reasonNoBinding}
}
