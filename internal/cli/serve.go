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

	"github.com/taskmedia/outprint/internal/button"
	"github.com/taskmedia/outprint/internal/gateway"
	"github.com/taskmedia/outprint/internal/limits"
	"github.com/taskmedia/outprint/internal/settings"
	"github.com/taskmedia/outprint/internal/startup"
	"github.com/taskmedia/outprint/internal/version"
	"github.com/taskmedia/outprint/internal/workerserver"
)

const (
	envListen      = settings.EnvListen
	envOutlineURL  = settings.EnvOutlineURL
	envOutlinePub  = settings.EnvOutlinePublicURL
	envWorkerURL   = settings.EnvWorkerURL
	envWorkerToken = settings.EnvWorkerToken

	defaultGatewayListen = ":8080"
	defaultWorkerListen  = ":8081"

	healthcheckTimeout = 3 * time.Second
)

const serveUsage = `       outprint gateway [--listen ADDR] [--service-config FILE]
                        (env: OUTLINE_URL, OUTLINE_PUBLIC_URL, OUTPRINT_WORKER_URL, OUTPRINT_WORKER_TOKEN,
                         OUTPRINT_ADMISSION_SLOTS, OUTPRINT_ADMISSION_QUEUE_DEPTH,
                         OUTPRINT_ADMISSION_PER_USER, OUTPRINT_ADMISSION_MAX_WAIT)
       outprint worker  [--listen ADDR] [--service-config FILE]
                        (env: OUTPRINT_WORKER_TOKEN, OUTPRINT_TIMEOUT,
                         OUTPRINT_MAX_PAGES, OUTPRINT_MAX_PDF_BYTES)
       outprint healthcheck URL`

type serveOptions struct {
	Listen        string
	ServiceConfig string
}

func parseServe(name, defaultListen string, args []string, getenv func(string) string) (serveOptions, error) {
	var o serveOptions
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&o.Listen, "listen", firstNonEmpty(getenv(envListen), defaultListen), "address to listen on")
	fs.StringVar(&o.ServiceConfig, "service-config", "", "service configuration file (default $OUTPRINT_SERVICE_CONFIG)")
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

// logStartup records which build is running, once, at info level.
func logStartup(log *slog.Logger, service string) {
	log.Info("starting "+service, "version", version.Version, "commit", version.Commit)
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
		fmt.Fprintf(stderr, "outprint: %v\n%s\n", err, usage)
		return 2
	}
	log := serviceLogger(stderr, getenv)
	logStartup(log, "gateway")
	if getenv(envOutlineURL) == "" || getenv(envWorkerURL) == "" {
		fmt.Fprintf(stderr, "outprint: gateway needs %s and %s\n", envOutlineURL, envWorkerURL)
		return 2
	}
	started, err := loadStartup(o, getenv, log)
	if err != nil {
		fmt.Fprintf(stderr, "outprint: %v\n", err)
		return 1
	}
	var buttonConfig button.Config
	buttonConfig.ReadEnv(getenv)
	ctx, stop := signalContext()
	defer stop()
	err = gateway.Run(ctx, gateway.RunConfig{
		Listen: o.Listen, Registry: started.Registry, ImageHosts: started.Service.ImageHosts,
		OutlineURL: getenv(envOutlineURL), OutlinePublicURL: getenv(envOutlinePub), WorkerURL: getenv(envWorkerURL), WorkerToken: getenv(envWorkerToken),
		RenderLimit: timeoutFromEnv(getenv), Admission: started.Service.Admission, Button: buttonConfig, Logger: log,
	})
	if err != nil {
		fmt.Fprintf(stderr, "outprint: gateway: %v\n", err)
		return 1
	}
	return 0
}

// runWorker returns non-zero after a restart request, a timeout, a kill or
// a failure to start, so the container runtime starts a fresh process.
func runWorker(args []string, getenv func(string) string, stderr io.Writer) int {
	o, err := parseServe("worker", defaultWorkerListen, args, getenv)
	if err != nil {
		fmt.Fprintf(stderr, "outprint: %v\n%s\n", err, usage)
		return 2
	}
	log := serviceLogger(stderr, getenv)
	logStartup(log, "worker")
	jobLimits, err := limits.FromEnv(getenv)
	if err != nil {
		fmt.Fprintf(stderr, "outprint: %v\n", err)
		return 2
	}
	started, err := loadStartup(o, getenv, log)
	if err != nil {
		fmt.Fprintf(stderr, "outprint: %v\n", err)
		return 1
	}
	ctx, stop := signalContext()
	defer stop()
	err = workerserver.Run(ctx, workerserver.RunConfig{
		Listen: o.Listen, Token: getenv(envWorkerToken),
		CacheDir: defaultCacheDir, Timeout: timeoutFromEnv(getenv),
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
		fmt.Fprintf(stderr, "usage: outprint healthcheck URL\n")
		return 2
	}
	client := &http.Client{Timeout: healthcheckTimeout}
	resp, err := client.Get(args[0])
	if err != nil {
		fmt.Fprintf(stderr, "outprint: healthcheck: %v\n", err)
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(stderr, "outprint: healthcheck: status %d\n", resp.StatusCode)
		return 1
	}
	return 0
}
