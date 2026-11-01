package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/taskmedia/outline-latex-pdf/internal/gateway"
	"github.com/taskmedia/outline-latex-pdf/internal/limits"
	"github.com/taskmedia/outline-latex-pdf/internal/startup"
	"github.com/taskmedia/outline-latex-pdf/internal/workerserver"
)

const (
	envListen      = "RENDERER_LISTEN"
	envOutlineURL  = "OUTLINE_URL"
	envWorkerURL   = "RENDERER_WORKER_URL"
	envWorkerToken = workerserver.TokenEnv

	defaultGatewayListen = ":8080"
	defaultWorkerListen  = ":8081"

	healthcheckTimeout = 3 * time.Second
)

const serveUsage = `       renderer gateway [--listen ADDR] [--service-config FILE]
                        (env: OUTLINE_URL, RENDERER_WORKER_URL, RENDERER_WORKER_TOKEN)
       renderer worker  [--listen ADDR] [--service-config FILE]
                        (env: RENDERER_WORK_DIR, RENDERER_WORKER_TOKEN, RENDERER_TIMEOUT,
                         RENDERER_MAX_PAGES, RENDERER_MAX_PDF_BYTES)
       renderer healthcheck URL`

type serveOptions struct {
	Listen        string
	ServiceConfig string
}

func parseServe(name, defaultListen string, args []string, getenv func(string) string) (serveOptions, error) {
	var o serveOptions
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&o.Listen, "listen", firstNonEmpty(getenv(envListen), defaultListen), "address to listen on")
	fs.StringVar(&o.ServiceConfig, "service-config", "", "service configuration file (default $RENDERER_SERVICE_CONFIG)")
	if err := fs.Parse(args); err != nil {
		return serveOptions{}, err
	}
	if fs.NArg() != 0 {
		return serveOptions{}, errors.New("unexpected arguments")
	}
	return o, nil
}

// serviceLogger logs at info by default: unlike the render command, a
// service is expected to log what it does.
func serviceLogger(stderr io.Writer, getenv func(string) string) *slog.Logger {
	level := slog.LevelInfo
	if name := getenv(envLogLevel); name != "" {
		if err := level.UnmarshalText([]byte(name)); err != nil {
			level = slog.LevelInfo
		}
	}
	return slog.New(slog.NewTextHandler(stderr, &slog.HandlerOptions{Level: level}))
}

func signalContext() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
}

func loadStartup(o serveOptions, getenv func(string) string, log *slog.Logger) (*startup.Result, error) {
	started, err := startup.Run(o.ServiceConfig, getenv)
	if err != nil {
		return nil, err
	}
	for _, w := range started.Warnings {
		log.Warn("package warning", "finding", w.String())
	}
	return started, nil
}

func runGateway(args []string, getenv func(string) string, stderr io.Writer) int {
	o, err := parseServe("gateway", defaultGatewayListen, args, getenv)
	if err != nil {
		fmt.Fprintf(stderr, "renderer: %v\n%s\n", err, usage)
		return 2
	}
	log := serviceLogger(stderr, getenv)
	if getenv(envOutlineURL) == "" || getenv(envWorkerURL) == "" {
		fmt.Fprintf(stderr, "renderer: gateway needs %s and %s\n", envOutlineURL, envWorkerURL)
		return 2
	}
	started, err := loadStartup(o, getenv, log)
	if err != nil {
		fmt.Fprintf(stderr, "renderer: %v\n", err)
		return 1
	}
	ctx, stop := signalContext()
	defer stop()
	err = gateway.Run(ctx, gateway.RunConfig{
		Listen: o.Listen, Registry: started.Registry, ImageHosts: started.Service.ImageHosts,
		OutlineURL: getenv(envOutlineURL), WorkerURL: getenv(envWorkerURL), WorkerToken: getenv(envWorkerToken),
		RenderLimit: timeoutFromEnv(getenv), Logger: log,
	})
	if err != nil {
		fmt.Fprintf(stderr, "renderer: gateway: %v\n", err)
		return 1
	}
	return 0
}

// runWorker returns non-zero after a restart request, a timeout, a kill or
// a failure to start, so the container runtime starts a fresh process.
func runWorker(args []string, getenv func(string) string, stderr io.Writer) int {
	o, err := parseServe("worker", defaultWorkerListen, args, getenv)
	if err != nil {
		fmt.Fprintf(stderr, "renderer: %v\n%s\n", err, usage)
		return 2
	}
	log := serviceLogger(stderr, getenv)
	jobLimits, err := limits.FromEnv(getenv)
	if err != nil {
		fmt.Fprintf(stderr, "renderer: %v\n", err)
		return 2
	}
	started, err := loadStartup(o, getenv, log)
	if err != nil {
		fmt.Fprintf(stderr, "renderer: %v\n", err)
		return 1
	}
	ctx, stop := signalContext()
	defer stop()
	err = workerserver.Run(ctx, workerserver.RunConfig{
		Listen: o.Listen, Token: getenv(envWorkerToken), WorkDir: getenv(envWorkDir),
		CacheDir: firstNonEmpty(getenv(envCacheDir), defaultCacheDir), Timeout: timeoutFromEnv(getenv),
		Limits: jobLimits, Startup: started, Logger: log,
	})
	if err != nil {
		log.Error("worker stopped", "error", err.Error())
		return 1
	}
	return 0
}

// runHealthcheck is a probe for container health checks: the image has no
// curl. It succeeds on a 200 from the URL.
func runHealthcheck(args []string, stderr io.Writer) int {
	if len(args) != 1 {
		fmt.Fprintf(stderr, "usage: renderer healthcheck URL\n")
		return 2
	}
	client := &http.Client{Timeout: healthcheckTimeout}
	resp, err := client.Get(args[0])
	if err != nil {
		fmt.Fprintf(stderr, "renderer: healthcheck: %v\n", err)
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(stderr, "renderer: healthcheck: status %d\n", resp.StatusCode)
		return 1
	}
	return 0
}
