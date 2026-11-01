package images

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/taskmedia/outprint/internal/outline"
)

const (
	testBase = "https://outline.example.invalid"
	testID   = "0b6f1a52-3c41-4d2e-9a77-5f0c2e8d1b34"
)

func TestOutlineSourceLocate(t *testing.T) {
	src, err := NewOutlineSource(testBase, nil, outline.Session{})
	if err != nil {
		t.Fatal(err)
	}
	wantKey := "/api/attachments.redirect?id=" + testID
	ok := []string{
		"/api/attachments.redirect?id=" + testID,
		testBase + "/api/attachments.redirect?id=" + testID,
		"https://OUTLINE.example.invalid:443/api/attachments.redirect?id=" + testID,
		"/api/attachments.redirect?id=" + testID + "&other=1",
		"  /api/attachments.redirect?id=" + testID + "  ",
		"/api/attachments.redirect?id=" + testID + "#frag",
	}
	for _, addr := range ok {
		if key, rej := src.Locate(addr); rej != nil || key != wantKey {
			t.Errorf("%q: key %q, rejection %v", addr, key, rej)
		}
	}
	files, rej := src.Locate("/api/files.get?key=uploads%2Fabc%2Fpic.png&sig=s")
	if rej != nil || files != "/api/files.get?key=uploads%2Fabc%2Fpic.png&sig=s" {
		t.Errorf("files.get: %q %v", files, rej)
	}

	kinds := map[string]string{
		"":                                  KindMissing,
		"/api/attachments.redirect":         KindMissing,
		"/api/attachments.redirect?id=":     KindMissing,
		"/api/attachments.redirect?id=../x": KindMissing,
		"/api/attachments.redirect?id=%00":  KindMissing,
		"/api/files.get":                    KindMissing,

		"https://cdn.example.invalid/a.png":                                                          KindForbidden,
		"http://outline.example.invalid/api/attachments.redirect?id=" + testID:                       KindForbidden,
		"https://outline.example.invalid:8443/api/attachments.redirect?id=" + testID:                 KindForbidden,
		"https://outline.example.invalid.evil.example.invalid/api/attachments.redirect?id=" + testID: KindForbidden,
		"https://user:pw@outline.example.invalid/api/attachments.redirect?id=" + testID:              KindForbidden,
		"https://outline.example.invalid@evil.example.invalid/api/attachments.redirect?id=" + testID: KindForbidden,
		"//evil.example.invalid/api/attachments.redirect?id=" + testID:                               KindForbidden,
		"///evil.example.invalid/x":           KindForbidden,
		"\\\\evil.example.invalid\\x":         KindForbidden,
		"/\\evil.example.invalid/x":           KindForbidden,
		"data:image/png;base64,AAAA":          KindForbidden,
		"file:///etc/passwd":                  KindForbidden,
		"javascript:alert(1)":                 KindForbidden,
		"ftp://outline.example.invalid/x.png": KindForbidden,
		"images/img-1.png":                    KindForbidden,
		"img-1.png":                           KindForbidden,
		"../images/img-1.png":                 KindForbidden,
		"/etc/passwd":                         KindForbidden,
		"/api/documents.info?id=" + testID:    KindForbidden,
		"/api/attachments.redirect/../documents.info?id=" + testID:   KindForbidden,
		"/api/attachments.redirectx?id=" + testID:                    KindForbidden,
		"https://[::1]/api/attachments.redirect?id=" + testID:        KindForbidden,
		"https://127.0.0.1/api/attachments.redirect?id=" + testID:    KindForbidden,
		"https://169.254.169.254/latest/meta-data/":                  KindForbidden,
		"/api/attachments.redirect?id=" + testID + "\n/x":            KindForbidden,
		"%2F%2Fevil.example.invalid/x":                               KindForbidden,
		"https://outline.example.invalid/%2e%2e/api/files.get?key=x": KindForbidden,
	}
	for addr, want := range kinds {
		key, rej := src.Locate(addr)
		if rej == nil || rej.Kind != want {
			t.Errorf("%q: key %q, rejection %v, want %s", addr, key, rej, want)
		}
	}
}

func TestOutlineSourceWithPathPrefix(t *testing.T) {
	src, err := NewOutlineSource("http://wiki.example.invalid:3000/team/", nil, outline.Session{})
	if err != nil {
		t.Fatal(err)
	}
	key, rej := src.Locate("/team/api/attachments.redirect?id=" + testID)
	if rej != nil || key != "/team/api/attachments.redirect?id="+testID {
		t.Errorf("key %q, %v", key, rej)
	}
	if _, rej := src.Locate("/api/attachments.redirect?id=" + testID); rej == nil {
		t.Error("a path outside the prefix must be rejected")
	}
	if _, rej := src.Locate("http://wiki.example.invalid:3000/team/api/attachments.redirect?id=" + testID); rej != nil {
		t.Error(rej)
	}
}

func TestNewOutlineSourceRejectsBadURL(t *testing.T) {
	for _, u := range []string{"", "ftp://x.example.invalid", "https://", "::"} {
		if _, err := NewOutlineSource(u, nil, outline.Session{}); err == nil {
			t.Errorf("%q accepted", u)
		}
	}
}

type stubFetcher struct {
	err    error
	target string
	cookie string
}

func (s *stubFetcher) Image(_ context.Context, target string, sess outline.Session) ([]byte, error) {
	s.target, s.cookie = target, sess.Cookie
	return []byte("data"), s.err
}

func TestOutlineSourceFetchMapsErrors(t *testing.T) {
	cases := map[error]error{
		outline.ErrNotFound:  ErrNotFound,
		outline.ErrForbidden: ErrForbidden,
		outline.ErrTooLarge:  ErrTooLarge,
	}
	for in, want := range cases {
		src, _ := NewOutlineSource(testBase, &stubFetcher{err: in}, outline.Session{})
		if _, err := src.Fetch(context.Background(), "/api/x"); !errors.Is(err, want) {
			t.Errorf("%v mapped to %v, want %v", in, err, want)
		}
	}
	f := &stubFetcher{}
	src, _ := NewOutlineSource(testBase, f, outline.Session{Cookie: "c=1"})
	data, err := src.Fetch(context.Background(), "/api/attachments.redirect?id="+testID)
	if err != nil || string(data) != "data" || f.cookie != "c=1" || f.target != "/api/attachments.redirect?id="+testID {
		t.Errorf("fetch: %q %v %+v", data, err, f)
	}
	src, _ = NewOutlineSource(testBase, &stubFetcher{err: outline.ErrUnavailable}, outline.Session{})
	if _, err := src.Fetch(context.Background(), "/api/x"); !errors.Is(err, outline.ErrUnavailable) {
		t.Errorf("unavailable passed through as %v", err)
	}
}

func TestDirSource(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a b.png"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "secret.png"), []byte("s"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "secret.png"), filepath.Join(dir, "link.png")); err != nil {
		t.Fatal(err)
	}
	src := DirSource{Root: dir}
	key, rej := src.Locate("a%20b.png")
	if rej != nil {
		t.Fatal(rej)
	}
	data, err := src.Fetch(context.Background(), key)
	if err != nil || string(data) != "x" {
		t.Errorf("fetch: %q %v", data, err)
	}
	for _, addr := range []string{"../secret.png", "/etc/passwd", "https://x.example.invalid/a.png", "file:///etc/passwd",
		"sub/../../x.png", "..", ".", "a\\b.png", "data:image/png;base64,AA", "%2e%2e/x.png"} {
		if _, rej := src.Locate(addr); rej == nil || rej.Kind != KindForbidden {
			t.Errorf("%q: %v", addr, rej)
		}
	}
	if _, rej := src.Locate(""); rej == nil || rej.Kind != KindMissing {
		t.Errorf("empty: %v", rej)
	}
	if _, err := src.Fetch(context.Background(), "nope.png"); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing file: %v", err)
	}
	if _, err := src.Fetch(context.Background(), "link.png"); !errors.Is(err, ErrForbidden) {
		t.Errorf("symlink out of the root: %v", err)
	}
}
