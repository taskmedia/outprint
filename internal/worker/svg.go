package worker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/taskmedia/outprint/internal/limits"
)

const (
	svgExt        = ".svg"
	pdfExt        = ".pdf"
	pdfMagic      = "%PDF"
	maxLoggedLine = 120
)

var errSVGOutputTooLarge = errors.New("the converted PDF is over the size limit")

// svgPhase is the first phase of a job. It converts every SVG of the bundle
// and the Configuration's logo to a vector PDF with rsvg-convert:
// images/<name>.svg becomes images/<name>.pdf, which the Lua filter
// substitutes for the SVG reference, and the logo becomes logo.pdf.
//
// The SVG goes in through standard input. rsvg-convert then has no base file,
// so it can load nothing but data: URIs, whatever the SVG asks for, and the
// PDF comes out through standard output, where its size is bounded as it is
// read.
func (p *pipeline) svgPhase() error {
	for _, name := range svgNames(p.job.Images) {
		if err := p.convertBundleSVG(name); err != nil {
			return err
		}
	}
	if logo, ok := p.job.Values[logoField].(string); ok {
		return p.convertLogo(logo)
	}
	return nil
}

func svgNames(images map[string][]byte) []string {
	var names []string
	for name := range images {
		if isSVGName(name) {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

func isSVGName(name string) bool { return strings.EqualFold(filepath.Ext(name), svgExt) }

func pdfNameFor(svgName string) string {
	return strings.TrimSuffix(svgName, filepath.Ext(svgName)) + pdfExt
}

func (p *pipeline) convertBundleSVG(name string) error {
	pdfName := pdfNameFor(name)
	if _, clash := p.job.Images[pdfName]; clash {
		return p.fail(PhaseSVG, KindInvalidJob, fmt.Errorf("image %q and %q have the same converted name", name, pdfName))
	}
	return p.convertSVG(p.job.Images[name], filepath.Join(p.dir, imagesDirName, pdfName))
}

func (p *pipeline) convertLogo(logo string) error {
	src, err := p.job.Configuration.AssetPath(logo)
	if err != nil {
		return p.fail(PhaseSVG, KindInvalidJob, fmt.Errorf("logo: %w", err))
	}
	if !strings.EqualFold(filepath.Ext(src), svgExt) {
		return p.fail(PhaseSVG, KindInvalidJob, fmt.Errorf("logo %q: only SVG is supported", logo))
	}
	data, err := readLimited(src, limits.MaxSVGBytes)
	if err != nil {
		return p.fail(PhaseSVG, KindInvalidJob, fmt.Errorf("logo %q: %w", logo, err))
	}
	return p.convertSVG(data, filepath.Join(p.dir, logoJobFile))
}

func readLimited(path string, max int) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, int64(max)+1))
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}
	if len(data) > max {
		return nil, fmt.Errorf("larger than %d bytes", max)
	}
	return data, nil
}

// convertSVG runs one conversion and writes the PDF to dst.
func (p *pipeline) convertSVG(svg []byte, dst string) error {
	if len(svg) > limits.MaxSVGBytes {
		return p.fail(PhaseSVG, KindInvalidJob, fmt.Errorf("an SVG is %d bytes, at most %d", len(svg), limits.MaxSVGBytes))
	}
	ctx, cancel := context.WithTimeout(p.ctx, p.svgTimeout)
	defer cancel()
	out := &limitedBuffer{max: p.svgMaxPDF, onExceed: cancel}
	var stderr cappedBuffer
	res, err := runToolIO(ctx, p.dir, buildSVGEnv(p.dir, os.LookupEnv), "rsvg-convert", rsvgArgs(), bytes.NewReader(svg), out, &stderr)
	if err != nil || !bytes.HasPrefix(out.buf.Bytes(), []byte(pdfMagic)) {
		return p.svgFailure(ctx, res, out, firstLine(stderr.buf.String()))
	}
	if err := os.WriteFile(dst, out.buf.Bytes(), 0o600); err != nil {
		return p.fail(PhaseSVG, KindInternal, fmt.Errorf("write converted svg: %w", err))
	}
	p.logPhase(slog.LevelDebug, PhaseSVG, res, "")
	return nil
}

// rsvgArgs reads the SVG from standard input and writes a PDF to standard
// output. --unlimited is never passed: librsvg keeps its limits on XML size
// and nesting.
func rsvgArgs() []string { return []string{"-f", "pdf", "-"} }

// svgFailure maps the end of a failed conversion to the structured error.
// Tool output is never part of the error; its first line goes to the log.
func (p *pipeline) svgFailure(ctx context.Context, res toolResult, out *limitedBuffer, stderrLine string) error {
	switch {
	case out.exceeded:
		p.log.Warn("svg conversion failed", "job", p.id, "phase", PhaseSVG, "kind", KindOutputTooLarge)
		return &Error{Phase: PhaseSVG, Kind: KindOutputTooLarge, JobID: p.id, Err: errSVGOutputTooLarge}
	case p.ctx.Err() != nil:
		return p.timeoutError(PhaseSVG, res)
	case ctx.Err() != nil:
		p.log.Warn("svg conversion failed", "job", p.id, "phase", PhaseSVG, "kind", KindTimeout)
		return &Error{Phase: PhaseSVG, Kind: KindTimeout, JobID: p.id,
			Err: fmt.Errorf("an SVG took longer than %s to convert", p.svgTimeout)}
	case p.outOfResources(res):
		p.logPhase(slog.LevelWarn, PhaseSVG, res, "")
		return &Error{Phase: PhaseSVG, Kind: KindResource, JobID: p.id, Err: errors.New("the job ran out of memory or scratch space")}
	}
	p.log.Warn("svg conversion failed", "job", p.id, "phase", PhaseSVG, "kind", KindSVGConvert,
		"exit_code", res.ExitCode, "tool_error", stderrLine)
	return &Error{Phase: PhaseSVG, Kind: KindSVGConvert, JobID: p.id, Err: fmt.Errorf("rsvg-convert exited with code %d", res.ExitCode)}
}

// firstLine is the first line of tool output reduced to printable ASCII and
// cut short, so that a log line cannot carry much of anything.
func firstLine(s string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	var b strings.Builder
	for _, r := range line {
		if b.Len() >= maxLoggedLine {
			break
		}
		if r >= ' ' && r < 0x7f {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// limitedBuffer keeps what is written to it up to max bytes. The write that
// would pass the limit fails, which ends the tool's output, and calls
// onExceed, which kills the tool.
type limitedBuffer struct {
	buf      bytes.Buffer
	max      int
	exceeded bool
	onExceed func()
}

func (l *limitedBuffer) Write(p []byte) (int, error) {
	if l.buf.Len()+len(p) > l.max {
		l.exceeded = true
		l.onExceed()
		return 0, errSVGOutputTooLarge
	}
	return l.buf.Write(p)
}
