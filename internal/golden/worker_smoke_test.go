//go:build golden

// Tier-2 smoke test of the two roles: the real `renderer worker` process
// renders a posted bundle, and an in-process Gateway with a fake Outline
// renders a Document through it. Like the other tier-2 tests it runs in the
// renderer image and never skips.
package golden

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/taskmedia/outprint/internal/bundle"
	"github.com/taskmedia/outprint/internal/gateway"
	"github.com/taskmedia/outprint/internal/outline"
	"github.com/taskmedia/outprint/internal/startup"
	"github.com/taskmedia/outprint/internal/workerapi"
)

const smokeMarkdown = "Thank you for your enquiry about the spring plant sale.\n"

func startWorker(t *testing.T) string {
	t.Helper()
	bin := os.Getenv("RENDERER_BIN")
	if bin == "" {
		bin = "renderer"
	}
	addr := freeAddr(t)
	cmd := exec.Command(bin, "worker")
	cmd.Env = append(os.Environ(), "RENDERER_LISTEN="+addr, "RENDERER_WORK_DIR="+t.TempDir())
	var logs bytes.Buffer
	cmd.Stderr = &logs
	if err := cmd.Start(); err != nil {
		t.Fatalf("start worker: %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		if t.Failed() {
			t.Logf("worker log:\n%s", logs.String())
		}
	})
	base := "http://" + addr
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if resp, err := http.Get(base + workerapi.HealthPath); err == nil {
			resp.Body.Close()
			return base
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("worker did not come up:\n%s", logs.String())
	return ""
}

func freeAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().String()
}

func writePDF(t *testing.T, pdf []byte) string {
	t.Helper()
	if !bytes.HasPrefix(pdf, []byte("%PDF")) {
		t.Fatalf("not a PDF: %.40q", pdf)
	}
	path := filepath.Join(t.TempDir(), "out.pdf")
	if err := os.WriteFile(path, pdf, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestWorkerRoleRendersABundle(t *testing.T) {
	base := startWorker(t)
	client, err := workerapi.NewClient(base, "", 90*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	pdf, err := client.Render(context.Background(), bundle.Bundle{
		Markdown:      []byte(smokeMarkdown),
		Values:        map[string]any{"subject": "Spring plant sale", "recipient": []string{"Ada Example"}},
		Template:      "neutral",
		DocumentType:  "letter",
		Configuration: "example",
	})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	text := pdfText(t, writePDF(t, pdf))
	for _, want := range []string{"Thank you for your enquiry", "Spring plant sale", "Ada Example"} {
		if !strings.Contains(text, want) {
			t.Errorf("PDF text lacks %q:\n%s", want, text)
		}
	}
}

func TestWorkerRoleRefusesUnknownPairing(t *testing.T) {
	base := startWorker(t)
	client, _ := workerapi.NewClient(base, "", 30*time.Second)
	_, err := client.Render(context.Background(), bundle.Bundle{
		Markdown: []byte("x"), Template: "other", DocumentType: "letter", Configuration: "example",
	})
	we, ok := err.(*workerapi.Error)
	if !ok || we.Status != http.StatusBadRequest {
		t.Errorf("err = %v, want a 400 from the Worker", err)
	}
}

func TestGatewayRendersADocumentThroughTheWorker(t *testing.T) {
	workerURL := startWorker(t)
	fakeOutline := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cookie") == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.URL.Path == "/api/auth.info" {
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"user": map[string]any{"id": "user-1"}}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{
			"id": "d1", "title": "Spring plant sale", "text": smokeMarkdown,
			"updatedAt": "2026-03-02T10:00:00.000Z", "collectionId": "c1",
		}})
	}))
	defer fakeOutline.Close()

	started, err := startup.Run("", os.Getenv)
	if err != nil {
		t.Fatal(err)
	}
	oc, _ := outline.NewClient(fakeOutline.URL, 0)
	wc, _ := workerapi.NewClient(workerURL, "", 90*time.Second)
	gw := httptest.NewServer(gateway.New(gateway.Config{
		Registry: started.Registry, Outline: oc, Worker: wc,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}).Handler())
	defer gw.Close()

	req, _ := http.NewRequest(http.MethodGet, gw.URL+"/render/d1", nil)
	req.Header.Set("Cookie", "accessToken=smoke")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d: %s", resp.StatusCode, body)
	}
	if got := resp.Header.Get("Content-Disposition"); !strings.Contains(got, `filename="Spring plant sale.pdf"`) {
		t.Errorf("Content-Disposition = %q", got)
	}
	text := pdfText(t, writePDF(t, body))
	// The letter has no title position; the derived last-modified date is
	// printed, in the Template's language.
	if !strings.Contains(text, "Thank you for your enquiry") || !strings.Contains(text, "2 March 2026") {
		t.Errorf("PDF text:\n%s", text)
	}
}
