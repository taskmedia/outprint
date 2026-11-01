package outline

import (
	"errors"
	"net/netip"
	"net/url"
	"strings"
	"testing"
)

func TestBlockedAddr(t *testing.T) {
	blockedList := []string{"127.0.0.1", "10.1.2.3", "172.20.0.1", "192.168.0.1", "169.254.169.254", "100.64.0.1", "0.0.0.0",
		"224.0.0.1", "255.255.255.255", "::1", "::", "fe80::1", "fc00::1", "::ffff:127.0.0.1", "::ffff:10.0.0.1", "64:ff9b::1", "2002::1", "ff02::1"}
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
