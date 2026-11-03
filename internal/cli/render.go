// Package cli implements the outprint command line.
package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/taskmedia/outprint/internal/config"
	"github.com/taskmedia/outprint/internal/images"
	"github.com/taskmedia/outprint/internal/metadata"
	"github.com/taskmedia/outprint/internal/packages"
	"github.com/taskmedia/outprint/internal/settings"
	"github.com/taskmedia/outprint/internal/version"
	"github.com/taskmedia/outprint/internal/worker"
)

const usage = `usage: outprint render --template NAME --config NAME [--title TEXT] [--date YYYY-MM-DD]
                       [--json-errors] [--templates-root DIR] [--configs-root DIR]
                       [--timeout DURATION] [--debug-tex-log]
                       -o OUTPUT.pdf INPUT.md
       outprint warm-cache --template NAME --config NAME --out DIR
                           [--templates-root DIR] [--configs-root DIR]
       outprint validate [--service-config FILE]
                         [--templates-root DIR] [--configs-root DIR]
                         [--template NAME] [--config NAME]
       outprint version
` + serveUsage

const (
	envTemplatesRoot = config.EnvTemplatesRoot
	envConfigsRoot   = config.EnvConfigurationsRoot

	defaultTemplatesRoot = "./templates"
	defaultConfigsRoot   = "./configurations"

	envTimeout  = settings.EnvTimeout
	envLogLevel = settings.EnvLogLevel

	defaultCacheDir = "/opt/outprint/texcache"

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
	case "version":
		fmt.Fprintln(stdout, version.Line("outprint"))
		return 0
	}
	fmt.Fprintln(stderr, usage)
	return 2
}

func runRender(args []string, stdout, stderr io.Writer) int {
	opts, err := parseRender(args, os.Getenv)
	if err != nil {
		fmt.Fprintf(stderr, "outprint: %v\n%s\n", err, usage)
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
	fs.BoolVar(&o.DebugTeXLog, "debug-tex-log", false,
		"print the full TeX log to stderr (UNSAFE: may contain document content)")
	o.LogLevel = logLevelFromEnv(getenv)
	o.CacheDir = defaultCacheDir
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
	resolved, imgs, err := resolveDocument(ctx, o, markdown, tpl, cfg, modified)
	if err != nil {
		return err
	}
	pdf, err := worker.Render(ctx, workerJob(o, resolved, imgs, tpl, cfg, stderr))
	if err != nil {
		return err
	}
	if err := os.WriteFile(o.Output, pdf, 0o644); err != nil {
		return fmt.Errorf("write output: %w", err)
	}
	return nil
}

// resolveDocument resolves the Metadata Block and the images of the input.
// The images are read from the directory of the input file by the same
// pipeline the Gateway runs on Outline attachments. The problems of both
// come back as one list.
func resolveDocument(ctx context.Context, o renderOptions, markdown []byte, tpl packages.Template,
	cfg packages.Configuration, modified string) (metadata.Result, images.Result, error) {
	resolved, err := metadata.Resolve(metadata.Input{
		Markdown:      markdown,
		Title:         o.Title,
		Modified:      modified,
		Template:      tpl,
		Configuration: cfg,
	})
	var metaList *metadata.ErrorList
	if err != nil && !errors.As(err, &metaList) {
		return metadata.Result{}, images.Result{}, err
	}
	body, offset := metadata.Body(markdown)
	imgs, problems, err := images.Process(ctx, body, offset, images.DirSource{Root: filepath.Dir(o.Input)})
	if err != nil {
		return metadata.Result{}, images.Result{}, fmt.Errorf("read images: %w", err)
	}
	if list := images.Combine(metaList, problems); list != nil {
		return metadata.Result{}, images.Result{}, list
	}
	resolved.Body = imgs.Markdown
	return resolved, imgs, nil
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

func workerJob(o renderOptions, r metadata.Result, imgs images.Result, tpl packages.Template, cfg packages.Configuration, stderr io.Writer) worker.Job {
	job := worker.Job{
		Markdown:      r.Body,
		Images:        imgs.Images,
		Values:        r.Values,
		Template:      tpl,
		DocumentType:  r.DocumentType,
		Configuration: cfg,
		Timeout:       o.Timeout,
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

// logLevelFromEnv reads OUTPRINT_LOG_LEVEL (debug, info, warn, error). The
// default, warn, logs only failures.
func logLevelFromEnv(getenv func(string) string) slog.Level {
	var level slog.Level
	name := getenv(envLogLevel)
	if name == "" || level.UnmarshalText([]byte(name)) != nil {
		return slog.LevelWarn
	}
	return level
}
