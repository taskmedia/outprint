package workerserver

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/taskmedia/outline-latex-pdf/internal/worker"
	"github.com/taskmedia/outline-latex-pdf/internal/workerapi"
)

func runConfig(t *testing.T, r Renderer) (RunConfig, *bytes.Buffer, string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	logs := &bytes.Buffer{}
	src := PostureSource{UID: 10001, GID: 10001, ProcDir: t.TempDir()}
	return RunConfig{
		Listener: ln, Renderer: r, Source: &src,
		Logger: slog.New(slog.NewTextHandler(logs, nil)),
	}, logs, "http://" + ln.Addr().String()
}

func TestRunRefusesRoot(t *testing.T) {
	cfg, _, _ := runConfig(t, &fakeRenderer{})
	cfg.Source.UID = 0
	if err := Run(context.Background(), cfg); !errors.Is(err, ErrRoot) {
		t.Errorf("Run = %v, want ErrRoot", err)
	}
}

func TestRunRefusesMissingWorkDir(t *testing.T) {
	cfg, _, _ := runConfig(t, &fakeRenderer{})
	cfg.WorkDir = filepath.Join(t.TempDir(), "missing")
	err := Run(context.Background(), cfg)
	if err == nil || !strings.Contains(err.Error(), "work directory") {
		t.Errorf("Run = %v", err)
	}
}

func TestRunLogsPostureAndStopsOnContext(t *testing.T) {
	cfg, logs, base := runConfig(t, &fakeRenderer{})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Run(ctx, cfg) }()
	waitHealthy(t, base)
	cancel()
	if err := <-done; err != nil {
		t.Errorf("Run = %v", err)
	}
	if !strings.Contains(logs.String(), "worker security posture") || !strings.Contains(logs.String(), "uid=10001") {
		t.Errorf("posture not logged:\n%s", logs)
	}
}

func TestRunExitsWithRestartAfterTimeoutResponse(t *testing.T) {
	r := &fakeRenderer{err: &worker.Error{Phase: worker.PhaseTimeout, Kind: worker.KindTimeout, JobID: "j"}}
	cfg, _, base := runConfig(t, r)
	done := make(chan error, 1)
	go func() { done <- Run(context.Background(), cfg) }()
	waitHealthy(t, base)

	c, _ := workerapi.NewClient(base, "", 5*time.Second)
	_, err := c.Render(context.Background(), sample())
	var werr *workerapi.Error
	if !errors.As(err, &werr) || werr.Status != 504 {
		t.Fatalf("the response must be delivered before the exit: %v", err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, ErrRestart) {
			t.Errorf("Run = %v, want ErrRestart", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after a timeout")
	}
}

func waitHealthy(t *testing.T, base string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(base + workerapi.HealthPath)
		if err == nil {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("worker did not come up")
}
