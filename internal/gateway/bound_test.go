package gateway

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestImageSlotsBoundTheImagePhaseAcrossUsers(t *testing.T) {
	g := New(Config{ImageConcurrency: 1})
	release, f := g.acquireImageSlot(context.Background())
	if f != nil {
		t.Fatal(f)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, f := g.acquireImageSlot(ctx); f == nil || f.status != http.StatusGatewayTimeout {
		t.Fatalf("second acquire should time out with 504, got %+v", f)
	}
	release()
	if _, f := g.acquireImageSlot(context.Background()); f != nil {
		t.Fatalf("slot not released: %+v", f)
	}
}

func TestBuildDeadlineEndsTheImagePhase(t *testing.T) {
	e := newEnvConfigured(t, true, nil, nil, func(c *Config) { c.BuildTimeout = time.Nanosecond })
	e.outline.add("d", 200, "T", "![x](/api/attachments.redirect?id=1)\n")
	resp, body := e.get(t, "d", cookieCanary)
	if resp.StatusCode != http.StatusGatewayTimeout {
		t.Fatalf("status %d, want 504: %s", resp.StatusCode, body)
	}
}

func TestStatusReusesLanguageAndRefusedSessions(t *testing.T) {
	e := newEnv(t, true)
	e.outline.add("d", 200, "T", "x\n")
	e.outline.language = "de_DE"
	for i := 0; i < 3; i++ {
		if resp, body := e.getPath(t, "/status/d", cookieCanary); resp.StatusCode != 200 {
			t.Fatalf("status %d: %s", resp.StatusCode, body)
		}
	}
	if n := e.outline.calls("/api/auth.info"); n != 1 {
		t.Errorf("auth.info called %d times, want 1", n)
	}

	e.outline.add("d", 401, "T", "x\n")
	for i := 0; i < 3; i++ {
		if resp, _ := e.getPath(t, "/status/d", "accessToken=other"); resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("status %d, want 401", resp.StatusCode)
		}
	}
	if n := e.outline.calls("/api/documents.info"); n != 4 {
		t.Errorf("documents.info called %d times, want 4 (3 valid + 1 refused)", n)
	}
}
