// Package worker turns a job into a PDF: SVG images and logo to PDF, Pandoc with Lua
// filters, LuaLaTeX. Every job runs in a fresh job directory with an
// environment rebuilt from scratch and an engine configuration of the
// Worker's own. Document values reach TeX only as Pandoc metadata.
package worker

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/taskmedia/outprint/internal/limits"
	"github.com/taskmedia/outprint/internal/packages"
)

const (
	// DefaultTimeout is the wall-clock limit of one job.
	DefaultTimeout  = limits.WallClock
	maxLualatexRuns = limits.MaxTeXRuns
	logoField       = "logo"
	logoJobFile     = "logo.pdf"
	pandocReader    = "markdown-raw_tex-raw_attribute-tex_math_dollars-yaml_metadata_block"
	texJobName      = "doc"
	filterMarker    = "renderer-filter:"
)

// Job is everything the pipeline needs to produce one PDF. Markdown is the
// Document body without its Metadata Block; Values are the merged, typed
// field values of the Document Type (see package metadata).
type Job struct {
	Markdown      []byte
	Values        map[string]any
	Template      packages.Template
	DocumentType  string
	Configuration packages.Configuration
	// Images are the canonical image files of the Document by bundle name.
	// Markdown may refer to them as images/<name> and to nothing else.
	Images map[string][]byte

	// Timeout is the wall-clock limit for the whole job; zero means
	// DefaultTimeout.
	Timeout time.Duration
	// MaxPages and MaxPDFBytes limit the result; zero means the starting
	// limits of package limits.
	MaxPages    int
	MaxPDFBytes int64
	// WorkRoot is where the job directory is created. A tmpfs in production.
	// Empty means the system temporary directory.
	WorkRoot string
	// CacheDir is a pre-built luaotfload cache. It is copied into the job's
	// own cache directory; the original is never written to. Ignored if it
	// does not exist.
	CacheDir string
	// Logger receives job, phase, duration, exit code and the first TeX error
	// line, never Document content. Nil logs warnings to stderr.
	Logger *slog.Logger
	// DebugTeXLog, if set, receives the full TeX log after a loud warning.
	// The log can contain Document content. Never set it by default.
	DebugTeXLog io.Writer
}

// Render runs the pipeline and returns the PDF bytes.
func Render(ctx context.Context, job Job) ([]byte, error) {
	var pdf []byte
	err := runJob(ctx, job, nil, &pdf)
	return pdf, err
}

// runJob is Render with a hook that sees the finished job directory before
// it is removed.
func runJob(ctx context.Context, job Job, afterTeX func(jobDir string) error, pdf *[]byte) error {
	id, err := newJobID()
	if err != nil {
		return &Error{Phase: PhaseSetup, Kind: KindInternal, JobID: "unknown", Err: err}
	}
	p := &pipeline{job: job, id: id, log: job.Logger, afterTeX: afterTeX,
		svgTimeout: limits.SVGConvertTimeout, svgMaxPDF: limits.MaxSVGPDFBytes}
	if p.log == nil {
		p.log = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	}
	timeout := job.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	var cancel context.CancelFunc
	p.ctx, cancel = context.WithTimeout(ctx, timeout)
	defer cancel()
	p.timeout = timeout

	out, err := p.run()
	if err != nil {
		return err
	}
	*pdf = out
	return nil
}

type pipeline struct {
	job      Job
	id       string
	ctx      context.Context
	timeout  time.Duration
	log      *slog.Logger
	dir      string
	env      []string
	afterTeX func(jobDir string) error
	// svgTimeout and svgMaxPDF bound one SVG conversion.
	svgTimeout time.Duration
	svgMaxPDF  int
}

func (p *pipeline) run() ([]byte, error) {
	tplFile, err := p.job.Template.DocumentTypeFile(p.job.DocumentType)
	if err != nil {
		return nil, p.fail(PhaseSetup, KindInvalidJob, err)
	}
	p.dir, err = createJobDir(p.job.WorkRoot, p.id)
	if err != nil {
		return nil, p.fail(PhaseSetup, KindInternal, err)
	}
	defer os.RemoveAll(p.dir)

	if err := p.prepare(); err != nil {
		return nil, p.fail(PhaseSetup, KindInternal, err)
	}
	if err := p.svgPhase(); err != nil {
		return nil, err
	}
	values, err := p.prepareValues()
	if err != nil {
		return nil, err
	}
	if err := p.pandoc(tplFile, values); err != nil {
		return nil, err
	}
	if err := p.typeset(); err != nil {
		return nil, err
	}
	if err := p.checkOutput(); err != nil {
		return nil, err
	}
	if p.afterTeX != nil {
		if err := p.afterTeX(p.dir); err != nil {
			return nil, p.fail(PhaseSetup, KindInternal, err)
		}
	}
	pdf, err := os.ReadFile(filepath.Join(p.dir, texJobName+".pdf"))
	if err != nil {
		return nil, p.fail(PhaseTeX, KindNoOutput, errors.New("no PDF was produced"))
	}
	return pdf, nil
}

// checkOutput enforces the page and PDF size limits before the PDF is read
// into memory. LuaLaTeX has to finish before either is known, so the
// wall-clock limit is what bounds a job that makes pages endlessly.
func (p *pipeline) checkOutput() error {
	max := limits.Job{MaxPages: p.job.MaxPages, MaxPDFBytes: p.job.MaxPDFBytes}.WithDefaults()
	info, err := os.Stat(filepath.Join(p.dir, texJobName+".pdf"))
	if err != nil {
		return p.fail(PhaseTeX, KindNoOutput, errors.New("no PDF was produced"))
	}
	pages, ok := pagesFromTeXLog(p.readTeXLog(toolResult{}))
	if !ok {
		return p.fail(PhaseTeX, KindNoOutput, errors.New("the TeX log does not report a page count"))
	}
	if pages > max.MaxPages {
		return p.fail(PhaseOutput, KindPagesExceeded, fmt.Errorf("%d pages, at most %d", pages, max.MaxPages))
	}
	if info.Size() > max.MaxPDFBytes {
		return p.fail(PhaseOutput, KindPDFTooLarge, fmt.Errorf("%d bytes, at most %d", info.Size(), max.MaxPDFBytes))
	}
	return nil
}

func (p *pipeline) prepare() error {
	if err := prepareDirs(p.dir); err != nil {
		return err
	}
	install, err := discoverInstall(p.ctx, p.dir)
	if err != nil {
		return err
	}
	cnf, err := engineSetup(p.dir, install)
	if err != nil {
		return err
	}
	p.env = buildEnv(p.dir, cnf, os.LookupEnv)

	steps := []func() error{
		func() error { return copyShared(p.job.Template, p.dir) },
		func() error { return copyTemplateFonts(p.job.Template, p.dir) },
		func() error { return writeImages(p.dir, p.job.Images) },
		p.copyCache,
	}
	for _, step := range steps {
		if err := step(); err != nil {
			return err
		}
	}
	return nil
}

func (p *pipeline) copyCache() error {
	if p.job.CacheDir == "" || !dirExists(p.job.CacheDir) {
		return nil
	}
	return copyTree(p.job.CacheDir, filepath.Join(p.dir, texmfVarDir), false)
}

// prepareValues points the logo at its converted PDF and formats dates.
func (p *pipeline) prepareValues() (map[string]any, error) {
	values := p.job.Values
	if _, ok := values[logoField].(string); ok {
		values = withValue(values, logoField, logoJobFile)
	}
	values, err := formatDates(p.job.Template, values)
	if err != nil {
		return nil, p.fail(PhaseSetup, KindInvalidJob, err)
	}
	return values, nil
}

func (p *pipeline) pandoc(templateFile string, values map[string]any) error {
	meta, err := pandocMetadata(p.job.Template, values)
	if err != nil {
		return p.fail(PhaseSetup, KindInvalidJob, fmt.Errorf("encode metadata: %w", err))
	}
	metaPath := filepath.Join(p.dir, "meta.yaml")
	inputPath := filepath.Join(p.dir, "input.md")
	for path, data := range map[string][]byte{metaPath: meta, inputPath: p.job.Markdown} {
		if err := os.WriteFile(path, data, 0o600); err != nil {
			return p.fail(PhaseSetup, KindInternal, fmt.Errorf("write job file: %w", err))
		}
	}
	filters, err := writeFilters(p.dir)
	if err != nil {
		return p.fail(PhaseSetup, KindInternal, err)
	}
	texPath := filepath.Join(p.dir, texJobName+".tex")
	res, err := runTool(p.ctx, p.dir, p.env, "pandoc", pandocArgs(templateFile, metaPath, inputPath, texPath, filters))
	if err != nil {
		return p.pandocFailure(res, err)
	}
	p.logPhase(slog.LevelDebug, PhasePandoc, res, "")
	return nil
}

// pandocFailure tells a deliberate filter failure from any other Pandoc
// failure. Only the failure kind is taken from the output.
func (p *pipeline) pandocFailure(res toolResult, err error) error {
	kind, ok := filterKind(res.Output)
	if !ok {
		return p.toolError(PhasePandoc, res, err)
	}
	p.logPhase(slog.LevelWarn, PhaseFilter, res, "")
	return &Error{Phase: PhaseFilter, Kind: kind, JobID: p.id, Err: errors.New("a filter rejected the document")}
}

func filterKind(output string) (Kind, bool) {
	_, rest, found := strings.Cut(output, filterMarker)
	if !found {
		return "", false
	}
	name, _, _ := strings.Cut(rest, ":")
	switch Kind(name) {
	case KindRawTeX, KindRawFormat, KindMath, KindImagePath:
		return Kind(name), true
	}
	return KindInternal, true
}

func (p *pipeline) typeset() error {
	for run := 1; run <= maxLualatexRuns; run++ {
		res, err := runTool(p.ctx, p.dir, p.env, "lualatex", lualatexArgs(texJobName+".tex"))
		texLog := p.readTeXLog(res)
		p.debugLog(texLog)
		if err != nil {
			return p.texFailure(res, texLog, err)
		}
		p.logPhase(slog.LevelDebug, PhaseTeX, res, "")
		if !needsRerun(texLog) {
			return nil
		}
	}
	return nil
}

func (p *pipeline) readTeXLog(res toolResult) string {
	data, err := os.ReadFile(filepath.Join(p.dir, texJobName+".log"))
	if err != nil {
		return res.Output
	}
	return string(data)
}

func (p *pipeline) texFailure(res toolResult, texLog string, err error) error {
	first := firstTeXErrorLine(texLog)
	if first == "" {
		first = firstTeXErrorLine(res.Output)
	}
	if p.ctx.Err() != nil {
		return p.toolError(PhaseTeX, res, err)
	}
	p.logPhase(slog.LevelWarn, PhaseTeX, res, first)
	if p.outOfResources(res) {
		return &Error{Phase: PhaseTeX, Kind: KindResource, JobID: p.id,
			Err: errors.New("the job ran out of memory or scratch space"), FirstTeXError: first}
	}
	return &Error{Phase: PhaseTeX, Kind: KindTeXError, JobID: p.id,
		Err: fmt.Errorf("lualatex exited with code %d", res.ExitCode), FirstTeXError: first}
}

// debugLog prints the full TeX log to the debug writer, behind a banner.
func (p *pipeline) debugLog(texLog string) {
	if p.job.DebugTeXLog == nil {
		return
	}
	fmt.Fprintf(p.job.DebugTeXLog, "%s\njob %s\n%s\n%s\n%s\n", debugBanner, p.id, debugBanner, texLog, debugBanner)
}

const debugBanner = "!!!!!!!! DEBUG TEX LOG: MAY CONTAIN DOCUMENT CONTENT. NEVER ENABLE IN PRODUCTION. !!!!!!!!"

// tool runs a tool whose output is of no interest beyond success or failure.
func (p *pipeline) tool(phase Phase, name string, args []string) error {
	res, err := runTool(p.ctx, p.dir, p.env, name, args)
	if err != nil {
		return p.toolError(phase, res, err)
	}
	p.logPhase(slog.LevelDebug, phase, res, "")
	return nil
}

// toolError reports a failed tool. A job that ran out of time is reported
// as a timeout whichever tool was running.
func (p *pipeline) toolError(phase Phase, res toolResult, err error) error {
	if res.TimedOut {
		return p.timeoutError(phase, res)
	}
	p.logPhase(slog.LevelWarn, phase, res, "")
	if p.outOfResources(res) {
		return &Error{Phase: phase, Kind: KindResource, JobID: p.id, Err: errors.New("the job ran out of memory or scratch space")}
	}
	return &Error{Phase: phase, Kind: KindToolFailed, JobID: p.id,
		Err: fmt.Errorf("exit code %d", res.ExitCode)}
}

func (p *pipeline) timeoutError(during Phase, res toolResult) error {
	p.logPhase(slog.LevelWarn, PhaseTimeout, res, "")
	return &Error{Phase: PhaseTimeout, Kind: KindTimeout, JobID: p.id,
		Err: fmt.Errorf("exceeded %s while in phase %s", p.timeout, during)}
}

// fail builds an error for a problem found outside a tool run.
func (p *pipeline) fail(phase Phase, kind Kind, cause error) error {
	if p.ctx != nil && p.ctx.Err() != nil {
		return p.timeoutError(phase, toolResult{ExitCode: exitCodeUnknown})
	}
	if errors.Is(cause, syscall.ENOSPC) {
		kind = KindResource
	}
	p.log.Warn("job failed", "job", p.id, "phase", phase, "kind", kind)
	return &Error{Phase: phase, Kind: kind, JobID: p.id, Err: cause}
}

func (p *pipeline) logPhase(level slog.Level, phase Phase, res toolResult, firstTeXError string) {
	attrs := []any{"job", p.id, "phase", phase, "duration_ms", res.Duration.Milliseconds(), "exit_code", res.ExitCode}
	if firstTeXError != "" {
		attrs = append(attrs, "tex_error", firstTeXError)
	}
	p.log.Log(context.Background(), level, "phase finished", attrs...)
}

func pandocArgs(templateFile, metadataFile, input, output string, filters []string) []string {
	args := []string{
		"--sandbox",
		"-f", pandocReader,
		"-t", "latex",
		"--template", templateFile,
		"--metadata-file", metadataFile,
	}
	for _, f := range filters {
		args = append(args, "--lua-filter", f)
	}
	return append(args, "-o", output, input)
}

func lualatexArgs(texFile string) []string {
	return []string{"-interaction=nonstopmode", "-halt-on-error", "-no-shell-escape", texFile}
}

func withValue(m map[string]any, key string, value any) map[string]any {
	out := maps.Clone(m)
	out[key] = value
	return out
}
