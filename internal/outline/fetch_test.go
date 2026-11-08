package outline

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"testing"
)

func TestBlockedAddr(t *testing.T) {
	blockedList := []string{"127.0.0.1", "10.1.2.3", "172.20.0.1", "192.168.0.1", "169.254.169.254", "100.64.0.1", "0.0.0.0",
		"224.0.0.1", "255.255.255.255", "::1", "::", "fe80::1", "fc00::1", "::ffff:127.0.0.1", "::ffff:10.0.0.1", "64:ff9b::1", "2002::1", "ff02::1",
		"64:ff9b:1::1", "::7f00:1", "::a00:1", "fec0::1", "192.88.99.1", "2001:20::1"}
	for _, a := range blockedList {
		if !blockedAddr(netip.MustParseAddr(a)) {
			t.Errorf("%s not blocked", a)
		}
	}
	for _, a := range []string{"11.1.0.1", "93.184.216.34", "2606:4700::1", "::ffff:11.1.0.1"} {
		if blockedAddr(netip.MustParseAddr(a)) {
			t.Errorf("%s blocked", a)
		}
	}
	if !blockedAddr(netip.Addr{}) {
		t.Error("the zero address must be blocked")
	}
}

func TestConnectControlRechecksTheAddress(t *testing.T) {
	var blockedErr *BlockedError
	if err := connectControl(false)("tcp", "127.0.0.1:80", nil); !errors.As(err, &blockedErr) {
		t.Errorf("loopback at connect time: %v", err)
	}
	if err := connectControl(false)("tcp", "not-an-address", nil); err == nil {
		t.Error("garbage address accepted")
	}
	if err := connectControl(false)("tcp", "example.invalid:80", nil); err == nil {
		t.Error("a name at connect time must be refused")
	}
	if err := connectControl(false)("tcp", "11.1.0.1:80", nil); err != nil {
		t.Error(err)
	}
	if err := connectControl(true)("tcp", "127.0.0.1:3000", nil); err != nil {
		t.Errorf("the configured origin is exempt: %v", err)
	}
}

func TestAllowedHostPorts(t *testing.T) {
	origin, _ := url.Parse("http://outline.example.invalid:3000")
	set, err := allowedHostPorts(origin, []string{"Files.Example.Invalid", "store.example.invalid:9000"})
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"outline.example.invalid:3000", "files.example.invalid:80", "files.example.invalid:443", "store.example.invalid:9000"} {
		if !set[k] {
			t.Errorf("%s missing from %v", k, set)
		}
	}
	if set["outline.example.invalid:80"] || set["store.example.invalid:443"] {
		t.Error("ports must not widen")
	}
	for _, bad := range []string{"", "http://x", "x/y", "u@x", "x y"} {
		if _, err := allowedHostPorts(origin, []string{bad}); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestImageTargetMustStayUnderTheAPI(t *testing.T) {
	c, _ := NewClient("http://outline.example.invalid/team", 0)
	for _, target := range []string{"/api/x", "/team/other", "http://evil.example.invalid/team/api/x", "/team/api/x\r\nHost: evil", ""} {
		if _, err := c.Image(t.Context(), target, Session{}); !errors.Is(err, ErrUnavailable) {
			t.Errorf("%q: %v", target, err)
		}
	}
	if !strings.HasSuffix(c.BaseURL(), "/team") {
		t.Error(c.BaseURL())
	}
}

func TestRedirectToThePublicOriginIsFetchedFromTheInternalOne(t *testing.T) {
	var sawCookie []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawCookie = append(sawCookie, r.URL.Path+" cookie="+r.Header.Get("Cookie"))
		switch r.URL.Path {
		case "/api/attachments.redirect":
			http.Redirect(w, r, "http://wiki.example.invalid:8090/api/files.get?sig=1", http.StatusFound)
		case "/api/files.get":
			_, _ = w.Write([]byte("image-bytes"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c, err := NewClient(srv.URL, 0)
	if err != nil {
		t.Fatal(err)
	}
	c, err = c.WithImages(ImageOptions{PublicURL: "http://wiki.example.invalid:8090"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := c.Image(t.Context(), "/api/attachments.redirect?id=x", Session{Cookie: "accessToken=canary"})
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "image-bytes" {
		t.Errorf("got %q", got)
	}
	want := []string{"/api/attachments.redirect cookie=accessToken=canary", "/api/files.get cookie="}
	if strings.Join(sawCookie, "|") != strings.Join(want, "|") {
		t.Errorf("requests %v, want %v", sawCookie, want)
	}
}

func TestPublicOriginDoesNotWidenTheAllowList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://other.example.invalid:8090/api/files.get", http.StatusFound)
	}))
	defer srv.Close()
	c, _ := NewClient(srv.URL, 0)
	c, err := c.WithImages(ImageOptions{PublicURL: "http://wiki.example.invalid:8090"})
	if err != nil {
		t.Fatal(err)
	}
	var blockedErr *BlockedError
	if _, err := c.Image(t.Context(), "/api/attachments.redirect?id=x", Session{}); !errors.As(err, &blockedErr) {
		t.Errorf("a redirect to another host must stay blocked: %v", err)
	}
	if _, err := c.WithImages(ImageOptions{PublicURL: "ftp://x"}); err == nil {
		t.Error("a public URL that is not http(s) was accepted")
	}
}

func TestForwardedProtoGoesToTheOutlineOriginOnly(t *testing.T) {
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.URL.Path+" proto="+r.Header.Get("X-Forwarded-Proto"))
		switch r.URL.Path {
		case "/api/attachments.redirect":
			http.Redirect(w, r, "http://wiki.example.invalid:8090/api/files.get?sig=1", http.StatusFound)
		default:
			_, _ = w.Write([]byte("image-bytes"))
		}
	}))
	defer srv.Close()

	c, err := NewClient(srv.URL, 0)
	if err != nil {
		t.Fatal(err)
	}
	// Order must not matter: set before and after WithImages.
	c, err = c.WithImages(ImageOptions{PublicURL: "http://wiki.example.invalid:8090"})
	if err != nil {
		t.Fatal(err)
	}
	c, err = c.WithForwardedProto("https")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Image(t.Context(), "/api/attachments.redirect?id=x", Session{Cookie: "accessToken=canary"}); err != nil {
		t.Fatal(err)
	}
	want := []string{"/api/attachments.redirect proto=https", "/api/files.get proto=https"}
	if strings.Join(seen, "|") != strings.Join(want, "|") {
		t.Errorf("requests %v, want %v", seen, want)
	}
}
