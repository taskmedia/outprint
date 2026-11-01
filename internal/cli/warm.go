package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"maps"
	"os"
	"slices"

	"github.com/taskmedia/outprint/internal/images"
	"github.com/taskmedia/outprint/internal/metadata"
	"github.com/taskmedia/outprint/internal/packages"
	"github.com/taskmedia/outprint/internal/worker"
)

// warmDocument touches every font style the Templates use: roman, bold,
// italic, bold italic and the monospace font of code.
const warmDocument = "Warm **bold** *italic* ***both*** `code`\n"

// runWarmCache renders a small document of each Document Type with an empty
// cache and keeps the luaotfload cache. It runs when the image is built.
func runWarmCache(args []string, getenv func(string) string, stderr io.Writer) int {
	o, err := parseWarm(args, getenv)
	if err != nil {
		fmt.Fprintf(stderr, "outprint: %v\n%s\n", err, usage)
		return 2
	}
	if err := warmCache(context.Background(), o, stderr); err != nil {
		fmt.Fprintf(stderr, "outprint: warm-cache: %v\n", err)
		return 1
	}
	return 0
}

type warmOptions struct {
	Template, Config, TemplatesRoot, ConfigsRoot, Out string
}

func parseWarm(args []string, getenv func(string) string) (warmOptions, error) {
	var o warmOptions
	fs := flag.NewFlagSet("warm-cache", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&o.Template, "template", "", "Template name")
	fs.StringVar(&o.Config, "config", "", "Template Configuration name")
	fs.StringVar(&o.TemplatesRoot, "templates-root", firstNonEmpty(getenv(envTemplatesRoot), defaultTemplatesRoot), "Templates root")
	fs.StringVar(&o.ConfigsRoot, "configs-root", firstNonEmpty(getenv(envConfigsRoot), defaultConfigsRoot), "Configurations root")
	fs.StringVar(&o.Out, "out", "", "directory that receives the cache")
	if err := fs.Parse(args); err != nil {
		return warmOptions{}, err
	}
	if o.Template == "" || o.Config == "" || o.Out == "" || fs.NArg() != 0 {
		return warmOptions{}, errors.New("--template, --config and --out are required")
	}
	return o, nil
}

func warmCache(ctx context.Context, o warmOptions, stderr io.Writer) error {
	tpl, err := packages.LoadTemplate(o.TemplatesRoot, o.Template)
	if err != nil {
		return err
	}
	cfg, err := packages.LoadConfiguration(o.ConfigsRoot, o.Config)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(o.Out, 0o755); err != nil {
		return fmt.Errorf("create output: %w", err)
	}
	for _, docType := range slices.Sorted(maps.Keys(tpl.DocumentTypes)) {
		markdown := "```yaml\ntype: " + docType + "\ntitle: Warm\n```\n" + warmDocument
		resolved, err := metadata.Resolve(metadata.Input{
			Markdown: []byte(markdown), Modified: "2026-01-01", Template: tpl, Configuration: cfg,
		})
		if err != nil {
			return fmt.Errorf("document type %s: %w", docType, err)
		}
		job := workerJob(renderOptions{Timeout: worker.DefaultTimeout}, resolved, images.Result{}, tpl, cfg, stderr)
		if err := worker.WarmCache(ctx, job, o.Out); err != nil {
			return fmt.Errorf("document type %s: %w", docType, err)
		}
	}
	return nil
}
