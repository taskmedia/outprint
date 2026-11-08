package outline

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const sessionCookie = "accessToken=canary-session"

func newClient(t *testing.T, h http.Handler) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c, err := NewClient(srv.URL, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestDocumentSendsCookieAndDecodes(t *testing.T) {
	var gotPath, gotCookie, gotID, gotType string
	c := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotCookie, gotType = r.URL.Path, r.Header.Get("Cookie"), r.Header.Get("Content-Type")
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotID = body["id"]
		io.WriteString(w, `{"data":{"id":"d1","title":"Hello","text":"body","updatedAt":"2026-03-02T23:30:00.000Z","collectionId":"c1"}}`)
	}))
	doc, err := c.Document(context.Background(), "slug-abc", Session{Cookie: sessionCookie})
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/api/documents.info" || gotCookie != sessionCookie || gotID != "slug-abc" || gotType != "application/json" {
		t.Errorf("request = %s cookie=%q id=%q type=%q", gotPath, gotCookie, gotID, gotType)
	}
	if doc.ID != "d1" || doc.Title != "Hello" || doc.Text != "body" || doc.CollectionID != "c1" ||
		doc.UpdatedAt.UTC().Format(time.RFC3339) != "2026-03-02T23:30:00Z" {
		t.Errorf("document = %+v", doc)
	}
}

func TestCollectionAndUserID(t *testing.T) {
	c := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/collections.info":
			io.WriteString(w, `{"data":{"id":"c1","name":"Letters","description":"hello"}}`)
		case "/api/auth.info":
			io.WriteString(w, `{"data":{"user":{"id":"u-42"},"team":{}}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	col, err := c.Collection(context.Background(), "c1", Session{Cookie: sessionCookie})
	if err != nil || col.Name != "Letters" || col.Description != "hello" {
		t.Errorf("Collection = %+v, %v", col, err)
	}
	id, err := c.UserID(context.Background(), Session{Cookie: sessionCookie})
	if err != nil || id != "u-42" {
		t.Errorf("UserID = %q, %v", id, err)
	}
}

func TestUserIDMissing(t *testing.T) {
	c := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, `{"data":{"user":{}}}`)
	}))
	if _, err := c.UserID(context.Background(), Session{}); !errors.Is(err, ErrUnavailable) {
		t.Errorf("err = %v, want ErrUnavailable", err)
	}
}

func TestStatusMapping(t *testing.T) {
	tests := []struct {
		status int
		want   error
	}{
		{401, ErrUnauthorized}, {403, ErrForbidden}, {404, ErrNotFound},
		{500, ErrUnavailable}, {400, ErrUnavailable}, {302, ErrUnavailable},
	}
	for _, tt := range tests {
		c := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Location", "http://other.example.invalid/")
			w.WriteHeader(tt.status)
			io.WriteString(w, `{"message":"outline words that must not travel"}`)
		}))
		_, err := c.Document(context.Background(), "x", Session{})
		if !errors.Is(err, tt.want) {
			t.Errorf("status %d: err = %v, want %v", tt.status, err, tt.want)
		}
		if err != nil && strings.Contains(err.Error(), "must not travel") {
			t.Errorf("status %d: error carries response text: %v", tt.status, err)
		}
	}
}

func TestRedirectIsNotFollowedAndCookieStays(t *testing.T) {
	var otherHits atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		otherHits.Add(1)
	}))
	defer other.Close()
	c := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/steal", http.StatusTemporaryRedirect)
	}))
	_, err := c.Document(context.Background(), "x", Session{Cookie: sessionCookie})
	if !errors.Is(err, ErrUnavailable) {
		t.Errorf("err = %v, want ErrUnavailable", err)
	}
	if otherHits.Load() != 0 {
		t.Error("the client followed a redirect to another origin")
	}
}

func TestResponseTooLarge(t *testing.T) {
	c := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, `{"data":{"text":"`+strings.Repeat("a", maxResponseBytes)+`"}}`)
	}))
	if _, err := c.Document(context.Background(), "x", Session{}); !errors.Is(err, ErrTooLarge) {
		t.Errorf("err = %v, want ErrTooLarge", err)
	}
}

func TestMalformedResponse(t *testing.T) {
	for _, body := range []string{`nope`, `{}`, `{"data":"str"}`} {
		c := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, body) }))
		if _, err := c.Document(context.Background(), "x", Session{}); !errors.Is(err, ErrUnavailable) {
			t.Errorf("body %q: err = %v, want ErrUnavailable", body, err)
		}
	}
}

func TestTimeoutHidesURL(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { <-release }))
	defer srv.Close()
	defer close(release)
	c, _ := NewClient(srv.URL, 50*time.Millisecond)
	_, err := c.Document(context.Background(), "x", Session{})
	if !errors.Is(err, ErrUnavailable) || strings.Contains(err.Error(), srv.URL) {
		t.Errorf("err = %v", err)
	}
}

func TestNewClientValidatesURL(t *testing.T) {
	for _, bad := range []string{"", "ftp://x.example.test", "https://", "https://u:p@x.example.test", "https://x.example.test?a=b", "no-scheme"} {
		if _, err := NewClient(bad, 0); err == nil {
			t.Errorf("NewClient(%q) succeeded", bad)
		}
	}
	c, err := NewClient("https://wiki.example.test/prefix/", 0)
	if err != nil || c.Origin() != "https://wiki.example.test" {
		t.Errorf("NewClient = %v, %v", c, err)
	}
}

func TestPathPrefixIsKept(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		io.WriteString(w, `{"data":{}}`)
	}))
	defer srv.Close()
	c, _ := NewClient(srv.URL+"/wiki/", 0)
	_, _ = c.Document(context.Background(), "x", Session{})
	if gotPath != "/wiki/api/documents.info" {
		t.Errorf("path = %q", gotPath)
	}
}

func TestUserLanguage(t *testing.T) {
	c := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"data":{"user":{"id":"u-42","language":"de_DE"}}}`)
	}))
	lang, err := c.UserLanguage(context.Background(), Session{Cookie: sessionCookie})
	if err != nil || lang != "de_DE" {
		t.Errorf("UserLanguage = %q, %v", lang, err)
	}
}

func TestUserLanguageMayBeAbsent(t *testing.T) {
	c := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, `{"data":{"user":{"id":"u-42"}}}`)
	}))
	lang, err := c.UserLanguage(context.Background(), Session{Cookie: sessionCookie})
	if err != nil || lang != "" {
		t.Errorf("UserLanguage = %q, %v; want empty and no error", lang, err)
	}
}

func TestForwardedProtoIsDerivedFromTheTwoURLs(t *testing.T) {
	for _, tc := range []struct{ internal, public, want string }{
		{"http://outline:3000", "https://docs.example.invalid", "https"},
		{"http://outline:3000", "http://docs.example.invalid", ""},
		{"https://docs.example.invalid", "https://docs.example.invalid", ""},
		{"http://outline:3000", "", ""},
		{"", "https://docs.example.invalid", ""},
	} {
		if got := ForwardedProto(tc.internal, tc.public); got != tc.want {
			t.Errorf("ForwardedProto(%q, %q) = %q, want %q", tc.internal, tc.public, got, tc.want)
		}
	}
}

func TestForwardedProtoIsSentWithEveryAPICall(t *testing.T) {
	var got []string
	c := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = append(got, r.Header.Get("X-Forwarded-Proto"))
		io.WriteString(w, `{"data":{"id":"d1"}}`)
	}))
	if _, err := c.Document(t.Context(), "a", Session{}); err != nil {
		t.Fatal(err)
	}
	c, err := c.WithForwardedProto("https")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Document(t.Context(), "a", Session{}); err != nil {
		t.Fatal(err)
	}
	if want := []string{"", "https"}; strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("X-Forwarded-Proto = %q, want %q", got, want)
	}
}

func TestWithForwardedProtoRejectsOtherValues(t *testing.T) {
	c := newClient(t, http.NotFoundHandler())
	if _, err := c.WithForwardedProto("javascript"); err == nil {
		t.Error("want an error for a value that is not http or https")
	}
}
