// Package worker turns a job into a PDF: SVG logo to PDF, Pandoc, LuaLaTeX.
// It runs in a fresh job directory with an environment rebuilt from scratch.
// Document values reach TeX only as Pandoc metadata.
package worker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/taskmedia/outline-latex-pdf/internal/packages"
)

const (
	defaultTimeout  = 2 * time.Minute
	maxOutputTail   = 2000
	maxLualatexRuns = 2
	logoFixedField  = "logo"
	logoJobFile     = "logo.pdf"
	pandocReader    = "markdown-raw_tex-raw_attribute-tex_math_dollars-yaml_metadata_block"
)

// Job is everything the pipeline needs to produce one PDF.
type Job struct {
	Markdown      []byte
	Fields        map[string]any
	Template      packages.Template
	DocumentType  string
	Configuration packages.Configuration
	Timeout       time.Duration
}

// Error reports the failing phase together with the tool output.
type Error struct {
	Phase  string
	Err    error
	Output string
}

func (e *Error) Error() string {
	out := e.Output
	if len(out) > maxOutputTail {
		out = out[len(out)-maxOutputTail:]
	}
	return fmt.Sprintf("%s: %v\n%s", e.Phase, e.Err, out)
}

func (e *Error) Unwrap() error { return e.Err }

// Render runs the pipeline and returns the PDF bytes.
func Render(ctx context.Context, job Job) ([]byte, error) {
	tplFile, err := job.Template.DocumentTypeFile(job.DocumentType)
	if err != nil {
		return nil, err
	}

	timeout := job.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	jobDir, err := os.MkdirTemp("", "render-*")
	if err != nil {
		return nil, fmt.Errorf("create job dir: %w", err)
	}
	defer os.RemoveAll(jobDir)

	env := buildEnv(jobDir, os.LookupEnv)
	if err := prepareDirs(jobDir); err != nil {
		return nil, err
	}
	if err := copyShared(job.Template, jobDir); err != nil {
		return nil, err
	}

	fixed := job.Configuration.Fixed
	if logo, ok := fixed[logoFixedField].(string); ok {
		if err := convertLogo(ctx, job.Configuration, logo, jobDir, env); err != nil {
			return nil, err
		}
		fixed = withValue(fixed, logoFixedField, logoJobFile)
	}

	metaPath := filepath.Join(jobDir, "meta.yaml")
	meta, err := yaml.Marshal(mergeMetadata(job.Configuration.Defaults, job.Fields, fixed))
	if err != nil {
		return nil, fmt.Errorf("encode metadata: %w", err)
	}
	inputPath := filepath.Join(jobDir, "input.md")
	for path, data := range map[string][]byte{metaPath: meta, inputPath: job.Markdown} {
		if err := os.WriteFile(path, data, 0o600); err != nil {
			return nil, fmt.Errorf("write job file: %w", err)
		}
	}

	texPath := filepath.Join(jobDir, "doc.tex")
	if _, err := run(ctx, "pandoc", jobDir, env, "pandoc", pandocArgs(tplFile, metaPath, inputPath, texPath)); err != nil {
		return nil, err
	}
	if err := typeset(ctx, jobDir, env); err != nil {
		return nil, err
	}
	pdf, err := os.ReadFile(filepath.Join(jobDir, "doc.pdf"))
	if err != nil {
		return nil, fmt.Errorf("read pdf: %w", err)
	}
	return pdf, nil
}

func typeset(ctx context.Context, jobDir string, env []string) error {
	for i := 0; i < maxLualatexRuns; i++ {
		out, err := run(ctx, "lualatex", jobDir, env, "lualatex", lualatexArgs("doc.tex"))
		if err != nil {
			return err
		}
		if !needsRerun(out) {
			return nil
		}
	}
	return nil
}

func pandocArgs(templateFile, metadataFile, input, output string) []string {
	return []string{
		"--sandbox",
		"-f", pandocReader,
		"-t", "latex",
		"--template", templateFile,
		"--metadata-file", metadataFile,
		"-o", output,
		input,
	}
}

func lualatexArgs(texFile string) []string {
	return []string{"-interaction=nonstopmode", "-halt-on-error", "-no-shell-escape", texFile}
}

func needsRerun(log string) bool {
	return strings.Contains(log, "Rerun to get") || strings.Contains(log, "Label(s) may have changed")
}

// mergeMetadata layers Configuration Defaults under the Document's values
// and Fixed Values over both.
func mergeMetadata(defaults, document, fixed map[string]any) map[string]any {
	merged := make(map[string]any, len(defaults)+len(document)+len(fixed))
	for _, layer := range []map[string]any{defaults, document, fixed} {
		for k, v := range layer {
			merged[k] = v
		}
	}
	return merged
}

func withValue(m map[string]any, key string, value any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	out[key] = value
	return out
}

// buildEnv returns the only environment child processes see. Nothing is
// inherited except the reproducibility variables.
func buildEnv(jobDir string, lookup func(string) (string, bool)) []string {
	env := []string{
		"PATH=/usr/local/bin:/usr/bin:/bin",
		"LC_ALL=C.UTF-8",
		"HOME=" + filepath.Join(jobDir, "home"),
		"TEXMFVAR=" + filepath.Join(jobDir, "texmf-var"),
		"TEXMFCONFIG=" + filepath.Join(jobDir, "texmf-config"),
		"TMPDIR=" + jobDir,
		"shell_escape=f",
	}
	for _, name := range []string{"SOURCE_DATE_EPOCH", "FORCE_SOURCE_DATE"} {
		if v, ok := lookup(name); ok {
			env = append(env, name+"="+v)
		}
	}
	return env
}

func prepareDirs(jobDir string) error {
	for _, d := range []string{"home", "texmf-var", "texmf-config", "shared"} {
		if err := os.Mkdir(filepath.Join(jobDir, d), 0o700); err != nil {
			return fmt.Errorf("create job dir: %w", err)
		}
	}
	return nil
}

func copyShared(t packages.Template, jobDir string) error {
	names, err := t.SharedFiles()
	if err != nil {
		return err
	}
	for _, name := range names {
		src, err := t.SharedPath(name)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(src)
		if err != nil {
			return fmt.Errorf("read shared file: %w", err)
		}
		if err := os.WriteFile(filepath.Join(jobDir, "shared", name), data, 0o600); err != nil {
			return fmt.Errorf("write shared file: %w", err)
		}
	}
	return nil
}

func convertLogo(ctx context.Context, cfg packages.Configuration, logo, jobDir string, env []string) error {
	src, err := cfg.AssetPath(logo)
	if err != nil {
		return fmt.Errorf("logo: %w", err)
	}
	if !strings.EqualFold(filepath.Ext(src), ".svg") {
		return fmt.Errorf("logo %q: only SVG is supported", logo)
	}
	args := []string{"-f", "pdf", "-o", filepath.Join(jobDir, logoJobFile), src}
	_, err = run(ctx, "rsvg-convert", jobDir, env, "rsvg-convert", args)
	return err
}

// run executes one tool with the given environment and returns its combined
// output. On cancellation the whole process group is killed.
func run(ctx context.Context, phase, dir string, env []string, name string, args []string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Env = env
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			err = errors.Join(err, ctxErr)
		}
		return out.String(), &Error{Phase: phase, Err: err, Output: out.String()}
	}
	return out.String(), nil
}
