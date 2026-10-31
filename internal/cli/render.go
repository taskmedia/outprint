// Package cli implements the renderer command line.
package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"

	"github.com/taskmedia/outline-latex-pdf/internal/config"
	"github.com/taskmedia/outline-latex-pdf/internal/metadata"
	"github.com/taskmedia/outline-latex-pdf/internal/packages"
	"github.com/taskmedia/outline-latex-pdf/internal/worker"
)

const usage = `usage: renderer render --template NAME --config NAME [--title TEXT] [--date YYYY-MM-DD]
                       [--json-errors] [--templates-root DIR] [--configs-root DIR]
                       [--timeout DURATION] [--debug-tex-log]
                       -o OUTPUT.pdf INPUT.md
       renderer warm-cache --template NAME --config NAME --out DIR
                           [--templates-root DIR] [--configs-root DIR]
       renderer validate [--service-config FILE]
                         [--templates-root DIR] [--configs-root DIR]
                         [--template NAME] [--config NAME]
` + serveUsage

const (
	envTemplatesRoot = config.EnvTemplatesRoot
	envConfigsRoot   = config.EnvConfigurationsRoot

	defaultTemplatesRoot = "./templates"
	defaultConfigsRoot   = "./configurations"

	envTimeout     = "RENDERER_TIMEOUT"
	envDebugTeXLog = "RENDERER_DEBUG_TEX_LOG"
	envWorkDir     = "RENDERER_WORK_DIR"
	envLogLevel    = "RENDERER_LOG_LEVEL"
	envCacheDir    = "RENDERER_TEXCACHE_DIR"

	defaultCacheDir = "/opt/renderer/texcache"

	isoDateLayout = "2006-01-02"
)

type renderOptions struct {
	Template      string
	Config        string
	TemplatesRoot string
	ConfigsRoot   string
	Title         string
	Date          string
	JSONErrors    bool
	Output        string
	Input         string
	Timeout       time.Duration
	DebugTeXLog   bool
	LogLevel      slog.Level
	WorkDir       string
	CacheDir      string
}

// Run executes the command line and returns the process exit code:
// 0 on success, 1 on failure, 2 on bad usage.
func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, usage)
		return 2
	}
	switch args[0] {
	case "render":
		return runRender(args[1:], stdout, stderr)
	case "validate":
		return runValidate(args[1:], os.Getenv, stdout, stderr)
	case "warm-cache":
		return runWarmCache(args[1:], os.Getenv, stderr)
	case "gateway":
		return runGateway(args[1:], os.Getenv, stderr)
	case "worker":
		return runWorker(args[1:], os.Getenv, stderr)
	case "healthcheck":
		return runHealthcheck(args[1:], stderr)
	}
	fmt.Fprintln(stderr, usage)
	return 2
}

func runRender(args []string, stdout, stderr io.Writer) int {
	opts, err := parseRender(args, os.Getenv)
	if err != nil {
		fmt.Fprintf(stderr, "renderer: %v\n%s\n", err, usage)
		return 2
	}
	if err := render(context.Background(), opts, stderr); err != nil {
		reportRenderError(err, opts, stdout, stderr)
		return 1
	}
	return 0
}

func parseRender(args []string, getenv func(string) string) (renderOptions, error) {
	var o renderOptions
	fs := flag.NewFlagSet("render", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&o.Template, "template", "", "Template name")
	fs.StringVar(&o.Config, "config", "", "Template Configuration name")
	fs.StringVar(&o.TemplatesRoot, "templates-root", firstNonEmpty(getenv(envTemplatesRoot), defaultTemplatesRoot), "Templates root")
	fs.StringVar(&o.ConfigsRoot, "configs-root", firstNonEmpty(getenv(envConfigsRoot), defaultConfigsRoot), "Configurations root")
	fs.StringVar(&o.Title, "title", "", "Document title, the default for the title field")
	fs.StringVar(&o.Date, "date", "", "last-modified date, ISO (default: the input file's modification date)")
	fs.BoolVar(&o.JSONErrors, "json-errors", false, "print metadata problems as the JSON error body on stdout")
	fs.StringVar(&o.Output, "o", "", "output PDF path")
	fs.DurationVar(&o.Timeout, "timeout", timeoutFromEnv(getenv), "wall-clock limit for the render")
	fs.BoolVar(&o.DebugTeXLog, "debug-tex-log", getenv(envDebugTeXLog) == "1",
		"print the full TeX log to stderr (UNSAFE: may contain document content)")
	o.WorkDir = getenv(envWorkDir)
	o.LogLevel = logLevelFromEnv(getenv)
	o.CacheDir = firstNonEmpty(getenv(envCacheDir), defaultCacheDir)
	if err := fs.Parse(args); err != nil {
		return renderOptions{}, err
	}
	switch {
	case o.Template == "":
		return renderOptions{}, errors.New("--template is required")
	case o.Config == "":
		return renderOptions{}, errors.New("--config is required")
	case o.Output == "":
		return renderOptions{}, errors.New("-o is required")
	case o.Timeout <= 0:
		return renderOptions{}, errors.New("--timeout must be positive")
	case fs.NArg() != 1:
		return renderOptions{}, errors.New("exactly one input file is required")
	}
	if _, err := time.Parse(isoDateLayout, o.Date); o.Date != "" && err != nil {
		return renderOptions{}, fmt.Errorf("--date %q is not an ISO date (YYYY-MM-DD)", o.Date)
	}
	o.Input = fs.Arg(0)
	return o, nil
}

func render(ctx context.Context, o renderOptions, stderr io.Writer) error {
	tpl, err := packages.LoadTemplate(o.TemplatesRoot, o.Template)
	if err != nil {
		return err
	}
	cfg, err := packages.LoadConfiguration(o.ConfigsRoot, o.Config)
	if err != nil {
		return err
	}
	markdown, err := os.ReadFile(o.Input)
	if err != nil {
		return fmt.Errorf("read input: %w", err)
	}
	modified, err := lastModified(o)
	if err != nil {
		return err
	}
	resolved, err := metadata.Resolve(metadata.Input{
		Markdown:      markdown,
		Title:         o.Title,
		Modified:      modified,
		Template:      tpl,
		Configuration: cfg,
	})
	if err != nil {
		return err
	}

	pdf, err := worker.Render(ctx, workerJob(o, resolved, tpl, cfg, stderr))
	if err != nil {
		return err
	}
	if err := os.WriteFile(o.Output, pdf, 0o644); err != nil {
		return fmt.Errorf("write output: %w", err)
	}
	return nil
}

// lastModified is the date given by --date, else the input file's
// modification date in UTC.
func lastModified(o renderOptions) (string, error) {
	if o.Date != "" {
		return o.Date, nil
	}
	info, err := os.Stat(o.Input)
	if err != nil {
		return "", fmt.Errorf("stat input: %w", err)
	}
	return info.ModTime().UTC().Format(isoDateLayout), nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func workerJob(o renderOptions, r metadata.Result, tpl packages.Template, cfg packages.Configuration, stderr io.Writer) worker.Job {
	job := worker.Job{
		Markdown:      r.Body,
		Values:        r.Values,
		Template:      tpl,
		DocumentType:  r.DocumentType,
		Configuration: cfg,
		Timeout:       o.Timeout,
		WorkRoot:      o.WorkDir,
		CacheDir:      o.CacheDir,
		Logger:        slog.New(slog.NewTextHandler(stderr, &slog.HandlerOptions{Level: o.LogLevel})),
	}
	if o.DebugTeXLog {
		job.DebugTeXLog = stderr
	}
	return job
}

func timeoutFromEnv(getenv func(string) string) time.Duration {
	if d, err := time.ParseDuration(getenv(envTimeout)); err == nil && d > 0 {
		return d
	}
	return worker.DefaultTimeout
}

// logLevelFromEnv reads RENDERER_LOG_LEVEL (debug, info, warn, error). The
// default, warn, logs only failures.
func logLevelFromEnv(getenv func(string) string) slog.Level {
	var level slog.Level
	name := getenv(envLogLevel)
	if name == "" || level.UnmarshalText([]byte(name)) != nil {
		return slog.LevelWarn
	}
	return level
}
