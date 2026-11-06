package workerserver

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/taskmedia/outprint/internal/bundle"
	"github.com/taskmedia/outprint/internal/limits"
	"github.com/taskmedia/outprint/internal/startup"
	"github.com/taskmedia/outprint/internal/worker"
)

// ErrRestart is returned by Run after it shut down because the Worker must
// restart. The caller exits non-zero so the container runtime starts a fresh
// process.
var ErrRestart = errors.New("worker restart requested")

const (
	readHeaderTimeout = 10 * time.Second
	// readTimeout covers reading a bundle of the largest allowed size.
	readTimeout = 2 * time.Minute
	// writeMargin is added to the render limit for the response write.
	writeMargin     = 30 * time.Second
	idleTimeout     = 60 * time.Second
	maxHeaderBytes  = 16 << 10
	shutdownTimeout = 10 * time.Second
)

// RunConfig is everything Run needs.
type RunConfig struct {
	// Listen is the address to serve on. Ignored when Listener is set.
	Listen   string
	Listener net.Listener
	Token    string
	WorkDir  string
	CacheDir string
	Timeout  time.Duration
	// Limits are the page and PDF size limits; zero fields mean the starting
	// limits.
	Limits  limits.Job
	Startup *startup.Result
	Logger  *slog.Logger
	// Source is where the posture is read from; the zero value reads the
	// running process.
	Source *PostureSource
	// Renderer replaces the package pipeline in tests.
	Renderer Renderer
}

// Run checks the sandbox, serves until ctx ends or the Worker must restart,
// and shuts down gracefully. It returns nil after a clean stop, ErrRestart
// after a restart request, and any other error for a failure to start.
func Run(ctx context.Context, cfg RunConfig) error {
	src := OSPostureSource()
	if cfg.Source != nil {
		src = *cfg.Source
	}
	if err := RefuseRoot(src.UID); err != nil {
		return err
	}
	if err := checkWorkDir(cfg.WorkDir); err != nil {
		return err
	}
	CollectPosture(src).Log(cfg.Logger)
	WarnIfTokenMissing(cfg.Logger, cfg.Token)

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = worker.DefaultTimeout
	}
	restart := make(chan string, 1)
	renderer := cfg.Renderer
	if renderer == nil {
		renderer = PackageRenderer{Registry: cfg.Startup.Registry, WorkRoot: cfg.WorkDir, CacheDir: cfg.CacheDir, Timeout: timeout, Limits: cfg.Limits.WithDefaults(), Logger: cfg.Logger}
	}
	handler := New(Config{
		Token: cfg.Token, Logger: cfg.Logger,
		OnRestart: func(reason string) { restart <- reason },
	}, renderer).Handler()

	ln, err := listener(cfg)
	if err != nil {
		return err
	}
	srv := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      readTimeout + timeout + writeMargin,
		IdleTimeout:       idleTimeout,
		MaxHeaderBytes:    maxHeaderBytes,
	}
	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve(ln) }()
	cfg.Logger.Info("worker listening", "addr", ln.Addr().String(), "token_required", cfg.Token != "",
		"max_markdown_bytes", bundle.MaxMarkdownBytes, "timeout", timeout.String(),
		"max_pages", cfg.Limits.WithDefaults().MaxPages, "max_pdf_bytes", cfg.Limits.WithDefaults().MaxPDFBytes)

	select {
	case err := <-serveErr:
		return fmt.Errorf("serve: %w", err)
	case reason := <-restart:
		return shutdownThen(srv, fmt.Errorf("%w: %s", ErrRestart, reason))
	case <-ctx.Done():
		return shutdownThen(srv, nil)
	}
}

func shutdownThen(srv *http.Server, result error) error {
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		_ = srv.Close()
	}
	return result
}

func listener(cfg RunConfig) (net.Listener, error) {
	if cfg.Listener != nil {
		return cfg.Listener, nil
	}
	ln, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		return nil, fmt.Errorf("listen: %w", err)
	}
	return ln, nil
}

// checkWorkDir refuses to start when the configured work root is missing or
// not a directory; an unset root means the system temporary directory.
func checkWorkDir(dir string) error {
	if dir == "" {
		return nil
	}
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("refusing to start: work directory %s is not a directory", dir)
	}
	return nil
}

// WarnIfTokenMissing logs a warning when the Worker runs without a token, since
// then any peer on its network can submit jobs.
func WarnIfTokenMissing(logger *slog.Logger, token string) {
	if token != "" || logger == nil {
		return
	}
	logger.Warn("worker token is not set: any peer on the worker network can submit render jobs; set OUTPRINT_WORKER_TOKEN")
}
