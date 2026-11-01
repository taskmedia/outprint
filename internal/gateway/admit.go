package gateway

import (
	"context"
	"errors"
	"net/http"

	"github.com/taskmedia/outprint/internal/admission"
	"github.com/taskmedia/outprint/internal/metadata"
	"github.com/taskmedia/outprint/internal/outline"
)

// Codes of an admission rejection, in the error list.
const (
	codeBusy      = "busy"
	codeUserLimit = "user_limit"

	// sourceService marks a problem that is nobody's mistake: waiting helps.
	sourceService = "service"

	messageBusy      = "busy, retry later: the renderer is at capacity"
	messageUserLimit = "busy, retry later: you already have a render in progress"
)

// confirmedUser asks Outline who the session belongs to. The id comes from
// Outline's answer and from nothing the client sent besides the cookie, which
// Outline itself validates. If it cannot be determined the request fails; it
// is never keyed by address or by the cookie.
func (g *Gateway) confirmedUser(ctx context.Context, s outline.Session) (string, *failure) {
	id, err := g.cfg.Outline.UserID(ctx, s)
	if err != nil {
		return "", outlineFailure(err)
	}
	if id == "" {
		return "", fail(http.StatusInternalServerError, PhaseOutline, "unavailable", "the document could not be rendered", errors.New("empty user id"))
	}
	return id, nil
}

// enterUser counts the render against the user, before any image is fetched,
// so one user cannot start many parallel image fetches.
func (g *Gateway) enterUser(userID string) (func(), *failure) {
	leave, err := g.cfg.Admission.EnterUser(userID)
	if err != nil {
		return nil, admissionFailure(err)
	}
	return leave, nil
}

// acquireSlot waits for a Worker slot, right before the Worker call, so that
// requests that fail validation never hold one.
func (g *Gateway) acquireSlot(ctx context.Context) (func(), *failure) {
	release, err := g.cfg.Admission.AcquireSlot(ctx)
	if err != nil {
		return nil, admissionFailure(err)
	}
	return release, nil
}

// admissionFailure maps a refusal to 429 with Retry-After and the error list;
// a caller that gave up gets a plain 503 that nobody reads.
func admissionFailure(err error) *failure {
	var rej *admission.Rejected
	if !errors.As(err, &rej) {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return fail(http.StatusServiceUnavailable, PhaseAdmission, "canceled", "the request was canceled", err)
		}
		return fail(http.StatusInternalServerError, PhaseAdmission, "internal", "the document could not be rendered", err)
	}
	code, text := codeBusy, messageBusy
	if errors.Is(rej, admission.ErrUserLimit) {
		code, text = codeUserLimit, messageUserLimit
	}
	list := metadata.ErrorList{Errors: []metadata.Error{{Source: sourceService, Code: code, Message: text}}}
	return &failure{
		status: http.StatusTooManyRequests, body: list, phase: PhaseAdmission, kind: code,
		retryAfter: rej.RetryAfter, cause: err,
	}
}
