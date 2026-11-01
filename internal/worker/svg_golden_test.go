//go:build golden

// Tier-2 tests of the SVG phase with the real rsvg-convert: vector output,
// the filter's mapping of SVG references, and the SVG attack corpus fed to
// the Worker directly, past the Gateway pre-check.
package worker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/taskmedia/outprint/internal/svgcorpus"
)

const squareSVG = `<svg xmlns="http://www.w3.org/2000/svg" width="40" height="40"><rect width="40" height="40" fill="#2F6F5E"/></svg>`

func TestSVGBundledRendersAsVector(t *testing.T) {
	job := letterJob(t, copiedTemplateRoot(t), "Before ![A square](images/img-1.svg) and ![](images/img-1.svg){width=20pt height=20pt} after.\n")
	job.Images = map[string][]byte{"img-1.svg": []byte(squareSVG)}

	pdf, err := Render(context.Background(), job)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "out.pdf")
	if err := os.WriteFile(path, pdf, 0o600); err != nil {
		t.Fatal(err)
	}
	listing, err := exec.Command("pdfimages", "-list", path).Output()
	if err != nil {
		t.Fatal(err)
	}
	if rows := strings.Split(strings.TrimSpace(string(listing)), "\n"); len(rows) > 2 {
		t.Errorf("the PDF holds raster images, an SVG must stay a vector:\n%s", listing)
	}
}

func TestSVGFilterMapsToItsConvertedPDFOnly(t *testing.T) {
	image := func(src string) map[string]any {
		return jsonDoc(map[string]any{}, para(map[string]any{"t": "Image", "c": []any{
			[]any{"", []string{}, [][]string{}}, []any{}, []string{src, ""}}}))
	}
	for _, src := range []string{"images/missing.svg", "images/MISSING.SVG", "../images/x.svg", "file:///tmp/x.svg", "images/.svg"} {
		out, err := pandocJSON(t, image(src))
		if kind, ok := filterKind(out); err == nil || !ok || kind != KindImagePath {
			t.Errorf("image %q: kind = %q, err = %v", src, kind, err)
		}
	}
}

func TestSVGGarbageIsAConversionErrorWithoutToolOutput(t *testing.T) {
	job := letterJob(t, copiedTemplateRoot(t), "![x](images/img-1.svg)\n")
	job.Images = map[string][]byte{"img-1.svg": []byte("this is SECRET-WORDS and not an svg")}

	_, err := Render(context.Background(), job)

	we := asWorkerError(t, err)
	if we.Phase != PhaseSVG || we.Kind != KindSVGConvert || strings.Contains(we.Error(), "SECRET-WORDS") {
		t.Fatalf("error = %v", we)
	}
}

func TestSVGWithAnOversizePDFIsRefused(t *testing.T) {
	var nested svgcorpus.Attack
	for _, a := range svgcorpus.Attacks(svgcorpus.Target{CanaryPath: "/none", URL: "http://127.0.0.1:1"}) {
		if a.Name == "nested patterns" {
			nested = a
		}
	}
	job := letterJob(t, copiedTemplateRoot(t), "![x](images/img-1.svg)\n")
	job.Images = map[string][]byte{"img-1.svg": nested.SVG}

	_, err := Render(context.Background(), job)

	we := asWorkerError(t, err)
	if we.Phase != PhaseSVG || we.Kind != KindOutputTooLarge {
		t.Fatalf("error = %v, want phase svg output_too_large", we)
	}
}

// TestSVGConversionLoadsNoLocalFile shows why the SVG goes in through
// standard input: librsvg loads a local picture an SVG names, absolute or
// relative, when it reads the SVG from a file (the control), and none when it
// reads it from a stream (the pipeline).
func TestSVGConversionLoadsNoLocalFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "real.png"), tinyPNG(t), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, href := range []string{"real.png", "file://" + filepath.Join(dir, "real.png"), filepath.Join(dir, "real.png")} {
		svg := fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" width="50" height="50"><image width="50" height="50" xlink:href="%s"/></svg>`, href)
		file := filepath.Join(dir, "t.svg")
		if err := os.WriteFile(file, []byte(svg), 0o644); err != nil {
			t.Fatal(err)
		}
		control, err := exec.Command("rsvg-convert", "-f", "pdf", file).Output()
		if err != nil || rasterImages(t, control) == 0 {
			t.Fatalf("control: reading %q from a file did not load the picture (%v), the check below proves nothing", href, err)
		}

		dst := filepath.Join(dir, "out.pdf")
		p := svgPipeline(dir, 20*time.Second, 1<<20)
		if err := p.convertSVG([]byte(svg), dst); err != nil {
			t.Fatalf("%q: %v", href, err)
		}
		pdf, _ := os.ReadFile(dst)
		if n := rasterImages(t, pdf); n != 0 {
			t.Errorf("%q: the pipeline loaded the local picture (%d raster images)", href, n)
		}
	}
}

func rasterImages(t *testing.T, pdf []byte) int {
	t.Helper()
	path := filepath.Join(t.TempDir(), "x.pdf")
	if err := os.WriteFile(path, pdf, 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("pdfimages", "-list", path).Output()
	if err != nil {
		t.Fatal(err)
	}
	return max(0, len(strings.Split(strings.TrimSpace(string(out)), "\n"))-2)
}

// requestCounter is an HTTP listener that counts every request it sees.
type requestCounter struct {
	url  string
	hits atomic.Int64
}

func newRequestCounter(t *testing.T) *requestCounter {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	c := &requestCounter{url: "http://" + ln.Addr().String()}
	srv := &http.Server{Handler: http.HandlerFunc(func(http.ResponseWriter, *http.Request) { c.hits.Add(1) })}
	go srv.Serve(ln)
	t.Cleanup(func() { srv.Close() })
	return c
}

const attackTimeout = 45 * time.Second

// TestSVGAttackCorpusPastThePrecheck feeds every non-heavy attack of the
// corpus to the Worker pipeline as if the Gateway had let it through. Each
// ends with a PDF or a structured error inside the time limit, never shows
// the content of the canary file or of /etc/passwd, and never makes an HTTP
// request; the next job is clean.
func TestSVGAttackCorpusPastThePrecheck(t *testing.T) {
	canary := "CANARY-" + randomHex(t)
	canaryPath := filepath.Join(t.TempDir(), "canary.txt")
	if err := os.WriteFile(canaryPath, []byte(canary+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	listener := newRequestCounter(t)
	root := copiedTemplateRoot(t)

	t.Run("corpus", func(t *testing.T) {
		for _, a := range svgcorpus.Attacks(svgcorpus.Target{CanaryPath: canaryPath, URL: listener.url}) {
			if a.Heavy {
				continue // memory and time bombs run under the container limits (internal/sandbox)
			}
			t.Run(a.Name, func(t *testing.T) {
				t.Parallel()
				runAttack(t, root, a.SVG, canary)
			})
		}
	})

	if n := listener.hits.Load(); n != 0 {
		t.Errorf("the listener saw %d request(s)", n)
	}
	clean := letterJob(t, root, "A plain letter after the attacks.\n")
	if _, err := Render(context.Background(), clean); err != nil {
		t.Fatalf("the next job failed: %v", err)
	}
}

func runAttack(t *testing.T, root string, svg []byte, canary string) {
	t.Helper()
	job := letterJob(t, root, "Before ![attack](images/img-1.svg) after.\n")
	job.Images = map[string][]byte{"img-1.svg": svg}
	job.Timeout = attackTimeout

	start := time.Now()
	pdf, err := Render(context.Background(), job)
	elapsed := time.Since(start)
	if elapsed > attackTimeout+10*time.Second {
		t.Errorf("the attack ran %s, beyond %s", elapsed, attackTimeout)
	}
	if err != nil {
		var we *Error
		if !errors.As(err, &we) || we.JobID == "" {
			t.Fatalf("not a structured error: %v", err)
		}
		for _, secret := range []string{canary, "root:x:0:0"} {
			if strings.Contains(err.Error(), secret) {
				t.Errorf("the error contains %q", secret)
			}
		}
		t.Logf("ended with %s/%s in %s", we.Phase, we.Kind, elapsed.Round(time.Millisecond))
		return
	}
	text := pdfToText(t, pdf)
	for _, secret := range []string{canary, "root:x:0:0"} {
		if strings.Contains(string(pdf), secret) || strings.Contains(text, secret) {
			t.Errorf("the PDF shows %q", secret)
		}
	}
	t.Logf("rendered in %s", elapsed.Round(time.Millisecond))
}

func pdfToText(t *testing.T, pdf []byte) string {
	t.Helper()
	cmd := exec.Command("pdftotext", "-", "-")
	cmd.Stdin = bytes.NewReader(pdf)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("pdftotext: %v", err)
	}
	return string(out)
}
