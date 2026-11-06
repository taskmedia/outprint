// Package limits is the one place that names the renderer's resource limits.
// The application enforces the per-job limits (time, pages, PDF size, TeX
// runs); the container limits are enforced by the container runtime and are
// listed here so the Compose file and the security tests agree with them.
package limits

import (
	"fmt"
	"strconv"
	"time"

	"github.com/taskmedia/outprint/internal/settings"
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

// Image limits, enforced by the Gateway while it fetches and canonicalises
// the Document's images; the bundle checks them again.
const (
	// MaxImages is the number of distinct images in one Document.
	MaxImages = 50
	// MaxImageBytes is the size of one fetched image and of one canonical image.
	MaxImageBytes = 10 << 20
	// MaxImagesTotal is the size of all canonical images of a Document.
	MaxImagesTotal = 50 << 20
	// MaxImagePixels is the largest image the Gateway decodes, checked on
	// the header before any pixel is decoded.
	MaxImagePixels = 40_000_000
	// MaxImageRedirects is how many redirects the image fetch follows.
	MaxImageRedirects = 5
)

// SVG limits. The Gateway pre-checks the source; the Worker bounds the
// conversion.
const (
	// MaxSVGBytes is the size of one SVG source.
	MaxSVGBytes = 2 << 20
	// MaxSVGElements and MaxSVGDepth bound the structure of an SVG.
	MaxSVGElements = 100_000
	// MaxSVGDepth is the nesting librsvg renders: deeper fails the conversion.
	MaxSVGDepth = 50
	// MaxSVGExpansion is how many elements an SVG may instantiate once every
	// <use> is expanded, which is what stops an exponential <use> bomb.
	MaxSVGExpansion = 500_000
	// MaxSVGSizePx is the largest natural width or height in CSS pixels;
	// MaxSVGViewBox is the largest viewBox side in user units.
	MaxSVGSizePx  = 100_000
	MaxSVGViewBox = 1_000_000
	// SVGConvertTimeout is how long one SVG may take to convert, and
	// MaxSVGPDFBytes is the largest PDF one conversion may produce.
	SVGConvertTimeout = 20 * time.Second
	MaxSVGPDFBytes    = 10 << 20
)

// Gateway image phase and status lookups.
const (
	// BundleTimeout caps building one bundle: every image fetch, decode and
	// rewrite of a request together.
	BundleTimeout = 3 * time.Minute
	// ImageConcurrency is how many requests may run the image phase at once.
	ImageConcurrency = 2
	// StatusTTL is how long a status lookup of the user's language is reused;
	// StatusDeniedTTL is how long a refused session is remembered.
	StatusTTL       = 60 * time.Second
	StatusDeniedTTL = 5 * time.Second
)

// Admission control, enforced by the Gateway. The starting values match one
// Worker replica; scale them together with the replicas.
const (
	// AdmissionSlots is how many renders run at once.
	AdmissionSlots = 1
	// AdmissionQueueDepth is how many requests wait for a slot.
	AdmissionQueueDepth = 4
	// AdmissionPerUser is how many renders one user may have, queued and
	// running together.
	AdmissionPerUser = 1
	// AdmissionMaxWait is how long a queued request waits for a slot.
	AdmissionMaxWait = 60 * time.Second
	// AdmissionAssumedRender seeds the Retry-After estimate until real
	// render times are known.
	AdmissionAssumedRender = 10 * time.Second
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
	EnvTimeout     = settings.EnvTimeout
	EnvMaxPages    = settings.EnvMaxPages
	EnvMaxPDFBytes = settings.EnvMaxPDFBytes
)

// Environment variables that override the admission keys of the service
// configuration (slots, queueDepth, perUser, maxWait).
const (
	EnvAdmissionSlots      = settings.EnvAdmissionSlots
	EnvAdmissionQueueDepth = settings.EnvAdmissionQueueDepth
	EnvAdmissionPerUser    = settings.EnvAdmissionPerUser
	EnvAdmissionMaxWait    = settings.EnvAdmissionMaxWait
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
