package cli

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/taskmedia/outprint/internal/worker"
)

func noEnv(string) string { return "" }

func TestParseRenderOptions(t *testing.T) {
	opts, err := parseRender([]string{
		"--template", "neutral", "--config", "example", "--title", "Spring sale",
		"--date", "2026-03-02", "--json-errors",
		"--templates-root", "/t", "--configs-root", "/c",
		"-o", "out.pdf", "in.md",
	}, noEnv)
	if err != nil {
		t.Fatal(err)
	}
	want := renderOptions{
		Template: "neutral", Config: "example", Title: "Spring sale", Date: "2026-03-02", JSONErrors: true,
		TemplatesRoot: "/t", ConfigsRoot: "/c",
		Output: "out.pdf", Input: "in.md",
		Timeout: 60 * time.Second, LogLevel: slog.LevelWarn, CacheDir: "/opt/outprint/texcache",
	}
	if opts != want {
		t.Fatalf("got %+v, want %+v", opts, want)
	}
}

func TestRootsDefaultAndEnvPrecedence(t *testing.T) {
	args := []string{"--template", "a", "--config", "b", "-o", "o.pdf", "in.md"}

	opts, err := parseRender(args, noEnv)
	if err != nil {
		t.Fatal(err)
	}
	if opts.TemplatesRoot != "./templates" || opts.ConfigsRoot != "./configurations" {
		t.Fatalf("defaults: %+v", opts)
	}

	env := func(k string) string {
		return map[string]string{"OUTPRINT_TEMPLATES_ROOT": "/env/t", "OUTPRINT_CONFIGURATIONS_ROOT": "/env/c"}[k]
	}
	opts, err = parseRender(args, env)
	if err != nil {
		t.Fatal(err)
	}
	if opts.TemplatesRoot != "/env/t" || opts.ConfigsRoot != "/env/c" {
		t.Fatalf("env: %+v", opts)
	}

	opts, err = parseRender(append([]string{"--templates-root", "/flag/t"}, args...), env)
	if err != nil {
		t.Fatal(err)
	}
	if opts.TemplatesRoot != "/flag/t" || opts.ConfigsRoot != "/env/c" {
		t.Fatalf("flag over env: %+v", opts)
	}
}

func TestParseRenderRejectsBadUsage(t *testing.T) {
	cases := map[string][]string{
		"missing template": {"--config", "b", "-o", "o.pdf", "in.md"},
		"missing config":   {"--template", "a", "-o", "o.pdf", "in.md"},
		"missing output":   {"--template", "a", "--config", "b", "in.md"},
		"missing input":    {"--template", "a", "--config", "b", "-o", "o.pdf"},
		"extra input":      {"--template", "a", "--config", "b", "-o", "o.pdf", "in.md", "more.md"},
		"unknown flag":     {"--template", "a", "--config", "b", "-o", "o.pdf", "--bogus", "in.md"},
		"removed --type":   {"--template", "a", "--config", "b", "--type", "letter", "-o", "o.pdf", "in.md"},
		"removed --fields": {"--template", "a", "--config", "b", "--fields", "f.yaml", "-o", "o.pdf", "in.md"},
		"bad date":         {"--template", "a", "--config", "b", "--date", "2 March", "-o", "o.pdf", "in.md"},
		"zero timeout":     {"--template", "a", "--config", "b", "--timeout", "0s", "-o", "o.pdf", "in.md"},
	}
	for name, args := range cases {
		if _, err := parseRender(args, noEnv); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

func TestRunUnknownSubcommand(t *testing.T) {
	var stdout, stderr strings.Builder
	if code := Run([]string{"frobnicate"}, &stdout, &stderr); code != 2 {
		t.Fatalf("exit code %d", code)
	}
	if !strings.Contains(stderr.String(), "usage") {
		t.Fatalf("stderr %q", stderr.String())
	}
	stderr.Reset()
	if code := Run(nil, &stdout, &stderr); code != 2 {
		t.Fatalf("exit code %d", code)
	}
}

const brokenDocument = "```yaml\nsubjct: typo\nname: Somebody\nside: x\n```\nBody\n"

// renderBroken runs render on a Document with metadata problems against the
// shipped packages. It fails before any external tool is needed.
func renderBroken(t *testing.T, extra ...string) (code int, stdout, stderr string, output string) {
	t.Helper()
	dir := t.TempDir()
	input := filepath.Join(dir, "in.md")
	if err := os.WriteFile(input, []byte(brokenDocument), 0o644); err != nil {
		t.Fatal(err)
	}
	output = filepath.Join(dir, "out.pdf")
	args := append([]string{"render", "--template", "neutral", "--config", "example",
		"--templates-root", "../../templates", "--configs-root", "../../configurations"}, extra...)
	args = append(args, "-o", output, input)
	var out, errOut strings.Builder
	code = Run(args, &out, &errOut)
	return code, out.String(), errOut.String(), output
}

func TestRenderPrintsMetadataProblemsReadably(t *testing.T) {
	code, stdout, stderr, output := renderBroken(t)

	if code != 1 {
		t.Fatalf("exit code %d, stderr:\n%s", code, stderr)
	}
	for _, want := range []string{"subjct", "did you mean", "Fixed Value", "document line 2"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr lacks %q:\n%s", want, stderr)
		}
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
	if _, err := os.Stat(output); err == nil {
		t.Error("no PDF may be written on error")
	}
}

func TestRenderJSONErrorsPrintsTheErrorBody(t *testing.T) {
	code, stdout, _, _ := renderBroken(t, "--json-errors")

	if code != 1 {
		t.Fatalf("exit code %d", code)
	}
	var body struct {
		Errors []struct {
			Field, Source, Code, Message string
			Line                         *int
		}
	}
	if err := json.Unmarshal([]byte(stdout), &body); err != nil {
		t.Fatalf("stdout is not the error body: %v\n%s", err, stdout)
	}
	if len(body.Errors) != 3 {
		t.Fatalf("want all 3 problems at once, got %+v", body.Errors)
	}
}

func TestLastModifiedUsesFlagThenFileDate(t *testing.T) {
	if got, err := lastModified(renderOptions{Date: "2026-03-02"}); err != nil || got != "2026-03-02" {
		t.Fatalf("flag: %q %v", got, err)
	}
	path := filepath.Join(t.TempDir(), "in.md")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	when := time.Date(2025, 11, 5, 23, 30, 0, 0, time.UTC)
	if err := os.Chtimes(path, when, when); err != nil {
		t.Fatal(err)
	}
	if got, err := lastModified(renderOptions{Input: path}); err != nil || got != "2025-11-05" {
		t.Fatalf("file: %q %v", got, err)
	}
}

func TestTimeoutAndDebugSwitchesDefaultOffAndComeFromFlagOrEnv(t *testing.T) {
	args := []string{"--template", "a", "--config", "b", "-o", "o.pdf", "in.md"}

	opts, err := parseRender(args, noEnv)
	if err != nil {
		t.Fatal(err)
	}
	if opts.DebugTeXLog || opts.Timeout != 60*time.Second {
		t.Fatalf("defaults: debug=%v timeout=%s", opts.DebugTeXLog, opts.Timeout)
	}

	env := func(k string) string {
		return map[string]string{"OUTPRINT_TIMEOUT": "90s"}[k]
	}
	opts, err = parseRender(args, env)
	if err != nil {
		t.Fatal(err)
	}
	if opts.Timeout != 90*time.Second {
		t.Fatalf("env: %+v", opts)
	}

	opts, err = parseRender(append([]string{"--timeout", "5s"}, args...), env)
	if err != nil {
		t.Fatal(err)
	}
	if opts.Timeout != 5*time.Second {
		t.Fatalf("flag over env: %s", opts.Timeout)
	}

	opts, err = parseRender(append([]string{"--debug-tex-log"}, args...), noEnv)
	if err != nil || !opts.DebugTeXLog {
		t.Fatalf("--debug-tex-log: %+v, %v", opts, err)
	}
}

func TestWorkerErrorsArePrintedReadablyWithoutLogs(t *testing.T) {
	cases := []struct {
		err  *worker.Error
		want string
	}{
		{&worker.Error{Phase: worker.PhaseTimeout, Kind: worker.KindTimeout, JobID: "j1"}, "--timeout"},
		{&worker.Error{Phase: worker.PhaseTeX, Kind: worker.KindTeXError, JobID: "j2", FirstTeXError: "! Hidden."}, "--debug-tex-log"},
		{&worker.Error{Phase: worker.PhaseFilter, Kind: worker.KindRawTeX, JobID: "j3"}, "internal error"},
	}
	for _, c := range cases {
		var stdout, stderr strings.Builder
		reportRenderError(c.err, renderOptions{Timeout: time.Minute}, &stdout, &stderr)
		out := stderr.String()
		if !strings.Contains(out, c.err.JobID) || !strings.Contains(out, string(c.err.Kind)) || !strings.Contains(out, c.want) {
			t.Errorf("output %q lacks job id, kind or %q", out, c.want)
		}
		if strings.Contains(out, "Hidden") {
			t.Errorf("output %q leaks the TeX error line", out)
		}
	}
}
