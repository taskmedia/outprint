package gateway

import (
	"bytes"
	"context"
	"log/slog"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/taskmedia/outline-latex-pdf/internal/config"
	"github.com/taskmedia/outline-latex-pdf/internal/startup"
)

func TestRunServesAndStops(t *testing.T) {
	started, err := startup.FromService(config.Service{
		TemplatesRoot: "../../templates", ConfigurationsRoot: "../../configurations",
		Bindings: []config.Binding{{Template: "neutral", Configuration: "example"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, RunConfig{
			Listener: ln, Registry: started.Registry,
			OutlineURL: "https://wiki.example.test", WorkerURL: "http://worker.example.test:8081",
			Logger: slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)),
		})
	}()
	deadline := time.Now().Add(3 * time.Second)
	for {
		resp, err := http.Get("http://" + ln.Addr().String() + "/healthz")
		if err == nil {
			resp.Body.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("gateway did not come up")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	if err := <-done; err != nil {
		t.Errorf("Run = %v", err)
	}
}

func TestRunRejectsBadURLs(t *testing.T) {
	log := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	for name, cfg := range map[string]RunConfig{
		"outline": {OutlineURL: "nope", WorkerURL: "http://w.example.test", Logger: log},
		"worker":  {OutlineURL: "https://o.example.test", WorkerURL: "nope", Logger: log},
	} {
		if err := Run(context.Background(), cfg); err == nil {
			t.Errorf("%s: Run accepted a bad URL", name)
		}
	}
}
