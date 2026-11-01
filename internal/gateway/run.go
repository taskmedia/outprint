package gateway

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/taskmedia/outline-latex-pdf/internal/admission"
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
	OutlineURL string
	// OutlinePublicURL is Outline's own URL setting when OutlineURL is an
	// internal address (optional; see docs/images.md).
	OutlinePublicURL string
	// ImageHosts are the hosts besides OutlineURL that an image redirect may
	// lead to (service configuration imageHosts).
	ImageHosts   []string
	WorkerURL    string
	WorkerToken  string
	RenderLimit  time.Duration
	Logger       *slog.Logger
	OutlineLimit time.Duration
	// Admission sizes the slot limit, queue and per-user limit. The zero
	// value means the defaults.
	Admission admission.Config
}

// newAdmission builds the controller; a zero config means the defaults.
func newAdmission(cfg admission.Config, renderLimit time.Duration) (*admission.Controller, error) {
	if cfg == (admission.Config{}) {
		cfg = admission.Defaults()
	}
	cfg.RenderTimeout = renderLimit
	return admission.New(cfg)
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
	if outlineClient, err = outlineClient.WithImages(outline.ImageOptions{Hosts: cfg.ImageHosts, PublicURL: cfg.OutlinePublicURL}); err != nil {
		return err
	}
	workerClient, err := workerapi.NewClient(cfg.WorkerURL, cfg.WorkerToken, renderLimit+workerMargin)
	if err != nil {
		return err
	}
	admit, err := newAdmission(cfg.Admission, renderLimit)
	if err != nil {
		return err
	}
	ln := cfg.Listener
	if ln == nil {
		if ln, err = net.Listen("tcp", cfg.Listen); err != nil {
			return fmt.Errorf("listen: %w", err)
		}
	}
	handler := New(Config{Registry: cfg.Registry, Outline: outlineClient, Worker: workerClient, Admission: admit, Logger: cfg.Logger}).Handler()
	srv := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      outline.DefaultTimeout + admit.MaxWait() + renderLimit + workerMargin + readTimeout,
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
