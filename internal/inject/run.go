package inject

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

const (
	readHeaderTimeout = 10 * time.Second
	idleTimeout       = 2 * time.Minute
	maxHeaderBytes    = 64 << 10
	shutdownTimeout   = 10 * time.Second
	healthTimeout     = 3 * time.Second
)

// Run serves until ctx ends. Only header reading and idle connections have a
// timeout: Outline streams events and WebSockets, and uploads can be slow.
func Run(ctx context.Context, cfg Config) error {
	h, err := New(cfg)
	if err != nil {
		return err
	}
	ln := cfg.Listener
	if ln == nil {
		if ln, err = net.Listen("tcp", firstNonEmpty(cfg.Listen, DefaultListen)); err != nil {
			return fmt.Errorf("listen: %w", err)
		}
	}
	srv := &http.Server{
		Handler:           h,
		ReadHeaderTimeout: readHeaderTimeout,
		IdleTimeout:       idleTimeout,
		MaxHeaderBytes:    maxHeaderBytes,
	}
	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve(ln) }()
	slog.Info("injector listening", "addr", ln.Addr().String(), "gateway_route", cfg.Gateway != "")

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
	if err := <-serveErr; err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve: %w", err)
	}
	return nil
}

// Healthcheck succeeds when the URL answers 200. The image has no shell or
// curl, so the container health check runs the binary itself.
func Healthcheck(ctx context.Context, target string) error {
	ctx, cancel := context.WithTimeout(ctx, healthTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<10))
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("healthcheck: status %d", resp.StatusCode)
	}
	return nil
}

// Main is the entry point of the injector binary: "injector" serves,
// "injector healthcheck URL" probes.
func Main(args []string, getenv func(string) string, stdout, stderr io.Writer) int {
	if len(args) > 0 {
		return mainSubcommand(args, stderr)
	}
	cfg, err := ConfigFromEnv(getenv)
	if err != nil {
		fmt.Fprintf(stderr, "injector: %v\n", err)
		return 2
	}
	configureLogging(cfg.LogLevel, stderr)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := Run(ctx, cfg); err != nil {
		fmt.Fprintf(stderr, "injector: %v\n", err)
		return 1
	}
	return 0
}

func mainSubcommand(args []string, stderr io.Writer) int {
	if args[0] != "healthcheck" || len(args) != 2 {
		fmt.Fprintln(stderr, "usage: injector            (configured by INJECTOR_* variables)\n       injector healthcheck URL")
		return 2
	}
	if err := Healthcheck(context.Background(), args[1]); err != nil {
		fmt.Fprintf(stderr, "injector: %v\n", err)
		return 1
	}
	return 0
}

func configureLogging(level string, w io.Writer) {
	var l slog.Level
	if err := l.UnmarshalText([]byte(level)); err != nil && level != "" {
		l = slog.LevelInfo
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: l})))
}
