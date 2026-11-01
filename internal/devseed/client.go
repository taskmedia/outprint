package devseed

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const maxAPIResponse = 8 << 20

// Client calls the Outline API as the signed-in user.
type Client struct {
	base  string
	token string
	http  *http.Client
}

// NewClient returns a client for the Outline at base (the injector origin).
func NewClient(base, token string) *Client {
	return &Client{base: strings.TrimRight(base, "/"), token: token, http: &http.Client{Timeout: 60 * time.Second}}
}

// APIError is an error answer of the Outline API.
type APIError struct {
	Method  string
	Status  int
	Message string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("outline %s: status %d: %s", e.Method, e.Status, e.Message)
}

// Call posts params as JSON to /api/<method> and decodes the "data" field of
// the answer into out (which may be nil).
func (c *Client) Call(ctx context.Context, method string, params, out any) error {
	body, err := json.Marshal(params)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/api/"+method, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("outline %s: %w", method, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxAPIResponse))
	if err != nil {
		return fmt.Errorf("outline %s: read: %w", method, err)
	}
	var envelope struct {
		Data    json.RawMessage `json:"data"`
		Message string          `json:"message"`
	}
	_ = json.Unmarshal(data, &envelope)
	if resp.StatusCode != http.StatusOK {
		return &APIError{Method: method, Status: resp.StatusCode, Message: envelope.Message}
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(envelope.Data, out); err != nil {
		return fmt.Errorf("outline %s: unexpected answer: %w", method, err)
	}
	return nil
}

// Attachment is an uploaded file; URL is what a document's Markdown refers to.
type Attachment struct {
	ID  string `json:"id"`
	URL string `json:"url"`
}

// Upload stores a file as an attachment of a document through the two-step
// flow of the browser: attachments.create, then a multipart post to the
// upload address it names.
func (c *Client) Upload(ctx context.Context, documentID, name, contentType string, content []byte) (Attachment, error) {
	var created struct {
		UploadURL  string         `json:"uploadUrl"`
		Form       map[string]any `json:"form"`
		Attachment Attachment     `json:"attachment"`
	}
	err := c.Call(ctx, "attachments.create", map[string]any{
		"documentId": documentID, "name": name, "contentType": contentType, "size": len(content),
	}, &created)
	if err != nil {
		return Attachment{}, err
	}
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for k, v := range created.Form {
		if err := w.WriteField(k, fmt.Sprint(v)); err != nil {
			return Attachment{}, err
		}
	}
	// The file must come after the fields.
	part, err := w.CreateFormFile("file", name)
	if err != nil {
		return Attachment{}, err
	}
	if _, err := part.Write(content); err != nil {
		return Attachment{}, err
	}
	if err := w.Close(); err != nil {
		return Attachment{}, err
	}
	target := created.UploadURL
	if strings.HasPrefix(target, "/") {
		target = c.base + target
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, &buf)
	if err != nil {
		return Attachment{}, err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+c.token)
	resp, err := c.http.Do(req)
	if err != nil {
		return Attachment{}, fmt.Errorf("upload %s: %w", name, err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxAPIResponse))
	if resp.StatusCode != http.StatusOK {
		return Attachment{}, fmt.Errorf("upload %s: status %s", name, strconv.Itoa(resp.StatusCode))
	}
	return created.Attachment, nil
}
