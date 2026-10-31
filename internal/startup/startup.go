// Package startup is what the Gateway and the Worker run before they serve:
// load the service configuration, load and validate every package, and
// refuse with a clear cause if anything is wrong.
package startup

import (
	"errors"
	"fmt"
	"strings"

	"github.com/taskmedia/outline-latex-pdf/internal/config"
	"github.com/taskmedia/outline-latex-pdf/internal/packages"
)

// Result is what a successful startup hands to the service.
type Result struct {
	Service  config.Service
	Registry *packages.Registry
	// Warnings are findings that do not stop startup; the caller logs them.
	Warnings []packages.Finding
}

// RefusedError is returned when startup must not proceed. Its message names
// every cause.
type RefusedError struct {
	Causes []string
}

func (e *RefusedError) Error() string {
	return fmt.Sprintf("refusing to start: %d problem(s):\n  %s", len(e.Causes), strings.Join(e.Causes, "\n  "))
}

// Run loads the service configuration at path (or, when path is empty, at
// the file named by $RENDERER_SERVICE_CONFIG), then loads and validates all
// packages. getenv is os.Getenv in production.
func Run(path string, getenv func(string) string) (*Result, error) {
	if path == "" {
		path = getenv(config.EnvServiceConfig)
	}
	if path == "" {
		return nil, &RefusedError{Causes: []string{"no service configuration: pass a path or set " + config.EnvServiceConfig}}
	}
	svc, errs := config.Load(path, getenv)
	if len(errs) > 0 {
		return nil, &RefusedError{Causes: causesOf(path, errs)}
	}
	return FromService(svc)
}

// FromService loads and validates all packages of an already parsed
// service configuration.
func FromService(svc config.Service) (*Result, error) {
	registry, findings := packages.Load(svc)
	if errs := packages.Errors(findings); len(errs) > 0 {
		causes := make([]string, len(errs))
		for i, f := range errs {
			causes[i] = f.String()
		}
		return nil, &RefusedError{Causes: causes}
	}
	return &Result{Service: svc, Registry: registry, Warnings: packages.Warnings(findings)}, nil
}

func causesOf(path string, errs []error) []string {
	causes := make([]string, len(errs))
	for i, err := range errs {
		causes[i] = fmt.Sprintf("service %s: %v", path, err)
	}
	return causes
}

// IsRefused reports whether err is a startup refusal.
func IsRefused(err error) bool {
	var r *RefusedError
	return errors.As(err, &r)
}
