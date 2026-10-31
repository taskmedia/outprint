// Package cli implements the renderer command line.
package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"gopkg.in/yaml.v3"

	"github.com/taskmedia/outline-latex-pdf/internal/packages"
	"github.com/taskmedia/outline-latex-pdf/internal/worker"
)

const usage = `usage: renderer render --template NAME --config NAME [--type NAME]
                       [--fields FILE] [--templates-root DIR] [--configs-root DIR]
                       -o OUTPUT.pdf INPUT.md`

const (
	envTemplatesRoot = "RENDERER_TEMPLATES_ROOT"
	envConfigsRoot   = "RENDERER_CONFIGURATIONS_ROOT"

	defaultTemplatesRoot = "./templates"
	defaultConfigsRoot   = "./configurations"

	defaultTypeField = "document_type"
)

type renderOptions struct {
	Template      string
	Config        string
	DocumentType  string
	TemplatesRoot string
	ConfigsRoot   string
	FieldsFile    string
	Output        string
	Input         string
}

// Run executes the command line and returns the process exit code.
func Run(args []string, stderr io.Writer) int {
	if len(args) == 0 || args[0] != "render" {
		fmt.Fprintln(stderr, usage)
		return 2
	}
	opts, err := parseRender(args[1:], os.Getenv)
	if err != nil {
		fmt.Fprintf(stderr, "renderer: %v\n%s\n", err, usage)
		return 2
	}
	if err := render(context.Background(), opts); err != nil {
		fmt.Fprintf(stderr, "renderer: %v\n", err)
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
	fs.StringVar(&o.DocumentType, "type", "", "Document Type (default: the Configuration's document_type)")
	fs.StringVar(&o.TemplatesRoot, "templates-root", firstNonEmpty(getenv(envTemplatesRoot), defaultTemplatesRoot), "Templates root")
	fs.StringVar(&o.ConfigsRoot, "configs-root", firstNonEmpty(getenv(envConfigsRoot), defaultConfigsRoot), "Configurations root")
	fs.StringVar(&o.FieldsFile, "fields", "", "YAML file with Document field values")
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
	documentType, err := resolveDocumentType(o.DocumentType, cfg.Defaults)
	if err != nil {
		return err
	}
	markdown, err := os.ReadFile(o.Input)
	if err != nil {
		return fmt.Errorf("read input: %w", err)
	}
	fields, err := readFields(o.FieldsFile)
	if err != nil {
		return err
	}

	pdf, err := worker.Render(ctx, worker.Job{
		Markdown:      markdown,
		Fields:        fields,
		Template:      tpl,
		DocumentType:  documentType,
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

func resolveDocumentType(flagValue string, defaults map[string]any) (string, error) {
	if flagValue != "" {
		return flagValue, nil
	}
	if def, ok := defaults[defaultTypeField].(string); ok && def != "" {
		return def, nil
	}
	return "", errors.New("no document type: pass --type or set document_type in the Configuration defaults")
}

func readFields(path string) (map[string]any, error) {
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read fields: %w", err)
	}
	var fields map[string]any
	if err := yaml.Unmarshal(data, &fields); err != nil {
		return nil, fmt.Errorf("parse fields: %w", err)
	}
	return fields, nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
