// Package limits is the one place that names the renderer's resource limits.
// The application enforces the per-job limits (time, pages, PDF size, TeX
// runs); the container limits are enforced by the container runtime and are
// listed here so the Compose file and the security tests agree with them.
package limits

import (
	"fmt"
	"strconv"
	"time"
)

// Per-job limits, enforced by the Worker pipeline.
const (
	// WallClock is the time a whole job may take.
	WallClock = 60 * time.Second
	// MaxPages is the page count of the produced PDF.
	MaxPages = 500
	// MaxPDFBytes is the size of the produced PDF. It is also the largest PDF
	// the Gateway accepts from the Worker, so it can be lowered by
	// configuration but not raised.
	MaxPDFBytes = 50 << 20
	// MaxMarkdownBytes is the size of the Document body in a job.
	MaxMarkdownBytes = 1 << 20
	// MaxTeXRuns is how often LuaLaTeX may run for one job.
	MaxTeXRuns = 2
)

// Worker container limits. The shipped Compose file sets them; the sandbox
// tests fail if it stops doing so.
const (
	ContainerMemoryBytes = 1 << 30
	ContainerCPUs        = 1
	ContainerPIDs        = 128
	ContainerTmpfsBytes  = 256 << 20
)

// Environment variables that tighten (or, for pages and time, change) the
// per-job limits. Tests use them to keep resource bombs short.
const (
	EnvTimeout     = "RENDERER_TIMEOUT"
	EnvMaxPages    = "RENDERER_MAX_PAGES"
	EnvMaxPDFBytes = "RENDERER_MAX_PDF_BYTES"
)

// Job is the effective per-job limits.
type Job struct {
	MaxPages    int
	MaxPDFBytes int64
}

// Defaults returns the starting limits.
func Defaults() Job {
	return Job{MaxPages: MaxPages, MaxPDFBytes: MaxPDFBytes}
}

// WithDefaults fills zero fields with the starting limits.
func (j Job) WithDefaults() Job {
	if j.MaxPages <= 0 {
		j.MaxPages = MaxPages
	}
	if j.MaxPDFBytes <= 0 {
		j.MaxPDFBytes = MaxPDFBytes
	}
	return j
}

// FromEnv reads the page and PDF size limits. An unset variable keeps the
// default; a malformed or out-of-range value is an error, so a typo never
// silently leaves a limit at its default or switches it off.
func FromEnv(getenv func(string) string) (Job, error) {
	job := Defaults()
	if raw := getenv(EnvMaxPages); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			return Job{}, fmt.Errorf("%s must be a positive integer, got %q", EnvMaxPages, raw)
		}
		job.MaxPages = n
	}
	if raw := getenv(EnvMaxPDFBytes); raw != "" {
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || n <= 0 || n > MaxPDFBytes {
			return Job{}, fmt.Errorf("%s must be an integer from 1 to %d, got %q", EnvMaxPDFBytes, int64(MaxPDFBytes), raw)
		}
		job.MaxPDFBytes = n
	}
	return job, nil
}
