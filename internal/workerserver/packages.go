package workerserver

import (
	"context"
	"log/slog"
	"time"

	"github.com/taskmedia/outprint/internal/bundle"
	"github.com/taskmedia/outprint/internal/config"
	"github.com/taskmedia/outprint/internal/limits"
	"github.com/taskmedia/outprint/internal/packages"
	"github.com/taskmedia/outprint/internal/worker"
)

// PackageRenderer runs the real pipeline. It resolves the bundle's names
// against the packages loaded at startup and refuses a pairing that is not
// on the allow-list, so the Gateway is not the only line of defence.
type PackageRenderer struct {
	Registry *packages.Registry
	WorkRoot string
	CacheDir string
	Timeout  time.Duration
	Limits   limits.Job
	Logger   *slog.Logger
}

// Render implements Renderer.
func (p PackageRenderer) Render(ctx context.Context, b bundle.Bundle) ([]byte, error) {
	if !p.Registry.Allowed(config.Binding{Template: b.Template, Configuration: b.Configuration}) {
		return nil, ErrUnknownPackage
	}
	tpl, okT := p.Registry.Template(b.Template)
	cfg, okC := p.Registry.Configuration(b.Configuration)
	if _, okD := tpl.DocumentTypes[b.DocumentType]; !okT || !okC || !okD {
		return nil, ErrUnknownPackage
	}
	return worker.Render(ctx, worker.Job{
		Markdown:      b.Markdown,
		Values:        b.Values,
		Template:      tpl,
		DocumentType:  b.DocumentType,
		Configuration: cfg,
		Images:        b.Images,
		Timeout:       p.Timeout,
		MaxPages:      p.Limits.MaxPages,
		MaxPDFBytes:   p.Limits.MaxPDFBytes,
		WorkRoot:      p.WorkRoot,
		CacheDir:      p.CacheDir,
		Logger:        p.Logger,
	})
}
