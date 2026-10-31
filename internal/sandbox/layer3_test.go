//go:build sandbox

package sandbox

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"
)

// bomb is one resource attack and what must follow it.
type bomb struct {
	env      map[string]string
	markdown string
	// kinds are the structured errors the attack may end in. An empty status
	// set for the attack means transport loss is acceptable too.
	kinds        []string
	status       int
	allowDropped bool
	// beforeJob is true for refusals that happen before a job exists.
	beforeJob bool
	restarts  bool
	limit     time.Duration
}

func (b bomb) timeout() time.Duration {
	if v := b.env["RENDERER_TIMEOUT"]; v != "" {
		d, _ := time.ParseDuration(v)
		return d
	}
	return 60 * time.Second
}

func runBomb(t *testing.T, b bomb) {
	t.Helper()
	t.Parallel()
	requireDocker(t, sandboxImage())
	c := loadCheckedCompose(t)
	w := startWorker(t, c, workerNetwork(t, c), b.env)

	start := time.Now()
	o := w.render(simpleLetter(b.markdown), b.timeout()+requestSlack)
	elapsed := time.Since(start)
	t.Logf("outcome: status %d, error %s, transport %q, %s", o.Status, o.Error, o.TransportError, elapsed.Round(time.Millisecond))

	if elapsed > b.timeout()+requestSlack {
		t.Errorf("the attack ran %s, beyond its limit %s", elapsed, b.timeout())
	}
	switch {
	case o.TransportError != "" && b.allowDropped:
		// The attack starved the Worker itself; recovery is the restart.
	case o.TransportError != "":
		t.Fatalf("no structured answer: %s", o.TransportError)
	default:
		got := o.errorBody()
		if o.Status != b.status || !slices.Contains(b.kinds, got.Kind) || (got.JobID == "") != b.beforeJob {
			t.Errorf("status %d error %s, want %d with kind in %v (job id expected: %v)", o.Status, o.Error, b.status, b.kinds, !b.beforeJob)
		}
	}
	if b.restarts {
		w.waitRestarted()
	} else if n := w.restartCount(); n != 0 {
		t.Errorf("the Worker restarted %d times, want none", n)
	}
	w.assertNextJobRenders()
}

func TestTeXInfiniteLoopEndsAtTheWallClockLimit(t *testing.T) {
	runBomb(t, bomb{
		env: map[string]string{"RENDERER_TIMEOUT": "8s"}, markdown: "probeaction-loop\n",
		status: 504, kinds: []string{"timeout"}, restarts: true,
	})
}

func TestMemoryBombIsReportedAsAResourceError(t *testing.T) {
	runBomb(t, bomb{
		env: map[string]string{"RENDERER_TIMEOUT": "40s"}, markdown: "probeaction-memory\n",
		status: 500, kinds: []string{"resource"}, restarts: true,
	})
}

func TestTmpfsFillIsReportedAsAResourceError(t *testing.T) {
	runBomb(t, bomb{
		env: map[string]string{"RENDERER_TIMEOUT": "40s"}, markdown: "probeaction-fill\n",
		status: 500, kinds: []string{"resource"}, restarts: true,
	})
}

func TestForkBombEndsAndTheWorkerRecovers(t *testing.T) {
	runBomb(t, bomb{
		env: map[string]string{"RENDERER_TIMEOUT": "10s"}, markdown: "probeaction-forkbomb\n",
		status: 504, kinds: []string{"timeout", "resource"}, restarts: true, allowDropped: true,
	})
}

func TestPageBombIsRefusedWithoutRestart(t *testing.T) {
	runBomb(t, bomb{
		env: map[string]string{"RENDERER_MAX_PAGES": "10", "RENDERER_TIMEOUT": "40s"}, markdown: pageBomb(120),
		status: 413, kinds: []string{"pages_exceeded"},
	})
}

// pageBomb is a Document that is long, not hostile: its page count is what
// exceeds the limit.
func pageBomb(paragraphs int) string {
	var doc strings.Builder
	for i := 0; i < paragraphs; i++ {
		fmt.Fprintf(&doc, "# Section %d\n\n%s\n\n", i, strings.Repeat("A long sentence about plants and gardens. ", 30))
	}
	return doc.String()
}

func TestOversizePDFIsRefusedWithoutRestart(t *testing.T) {
	runBomb(t, bomb{
		env: map[string]string{"RENDERER_MAX_PDF_BYTES": "20000"}, markdown: pageBomb(100),
		status: 413, kinds: []string{"pdf_too_large"},
	})
}

// documentBombs are real Documents, not unsafe tools: whatever the pipeline
// makes of them, it must end in time with a structured outcome.
func documentBombs() map[string]string {
	return map[string]string{
		"huge table":        hugeTable(2500, 8),
		"deeply nested":     deepNesting(),
		"giant cell":        giantCell(400_000),
		"hundred thousand":  manyLines(100_000),
		"nested blockquote": strings.Repeat("> ", 300) + "deep\n",
	}
}

func hugeTable(rows, cols int) string {
	var b strings.Builder
	header := "|" + strings.Repeat(" h |", cols) + "\n|" + strings.Repeat("---|", cols) + "\n"
	b.WriteString(header)
	for r := 0; r < rows; r++ {
		b.WriteString("|" + strings.Repeat(fmt.Sprintf(" cell %d |", r), cols) + "\n")
	}
	return b.String()
}

func deepNesting() string {
	var b strings.Builder
	for depth := 0; depth < 150; depth++ {
		b.WriteString(strings.Repeat("  ", depth) + "- level\n")
	}
	return b.String()
}

func giantCell(size int) string {
	return "| a | b |\n|---|---|\n| " + strings.Repeat("word ", size/5) + " | x |\n"
}

// manyLines is a list of n items, one source line each. At 100000 items it
// is still below the 1 MiB Markdown limit.
func manyLines(n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "- %d\n", i)
	}
	return b.String()
}

func TestMarkdownOverTheLimitIsRefused(t *testing.T) {
	runBomb(t, bomb{
		markdown: strings.Repeat("0123456789abcdef\n", 1<<16+1),
		status:   413, kinds: []string{"too_large"}, beforeJob: true,
	})
}

func TestDocumentBombsEndInTimeWithAStructuredOutcome(t *testing.T) {
	t.Parallel()
	requireDocker(t, sandboxImage())
	c := loadCheckedCompose(t)
	for name, markdown := range documentBombs() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			w := startWorker(t, c, workerNetwork(t, c), map[string]string{
				"RENDERER_TIMEOUT": "20s", "RENDERER_MAX_PAGES": "60",
			})
			o := w.render(simpleLetter(markdown), 20*time.Second+requestSlack)
			t.Logf("%s: status %d, error %s, transport %q, %dms", name, o.Status, o.Error, o.TransportError, o.ElapsedMS)
			if o.TransportError != "" {
				t.Fatalf("no structured answer: %s", o.TransportError)
			}
			if o.Status != 200 {
				kinds := []string{"timeout", "pages_exceeded", "tex_error", "resource", "pdf_too_large", "tool_failed"}
				if e := o.errorBody(); !slices.Contains(kinds, e.Kind) || e.JobID == "" {
					t.Errorf("status %d error %s is not a structured limit error", o.Status, o.Error)
				}
			}
			w.waitHealthy()
			w.assertNextJobRenders()
		})
	}
}

// TestHostileJobThroughThePipelineIsContained runs the hostile checks as the
// "lualatex" of a real Worker job, so the job's environment, directory and
// process group are the pipeline's own.
func TestHostileJobThroughThePipelineIsContained(t *testing.T) {
	requireDocker(t, sandboxImage())
	c := loadCheckedCompose(t)
	network := workerNetwork(t, c)
	a := newAdversary(t)
	a.plantJob(t, c, network)
	w := startWorker(t, c, network, nil, hostAlias...)

	p := map[string]any{
		"canary": a.canaryPath, "job_canary": a.jobDir + "/secret", "secret": a.secret,
		"dial": a.dial, "lookup": "example.org", "pids_limit": 128, "skip_fork_bomb": true,
	}
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	o := w.render(simpleLetter("probeaction-checks probeparams-"+hex.EncodeToString(raw)+"\n"), 60*time.Second)
	if o.Status != 200 {
		t.Fatalf("the hostile job did not return its report: status %d %s %s", o.Status, o.TransportError, o.Error)
	}
	for _, leak := range a.leaks() {
		if strings.Contains(o.Body, leak) {
			t.Fatalf("the report contains a canary value %q", leak)
		}
	}
	var report probeReport
	decodeJSON(t, o.Body, &report)
	logReport(t, report)
	expected := slices.DeleteFunc(a.expectedChecks(), func(n string) bool { return n == "fork_bomb" })
	assertContained(t, report, expected)
	assertPipelineFacts(t, report)
	w.assertClean()
}

// pipelineEnvNames is the tool environment the pipeline builds from scratch.
var pipelineEnvNames = []string{
	"PATH", "LC_ALL", "HOME", "TMPDIR", "TEXMFVAR", "TEXMFCACHE", "TEXMFCONFIG", "TEXMFCNF",
	"SOURCE_DATE_EPOCH", "FORCE_SOURCE_DATE",
}

func assertPipelineFacts(t *testing.T, report probeReport) {
	t.Helper()
	for _, name := range strings.Split(report.Info["env_names"], ",") {
		if !slices.Contains(pipelineEnvNames, name) {
			t.Errorf("the job's environment has an unexpected variable %q", name)
		}
	}
	assertErrno(t, report, "read_host_canary", "ENOENT")
	assertErrno(t, report, "read_other_job_dir", "ENOENT")
	assertErrno(t, report, "write:/etc/probe-write", "EROFS")
	assertErrno(t, report, "setuid_root", "EPERM")
	assertInfo(t, report, "uid", "10001")
	assertInfo(t, report, "memory_max", "1073741824")
	assertInfo(t, report, "pids_max", "128")
}
