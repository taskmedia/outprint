//go:build golden

// Tier-2 tests of the Worker itself. They need Pandoc, LuaLaTeX and the
// shipped packages, so they run inside the outprint image (make test-golden).
// None of them skips: a missing tool is a failure.
package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/taskmedia/outprint/internal/packages"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	if root := os.Getenv("OUTPRINT_REPO_ROOT"); root != "" {
		return root
	}
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("repository root not found")
		}
		dir = parent
	}
}

// templatesRoot is where the shipped Templates are: the image's root, or the
// repository's when run from a checkout.
func templatesRoot(t *testing.T) string {
	t.Helper()
	if root := os.Getenv("OUTPRINT_TEMPLATES_ROOT"); root != "" {
		return root
	}
	return filepath.Join(repoRoot(t), "templates")
}

// copiedTemplateRoot returns a templates root with a copy of the neutral
// Template that the test may modify.
func copiedTemplateRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	src := filepath.Join(templatesRoot(t), "neutral")
	if err := os.Mkdir(filepath.Join(root, "neutral"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := copyTree(src, filepath.Join(root, "neutral"), false); err != nil {
		t.Fatal(err)
	}
	return root
}

func loadTemplate(t *testing.T, root string) packages.Template {
	t.Helper()
	tpl, err := packages.LoadTemplate(root, "neutral")
	if err != nil {
		t.Fatal(err)
	}
	return tpl
}

func letterJob(t *testing.T, root, markdown string) Job {
	t.Helper()
	return Job{
		Markdown:     []byte(markdown),
		Values:       map[string]any{"language": "en"},
		Template:     loadTemplate(t, root),
		DocumentType: "letter",
		WorkRoot:     t.TempDir(),
	}
}

func asWorkerError(t *testing.T, err error) *Error {
	t.Helper()
	var we *Error
	if !errors.As(err, &we) {
		t.Fatalf("error %v is not a *worker.Error", err)
	}
	return we
}

// pandocJSON runs Pandoc with the embedded filters on a document given as
// JSON AST, the only way to produce nodes the Markdown reader never emits.
func pandocJSON(t *testing.T, ast map[string]any) (string, error) {
	t.Helper()
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, imagesDirName), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, filtersDir), 0o700); err != nil {
		t.Fatal(err)
	}
	filters, err := writeFilters(dir)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(ast)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "in.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	args := []string{"--sandbox", "-f", "json", "-t", "latex"}
	for _, f := range filters {
		args = append(args, "--lua-filter", f)
	}
	args = append(args, "in.json")
	res, err := runTool(context.Background(), dir, []string{"PATH=" + toolPath}, "pandoc", args)
	if err != nil {
		return res.Output, err
	}
	return res.Output, nil
}

func jsonDoc(meta map[string]any, blocks ...any) map[string]any {
	return map[string]any{
		"pandoc-api-version": []int{1, 23, 1},
		"meta":               meta,
		"blocks":             blocks,
	}
}

func para(inlines ...any) map[string]any {
	return map[string]any{"t": "Para", "c": inlines}
}

func str(s string) map[string]any { return map[string]any{"t": "Str", "c": s} }

func raw(format, text string) map[string]any {
	return map[string]any{"t": "RawInline", "c": []string{format, text}}
}

func TestFiltersRejectRawTeXAnywhere(t *testing.T) {
	rawTeX := raw("latex", `\input{/etc/passwd}`)
	cases := map[string]map[string]any{
		"inline in the body": jsonDoc(map[string]any{}, para(str("a"), rawTeX)),
		"block in the body": jsonDoc(map[string]any{},
			map[string]any{"t": "RawBlock", "c": []string{"tex", `\directlua{os.execute("x")}`}}),
		"inline in a metadata value": jsonDoc(map[string]any{
			"title": map[string]any{"t": "MetaInlines", "c": []any{rawTeX}},
		}, para(str("body"))),
		"block in a metadata value": jsonDoc(map[string]any{
			"abstract": map[string]any{"t": "MetaBlocks", "c": []any{
				map[string]any{"t": "RawBlock", "c": []string{"latex", `\write18{x}`}}}},
		}, para(str("body"))),
		"nested in a metadata list": jsonDoc(map[string]any{
			"recipient": map[string]any{"t": "MetaList", "c": []any{
				map[string]any{"t": "MetaInlines", "c": []any{rawTeX}}}},
		}, para(str("body"))),
		"inside emphasis": jsonDoc(map[string]any{},
			para(map[string]any{"t": "Emph", "c": []any{rawTeX}})),
	}
	for name, doc := range cases {
		t.Run(name, func(t *testing.T) {
			out, err := pandocJSON(t, doc)
			if err == nil {
				t.Fatalf("raw TeX was accepted:\n%s", out)
			}
			if kind, ok := filterKind(out); !ok || kind != KindRawTeX {
				t.Errorf("kind = %q (%v), want raw_tex; output:\n%s", kind, ok, out)
			}
			if strings.Contains(out, "passwd") || strings.Contains(out, "directlua") {
				t.Errorf("the failure message quotes the payload:\n%s", out)
			}
		})
	}
}

func TestFiltersFailClosedOnMathAndStrayRawFormats(t *testing.T) {
	math := map[string]any{"t": "Math", "c": []any{map[string]any{"t": "InlineMath"}, `\input{x}`}}
	out, err := pandocJSON(t, jsonDoc(map[string]any{}, para(math)))
	if kind, ok := filterKind(out); err == nil || !ok || kind != KindMath {
		t.Errorf("math: kind = %q, err = %v, output:\n%s", kind, err, out)
	}
	out, err = pandocJSON(t, jsonDoc(map[string]any{}, para(raw("openxml", "<w:p/>"))))
	if kind, ok := filterKind(out); err == nil || !ok || kind != KindRawFormat {
		t.Errorf("foreign raw format: kind = %q, err = %v, output:\n%s", kind, err, out)
	}
}

func TestFiltersAcceptOnlyBundledImages(t *testing.T) {
	image := func(src string) map[string]any {
		return jsonDoc(map[string]any{}, para(map[string]any{"t": "Image", "c": []any{
			[]any{"", []string{}, [][]string{}}, []any{}, []string{src, ""}}}))
	}
	for _, src := range []string{"../../etc/passwd", "/etc/passwd", "file:///etc/passwd", "https://example.org/x.png",
		"images/../x", "images/missing.png", "images/.hidden", "x.png", "images/sub/x.png"} {
		out, err := pandocJSON(t, image(src))
		if kind, ok := filterKind(out); err == nil || !ok || kind != KindImagePath {
			t.Errorf("image %q: kind = %q, err = %v", src, kind, err)
		}
	}
}

func TestRenderRejectsPathsOutsideTheBundleBeforeTeXSeesThem(t *testing.T) {
	root := copiedTemplateRoot(t)
	job := letterJob(t, root, "![](../../../etc/passwd)\n")

	_, err := Render(context.Background(), job)

	we := asWorkerError(t, err)
	if we.Phase != PhaseFilter || we.Kind != KindImagePath || we.JobID == "" {
		t.Fatalf("error = %+v", we)
	}
}

func TestBundledImageRenders(t *testing.T) {
	root := copiedTemplateRoot(t)
	job := letterJob(t, root, "Before ![A caption](images/pic-1.png) after.\n")
	job.Images = map[string][]byte{"pic-1.png": tinyPNG(t)}

	pdf, err := Render(context.Background(), job)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(pdf, []byte("%PDF-")) {
		t.Fatalf("not a PDF: %q", pdf[:min(8, len(pdf))])
	}
}

func tinyPNG(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 4, 4))); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// engine runs LuaLaTeX with the Worker's engine setup on hand-written TeX, as
// a hostile or careless Template would. It returns the job directory (kept
// until the test ends), the TeX log and the exit error.
func engine(t *testing.T, texBody string) (jobDir, texLog, pdfText string, err error) {
	t.Helper()
	p := &pipeline{
		job: Job{Template: packages.Template{}},
		id:  "engine-test",
		log: slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)),
		ctx: context.Background(),
	}
	p.dir, err = createJobDir(t.TempDir(), p.id)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.prepare(); err != nil {
		t.Fatal(err)
	}
	doc := "\\documentclass{article}\n\\begin{document}\n" + texBody + "\n\\end{document}\n"
	if err := os.WriteFile(filepath.Join(p.dir, "doc.tex"), []byte(doc), 0o600); err != nil {
		t.Fatal(err)
	}
	res, err := runTool(p.ctx, p.dir, p.env, "lualatex", lualatexArgs("doc.tex"))
	return p.dir, p.readTeXLog(res), extractText(t, filepath.Join(p.dir, "doc.pdf")), err
}

// extractText returns the text of a PDF, or "" if there is none.
func extractText(t *testing.T, pdf string) string {
	t.Helper()
	if _, err := os.Stat(pdf); err != nil {
		return ""
	}
	out, err := exec.Command("pdftotext", pdf, "-").Output()
	if err != nil {
		t.Fatalf("pdftotext: %v (the golden image provides it)", err)
	}
	return string(out)
}

func TestEngineCannotReadOutsideTheJobDirectory(t *testing.T) {
	secretDir := t.TempDir()
	canary := filepath.Join(secretDir, "canary.txt")
	if err := os.WriteFile(canary, []byte("CANARY-"+randomHex(t)), 0o600); err != nil {
		t.Fatal(err)
	}
	want, _ := os.ReadFile(canary)

	payloads := map[string]string{
		"input":           `\input{` + canary + `}`,
		"openin":          `\newread\f \openin\f=` + canary + `\relax \read\f to \x \show\x`,
		"lua io.open":     `\directlua{local f = io.open("` + canary + `", "r") if f then tex.print(f:read("*a")) end}`,
		"lua io.lines":    `\directlua{for l in io.lines("` + canary + `") do tex.print(l) end}`,
		"lua dofile":      `\directlua{dofile("` + canary + `")}`,
		"relative escape": `\input{../` + filepath.Base(secretDir) + `/canary.txt}`,
	}
	for name, payload := range payloads {
		t.Run(name, func(t *testing.T) {
			_, texLog, text, _ := engine(t, payload)
			if strings.Contains(texLog, string(want)) {
				t.Errorf("the TeX log contains the canary")
			}
			if strings.Contains(text, string(want)) {
				t.Errorf("the PDF contains the canary")
			}
		})
	}
}

func TestEngineCannotRunLuaToWriteOrSpawn(t *testing.T) {
	payloads := map[string]func(marker string) string{
		"os.execute": func(m string) string { return `\directlua{os.execute("touch ` + m + `")}` },
		"io.popen": func(m string) string {
			return `\directlua{local p = io.popen("touch ` + m + `") if p then p:close() end}`
		},
		"write18": func(m string) string { return `\immediate\write18{touch ` + m + `}` },
		"lua io.open w": func(m string) string {
			return `\directlua{local f = io.open("` + m + `", "w") if f then f:write("x") f:close() end}`
		},
		"openout": func(m string) string {
			return `\immediate\openout3=` + m + `\relax \immediate\write3{x}\immediate\closeout3`
		},
		"lua os.rename": func(m string) string { return `\directlua{os.rename("doc.tex", "` + m + `")}` },
		"socket": func(m string) string {
			return `\directlua{local ok = pcall(require, "socket") if ok then io.open("` + m + `", "w") end}`
		},
	}
	for name, payload := range payloads {
		t.Run(name, func(t *testing.T) {
			marker := filepath.Join(t.TempDir(), "marker-"+randomHex(t))
			engine(t, payload(marker))
			if _, err := os.Stat(marker); err == nil {
				t.Errorf("the payload created %s", marker)
			}
		})
	}
	t.Run("openout to the parent directory", func(t *testing.T) {
		name := "marker-parent-" + randomHex(t)
		dir, _, _, _ := engine(t, `\immediate\openout3=../`+name+`\relax \immediate\write3{x}\immediate\closeout3`)
		if _, err := os.Stat(filepath.Join(filepath.Dir(dir), name)); err == nil {
			t.Error("the payload wrote outside the job directory")
		}
	})
}

func TestEngineStillReadsItsOwnTreesAndWritesInsideTheJob(t *testing.T) {
	dir, texLog, _, err := engine(t, `Hello \texttt{mono}`)
	if err != nil {
		t.Fatalf("a plain document failed: %v\n%s", err, texLog)
	}
	if _, err := os.Stat(filepath.Join(dir, "doc.pdf")); err != nil {
		t.Error("no PDF in the job directory")
	}
}

func randomHex(t *testing.T) string {
	t.Helper()
	id, err := newJobID()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestTeXFailureReportsStructuredErrorWithoutTheLog(t *testing.T) {
	root := copiedTemplateRoot(t)
	letter := filepath.Join(root, "neutral", "letter.latex")
	data, err := os.ReadFile(letter)
	if err != nil {
		t.Fatal(err)
	}
	broken := strings.Replace(string(data), `\begin{document}`, `\begin{document}\undefinedcommandxyz{CONTENT-MARKER-12345}`, 1)
	if err := os.WriteFile(letter, []byte(broken), 0o644); err != nil {
		t.Fatal(err)
	}
	var logs, debug bytes.Buffer
	job := letterJob(t, root, "Body text with a SECRET-BODY-67890.\n")
	job.Logger = slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))

	_, err = Render(context.Background(), job)

	we := asWorkerError(t, err)
	if we.Phase != PhaseTeX || we.Kind != KindTeXError || we.JobID == "" {
		t.Fatalf("error = %+v", we)
	}
	if !strings.HasPrefix(we.FirstTeXError, "! Undefined control sequence") {
		t.Errorf("first TeX error = %q", we.FirstTeXError)
	}
	for _, text := range []string{we.Error(), logs.String()} {
		for _, secret := range []string{"CONTENT-MARKER-12345", "SECRET-BODY-67890", "undefinedcommandxyz"} {
			if strings.Contains(text, secret) {
				t.Errorf("%q leaks into %q", secret, text)
			}
		}
	}
	for _, want := range []string{we.JobID, "phase=tex", "exit_code=", "duration_ms=", "! Undefined control sequence"} {
		if !strings.Contains(logs.String(), want) {
			t.Errorf("log lacks %q:\n%s", want, logs.String())
		}
	}

	job.DebugTeXLog = &debug
	if _, err := Render(context.Background(), job); err == nil {
		t.Fatal("expected the same failure")
	}
	out := debug.String()
	if !strings.Contains(out, "DEBUG TEX LOG") || !strings.Contains(out, "undefinedcommandxyz") {
		t.Errorf("the debug switch did not print a bannered full log:\n%.600s", out)
	}
}

func TestAtMostTwoTeXRuns(t *testing.T) {
	root := copiedTemplateRoot(t)
	letter := filepath.Join(root, "neutral", "letter.latex")
	data, _ := os.ReadFile(letter)
	always := strings.Replace(string(data), `\begin{document}`,
		`\begin{document}\typeout{LaTeX Warning: Label(s) may have changed. Rerun to get cross-references right.}`, 1)
	if err := os.WriteFile(letter, []byte(always), 0o644); err != nil {
		t.Fatal(err)
	}
	var debug bytes.Buffer
	job := letterJob(t, root, "Text.\n")
	job.DebugTeXLog = &debug

	if _, err := Render(context.Background(), job); err != nil {
		t.Fatal(err)
	}

	if runs := strings.Count(debug.String(), "\njob "); runs != 2 {
		t.Errorf("TeX ran %d times, want exactly 2", runs)
	}
}

func TestTimeoutKillsTheJobAndCleansUp(t *testing.T) {
	root := copiedTemplateRoot(t)
	job := letterJob(t, root, "Text.\n")
	job.Timeout = 300 * time.Millisecond
	work := job.WorkRoot

	start := time.Now()
	_, err := Render(context.Background(), job)

	we := asWorkerError(t, err)
	if we.Phase != PhaseTimeout || we.Kind != KindTimeout {
		t.Fatalf("error = %+v", we)
	}
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Errorf("took %s to give up", elapsed)
	}
	entries, _ := os.ReadDir(work)
	if len(entries) != 0 {
		t.Errorf("the job directory was not removed: %v", entries)
	}
	if survivors := runningTools(t); len(survivors) != 0 {
		t.Errorf("tool processes survive: %v", survivors)
	}
}

// runningTools lists processes of the pipeline's tools, read from /proc.
func runningTools(t *testing.T) []string {
	t.Helper()
	comms, err := filepath.Glob("/proc/[0-9]*/comm")
	if err != nil {
		t.Fatal(err)
	}
	var found []string
	for _, path := range comms {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		switch name := strings.TrimSpace(string(data)); name {
		case "pandoc", "lualatex", "luahbtex", "rsvg-convert":
			found = append(found, name)
		}
	}
	return found
}

func TestJobDirectoryIsRemovedAfterSuccess(t *testing.T) {
	root := copiedTemplateRoot(t)
	job := letterJob(t, root, "Text.\n")

	if _, err := Render(context.Background(), job); err != nil {
		t.Fatal(err)
	}

	if entries, _ := os.ReadDir(job.WorkRoot); len(entries) != 0 {
		t.Errorf("the job directory was not removed: %v", entries)
	}
}

func TestWarmCacheFillsTheCacheAndJobsNeverWriteToIt(t *testing.T) {
	root := copiedTemplateRoot(t)
	cache := t.TempDir()
	if err := WarmCache(context.Background(), letterJob(t, root, "Warm **b** *i*\n"), cache); err != nil {
		t.Fatal(err)
	}
	before := listing(t, cache)
	if !strings.Contains(before, "luatex-cache") {
		t.Fatalf("the cache is empty:\n%s", before)
	}

	job := letterJob(t, root, "Text with `code`.\n")
	job.CacheDir = cache
	if _, err := Render(context.Background(), job); err != nil {
		t.Fatal(err)
	}

	if after := listing(t, cache); after != before {
		t.Errorf("a job changed the shared cache:\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

func listing(t *testing.T, dir string) string {
	t.Helper()
	var b strings.Builder
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		fmt.Fprintf(&b, "%s %d %d\n", path, info.Size(), info.ModTime().UnixNano())
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return b.String()
}
