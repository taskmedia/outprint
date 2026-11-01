package workerapi

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/taskmedia/outprint/internal/bundle"
)

func sample() bundle.Bundle {
	return bundle.Bundle{Markdown: []byte("x"), Template: "neutral", DocumentType: "letter", Configuration: "example"}
}

func clientFor(t *testing.T, h http.HandlerFunc, token string) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c, err := NewClient(srv.URL, token, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestRenderSendsBearerAndBundle(t *testing.T) {
	var auth, ctype, path string
	var body []byte
	c := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		auth, ctype, path = r.Header.Get("Authorization"), r.Header.Get("Content-Type"), r.URL.Path
		body, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", ContentTypePDF)
		io.WriteString(w, "%PDF")
	}, "tok")
	pdf, err := c.Render(context.Background(), sample())
	if err != nil || string(pdf) != "%PDF" {
		t.Fatalf("Render = %q, %v", pdf, err)
	}
	if auth != "Bearer tok" || ctype != ContentTypeJSON || path != RenderPath || !strings.Contains(string(body), `"template":"neutral"`) {
		t.Errorf("request: auth=%q type=%q path=%q body=%s", auth, ctype, path, body)
	}
}

func TestRenderWithoutTokenSendsNoAuthorization(t *testing.T) {
	var auth string
	c := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", ContentTypePDF)
	}, "")
	if _, err := c.Render(context.Background(), sample()); err != nil || auth != "" {
		t.Errorf("err %v, auth %q", err, auth)
	}
}

func TestRenderErrors(t *testing.T) {
	tests := map[string]struct {
		handler http.HandlerFunc
		busy    bool
		status  int
	}{
		"busy": {func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(503)
			io.WriteString(w, `{"phase":"queue","kind":"busy"}`)
		}, true, 503},
		"timeout": {func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(504)
			io.WriteString(w, `{"phase":"timeout","kind":"timeout","job_id":"j"}`)
		}, false, 504},
	}
	for name, tt := range tests {
		err := func() error {
			_, err := clientFor(t, tt.handler, "").Render(context.Background(), sample())
			return err
		}()
		var we *Error
		if !errors.As(err, &we) || we.Busy() != tt.busy || we.Status != tt.status {
			t.Errorf("%s: err = %v", name, err)
		}
	}
}

func TestRenderUnreachableCases(t *testing.T) {
	cases := map[string]http.HandlerFunc{
		"not a pdf":  func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, "html") },
		"junk error": func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(502); io.WriteString(w, "<html>") },
		"empty kind": func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(500); io.WriteString(w, "{}") },
		"redirect": func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "http://other.example.invalid/", 307)
		},
	}
	for name, h := range cases {
		if _, err := clientFor(t, h, "").Render(context.Background(), sample()); !errors.Is(err, ErrUnreachable) {
			t.Errorf("%s: err = %v, want ErrUnreachable", name, err)
		}
	}
}

func TestRenderPDFTooLarge(t *testing.T) {
	c := clientFor(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", ContentTypePDF)
		w.Write(make([]byte, MaxPDFBytes+1))
	}, "")
	var we *Error
	if _, err := c.Render(context.Background(), sample()); !errors.As(err, &we) || we.Status != 413 {
		t.Errorf("err = %v", err)
	}
}

func TestNewClientRejectsBadURL(t *testing.T) {
	for _, bad := range []string{"", "worker:8081", "ftp://w.example.test", "http://"} {
		if _, err := NewClient(bad, "", time.Second); err == nil {
			t.Errorf("NewClient(%q) succeeded", bad)
		}
	}
}

func TestTokenMatches(t *testing.T) {
	tests := []struct {
		header string
		want   bool
	}{
		{"Bearer s3cret", true}, {"Bearer s3cre", false}, {"Bearer s3cret2", false},
		{"s3cret", false}, {"", false}, {"bearer s3cret", false},
	}
	for _, tt := range tests {
		if got := TokenMatches(tt.header, "s3cret"); got != tt.want {
			t.Errorf("TokenMatches(%q) = %v", tt.header, got)
		}
	}
}
