// Package cli implements the renderer command line.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/taskmedia/outline-latex-pdf/internal/config"
	"github.com/taskmedia/outline-latex-pdf/internal/metadata"
	"github.com/taskmedia/outline-latex-pdf/internal/packages"
	"github.com/taskmedia/outline-latex-pdf/internal/worker"
)

const usage = `usage: renderer render --template NAME --config NAME [--title TEXT] [--date YYYY-MM-DD]
                       [--json-errors] [--templates-root DIR] [--configs-root DIR]
                       -o OUTPUT.pdf INPUT.md
       renderer validate [--service-config FILE]
                         [--templates-root DIR] [--configs-root DIR]
                         [--template NAME] [--config NAME]`

const (
	envTemplatesRoot = config.EnvTemplatesRoot
	envConfigsRoot   = config.EnvConfigurationsRoot

	defaultTemplatesRoot = "./templates"
	defaultConfigsRoot   = "./configurations"

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
	if err := render(context.Background(), opts); err != nil {
		reportRenderError(err, opts, stdout, stderr)
		return 1
	}
	return 0
}

// reportRenderError prints metadata problems readably, or as the JSON error
// body with --json-errors, and any other failure as a single line.
func reportRenderError(err error, opts renderOptions, stdout, stderr io.Writer) {
	var list *metadata.ErrorList
	if !errors.As(err, &list) {
		fmt.Fprintf(stderr, "renderer: %v\n", err)
		return
	}
	if opts.JSONErrors {
		if encodeErr := json.NewEncoder(stdout).Encode(list); encodeErr != nil {
			fmt.Fprintf(stderr, "renderer: encode errors: %v\n", encodeErr)
		}
		return
	}
	fmt.Fprintf(stderr, "renderer: %s has %d problem(s):\n", opts.Input, len(list.Errors))
	for _, e := range list.Errors {
		fmt.Fprintf(stderr, "  %s\n", e)
	}
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
	case fs.NArg() != 1:
		return renderOptions{}, errors.New("exactly one input file is required")
	}
	if _, err := time.Parse(isoDateLayout, o.Date); o.Date != "" && err != nil {
		return renderOptions{}, fmt.Errorf("--date %q is not an ISO date (YYYY-MM-DD)", o.Date)
	}
	o.Input = fs.Arg(0)
	return o, nil
}

func render(ctx context.Context, o renderOptions) error {
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

	pdf, err := worker.Render(ctx, worker.Job{
		Markdown:      resolved.Body,
		Values:        resolved.Values,
		Template:      tpl,
		DocumentType:  resolved.DocumentType,
		Configuration: cfg,
	})
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
