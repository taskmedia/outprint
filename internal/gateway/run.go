package gateway

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/taskmedia/outline-latex-pdf/internal/outline"
	"github.com/taskmedia/outline-latex-pdf/internal/packages"
	"github.com/taskmedia/outline-latex-pdf/internal/worker"
	"github.com/taskmedia/outline-latex-pdf/internal/workerapi"
)

const (
	readHeaderTimeout = 10 * time.Second
	readTimeout       = 30 * time.Second
	idleTimeout       = 60 * time.Second
	maxHeaderBytes    = 32 << 10
	shutdownTimeout   = 10 * time.Second
	// workerMargin is how much longer than the render limit the Gateway
	// waits for the Worker, so the Worker's own timeout answer arrives.
	workerMargin = 30 * time.Second
)

// RunConfig is everything Run needs.
type RunConfig struct {
	Listen   string
	Listener net.Listener
	Registry *packages.Registry
	// OutlineURL is the only origin the user's cookie is ever sent to.
	OutlineURL   string
	WorkerURL    string
	WorkerToken  string
	RenderLimit  time.Duration
	Logger       *slog.Logger
	OutlineLimit time.Duration
}

// Run serves until ctx ends. A failure to build the clients or to listen is
// returned before anything is served.
func Run(ctx context.Context, cfg RunConfig) error {
	renderLimit := cfg.RenderLimit
	if renderLimit <= 0 {
		renderLimit = worker.DefaultTimeout
	}
	outlineClient, err := outline.NewClient(cfg.OutlineURL, cfg.OutlineLimit)
	if err != nil {
		return err
	}
	workerClient, err := workerapi.NewClient(cfg.WorkerURL, cfg.WorkerToken, renderLimit+workerMargin)
	if err != nil {
		return err
	}
	ln := cfg.Listener
	if ln == nil {
		if ln, err = net.Listen("tcp", cfg.Listen); err != nil {
			return fmt.Errorf("listen: %w", err)
		}
	}
	handler := New(Config{Registry: cfg.Registry, Outline: outlineClient, Worker: workerClient, Logger: cfg.Logger}).Handler()
	srv := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      outline.DefaultTimeout + renderLimit + workerMargin + readTimeout,
		IdleTimeout:       idleTimeout,
		MaxHeaderBytes:    maxHeaderBytes,
	}
	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve(ln) }()
	cfg.Logger.Info("gateway listening", "addr", ln.Addr().String(), "outline_origin", outlineClient.Origin(),
		"worker_token", cfg.WorkerToken != "")

	select {
	case err := <-serveErr:
		return fmt.Errorf("serve: %w", err)
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		_ = srv.Close()
	}
	return nil
}
