package gateway

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/taskmedia/outline-latex-pdf/internal/bundle"
	"github.com/taskmedia/outline-latex-pdf/internal/metadata"
	"github.com/taskmedia/outline-latex-pdf/internal/outline"
	"github.com/taskmedia/outline-latex-pdf/internal/workerapi"
)

// Phases of the Gateway's own work, as they appear in logs and in a 500
// body. A failure inside the Worker keeps the Worker's phase.
const (
	PhaseRequest  = "request"
	PhaseOutline  = "outline"
	PhaseBinding  = "binding"
	PhaseMetadata = "metadata"
	PhaseBundle   = "bundle"
	PhaseWorker   = "worker"
)

const (
	// retryAfterSeconds is what a busy Worker tells the client to wait.
	retryAfterSeconds = 5

	codeNoBinding = "no_binding"
)

// message is the body of every error that is not a validation list. It never
// holds Document content, tool output or a cause: only a fixed sentence, the
// phase and the job id.
type message struct {
	Message string `json:"message"`
	Phase   string `json:"phase,omitempty"`
	JobID   string `json:"job_id,omitempty"`
}

// failure is a request that ends in an error response.
type failure struct {
	status     int
	body       any
	phase      string
	kind       string
	jobID      string
	retryAfter int
	cause      error
}

func fail(status int, phase, kind, text string, cause error) *failure {
	return &failure{status: status, phase: phase, kind: kind, cause: cause, body: message{Message: text, Phase: phase}}
}

func (f *failure) write(w http.ResponseWriter) {
	w.Header().Set("Content-Type", workerapi.ContentTypeJSON)
	w.Header().Set("Cache-Control", "no-store")
	if f.retryAfter > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(f.retryAfter))
	}
	w.WriteHeader(f.status)
	_ = json.NewEncoder(w).Encode(f.body)
}

func outlineFailure(err error) *failure {
	switch {
	case errors.Is(err, outline.ErrUnauthorized):
		return fail(http.StatusUnauthorized, PhaseOutline, "unauthorized", "sign in to Outline to download this document", err)
	case errors.Is(err, outline.ErrForbidden):
		return fail(http.StatusForbidden, PhaseOutline, "forbidden", "you are not allowed to read this document", err)
	case errors.Is(err, outline.ErrNotFound):
		return fail(http.StatusNotFound, PhaseOutline, "not_found", "document not found", err)
	case errors.Is(err, outline.ErrTooLarge):
		return fail(http.StatusRequestEntityTooLarge, PhaseOutline, "too_large", "the document is too large to render", err)
	}
	return fail(http.StatusInternalServerError, PhaseOutline, "unavailable", "the document could not be rendered", err)
}

// collectionFailure maps a failed read of the Collection overview. There is
// no fallback: without the overview the Binding is unknown.
func collectionFailure(err error) *failure {
	switch {
	case errors.Is(err, outline.ErrUnauthorized):
		return fail(http.StatusUnauthorized, PhaseBinding, "unauthorized", "sign in to Outline to download this document", err)
	case errors.Is(err, outline.ErrForbidden):
		return fail(http.StatusForbidden, PhaseBinding, "collection_forbidden",
			"you are not allowed to read the collection that holds this document's binding", err)
	case errors.Is(err, outline.ErrNotFound):
		return fail(http.StatusNotFound, PhaseBinding, "collection_not_found",
			"the collection that holds this document's binding was not found", err)
	case errors.Is(err, outline.ErrTooLarge):
		return fail(http.StatusRequestEntityTooLarge, PhaseBinding, "too_large", "the collection overview is too large to read", err)
	}
	return fail(http.StatusInternalServerError, PhaseBinding, "unavailable", "the document could not be rendered", err)
}

func bindingFailure(list *metadata.ErrorList) *failure {
	return &failure{status: http.StatusUnprocessableEntity, body: list, phase: PhaseBinding, kind: "invalid_binding", cause: list}
}

func noBindingFailure() *failure {
	list := metadata.ErrorList{Errors: []metadata.Error{{
		Source:  metadata.SourceCollection,
		Code:    codeNoBinding,
		Message: "no binding: this installation has no default Binding and the Document's Collection has none",
	}}}
	return &failure{status: http.StatusUnprocessableEntity, body: list, phase: PhaseBinding, kind: codeNoBinding}
}

func metadataFailure(err error) *failure {
	var list *metadata.ErrorList
	if errors.As(err, &list) {
		return &failure{status: http.StatusUnprocessableEntity, body: list, phase: PhaseMetadata, kind: "invalid_metadata", cause: err}
	}
	return fail(http.StatusInternalServerError, PhaseMetadata, "internal", "the document could not be rendered", err)
}

func bundleFailure(err error) *failure {
	var limit *bundle.LimitError
	if errors.As(err, &limit) {
		return fail(http.StatusRequestEntityTooLarge, PhaseBundle, "too_large", "the document is too large to render", err)
	}
	return fail(http.StatusInternalServerError, PhaseBundle, "invalid", "the document could not be rendered", err)
}

func workerFailure(err error) *failure {
	var we *workerapi.Error
	if !errors.As(err, &we) {
		return fail(http.StatusInternalServerError, PhaseWorker, "unreachable", "the document could not be rendered", err)
	}
	var f *failure
	switch {
	case we.Busy():
		f = fail(http.StatusTooManyRequests, we.Phase, we.Kind, "the renderer is busy, retry later", err)
		f.retryAfter = retryAfterSeconds
	case we.Kind == workerapi.KindTimeout:
		f = fail(http.StatusGatewayTimeout, we.Phase, we.Kind, "rendering took too long; shorten the document", err)
	case we.Kind == workerapi.KindTooLarge:
		f = fail(http.StatusRequestEntityTooLarge, we.Phase, we.Kind, "the document is too large to render", err)
	default:
		f = fail(http.StatusInternalServerError, we.Phase, we.Kind, "the document could not be rendered", err)
	}
	f.jobID = we.JobID
	f.body = message{Message: f.body.(message).Message, Phase: we.Phase, JobID: we.JobID}
	return f
}
