// Package outline is a minimal client for the Outline API. It acts as the
// end user by forwarding the user's session cookie, so Outline's own
// permissions decide what is visible. The cookie is sent only to the one
// configured Outline origin and a redirect is never followed.
package outline

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	// DefaultTimeout bounds one API call.
	DefaultTimeout = 15 * time.Second
	// maxResponseBytes caps an API response. The Markdown limit is far
	// below it; the margin covers the JSON envelope and escaping.
	maxResponseBytes = 8 << 20
)

// Errors a caller maps to a response. They carry no Outline response text.
var (
	ErrUnauthorized = errors.New("outline: not signed in")
	ErrForbidden    = errors.New("outline: access denied")
	ErrNotFound     = errors.New("outline: not found")
	ErrTooLarge     = errors.New("outline: response too large")
	// ErrUnavailable covers a transport failure, a redirect and any status
	// not mapped above.
	ErrUnavailable = errors.New("outline: unavailable")
)

// Session is what the end user's request brings along. Cookie is the raw
// Cookie header value.
type Session struct {
	Cookie string
}

// Document is the part of an Outline Document the renderer uses.
type Document struct {
	ID           string    `json:"id"`
	Title        string    `json:"title"`
	Text         string    `json:"text"`
	UpdatedAt    time.Time `json:"updatedAt"`
	CollectionID string    `json:"collectionId"`
}

// Collection is the part of an Outline Collection the renderer uses.
// Description is Markdown and holds the Binding block.
type Collection struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Client talks to one Outline installation.
type Client struct {
	base   *url.URL
	http   *http.Client
	images *imageFetch
}

// NewClient returns a client for the Outline at rawURL, which must be an
// http or https origin (a path prefix is allowed). timeout of zero means
// DefaultTimeout.
func NewClient(rawURL string, timeout time.Duration) (*Client, error) {
	base, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return nil, fmt.Errorf("outline url: %w", err)
	}
	if (base.Scheme != "http" && base.Scheme != "https") || base.Host == "" {
		return nil, errors.New("outline url must be an http or https URL with a host")
	}
	if base.User != nil || base.RawQuery != "" || base.Fragment != "" {
		return nil, errors.New("outline url must not contain credentials, a query or a fragment")
	}
	base.Path = strings.TrimRight(base.Path, "/")
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	return &Client{base: base, http: &http.Client{
		Timeout: timeout,
		// A redirect would carry the request, and with it the cookie,
		// somewhere the operator did not configure.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}, nil
}

// BaseURL is the configured URL without a trailing slash, with its path
// prefix if the Outline lives under one.
func (c *Client) BaseURL() string { return c.base.String() }

// Origin is the configured origin, for logs and for callers that must
// compare against it.
func (c *Client) Origin() string { return c.base.Scheme + "://" + c.base.Host }

// Document fetches a Document by id or url slug (documents.info).
func (c *Client) Document(ctx context.Context, id string, s Session) (Document, error) {
	var out Document
	err := c.call(ctx, "documents.info", map[string]string{"id": id}, s, &out)
	return out, err
}

// Collection fetches a Collection by id (collections.info).
func (c *Client) Collection(ctx context.Context, id string, s Session) (Collection, error) {
	var out Collection
	err := c.call(ctx, "collections.info", map[string]string{"id": id}, s, &out)
	return out, err
}

// UserID returns the id of the user Outline confirms for the session
// (auth.info). Admission control keys its per-user limit on it.
func (c *Client) UserID(ctx context.Context, s Session) (string, error) {
	var out struct {
		User struct {
			ID string `json:"id"`
		} `json:"user"`
	}
	if err := c.call(ctx, "auth.info", map[string]string{}, s, &out); err != nil {
		return "", err
	}
	if out.User.ID == "" {
		return "", fmt.Errorf("%w: auth.info has no user id", ErrUnavailable)
	}
	return out.User.ID, nil
}

func (c *Client) call(ctx context.Context, method string, params map[string]string, s Session, out any) error {
	body, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("encode %s: %w", method, err)
	}
	endpoint := c.base.JoinPath("api", method)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("%w: %s: %w", ErrUnavailable, method, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if s.Cookie != "" {
		req.Header.Set("Cookie", s.Cookie)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %s: %s", ErrUnavailable, method, errorKind(err))
	}
	defer resp.Body.Close()
	if err := statusError(method, resp.StatusCode); err != nil {
		return err
	}
	return decodeData(method, resp.Body, out)
}

// errorKind names a transport failure without the URL or any response text
// that net/http might include.
func errorKind(err error) string {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, context.Canceled):
		return "canceled"
	}
	return "request failed"
}

func statusError(method string, status int) error {
	switch status {
	case http.StatusOK:
		return nil
	case http.StatusUnauthorized:
		return ErrUnauthorized
	case http.StatusForbidden:
		return ErrForbidden
	case http.StatusNotFound:
		return ErrNotFound
	}
	return fmt.Errorf("%w: %s returned status %d", ErrUnavailable, method, status)
}

func decodeData(method string, body io.Reader, out any) error {
	data, err := io.ReadAll(io.LimitReader(body, maxResponseBytes+1))
	if err != nil {
		return fmt.Errorf("%w: %s: read response", ErrUnavailable, method)
	}
	if len(data) > maxResponseBytes {
		return ErrTooLarge
	}
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil || len(envelope.Data) == 0 {
		return fmt.Errorf("%w: %s: unexpected response", ErrUnavailable, method)
	}
	if err := json.Unmarshal(envelope.Data, out); err != nil {
		return fmt.Errorf("%w: %s: unexpected response", ErrUnavailable, method)
	}
	return nil
}
