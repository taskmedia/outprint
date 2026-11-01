//go:build sandbox

package sandbox

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/taskmedia/outprint/internal/bundle"
)

const (
	workerPort    = "8081"
	healthTimeout = 60 * time.Second
	// requestSlack is what a request may take beyond the Worker's own limit:
	// the kill, the response and the process boundary.
	requestSlack = 20 * time.Second
	fixtureDir   = "../../testdata/fixtures/"
)

// outcome mirrors what sandbox-probe post prints.
type outcome struct {
	Status         int             `json:"status"`
	ContentType    string          `json:"content_type"`
	Error          json.RawMessage `json:"error"`
	BodyBytes      int             `json:"body_bytes"`
	Body           string          `json:"body"`
	Text           string          `json:"text"`
	TransportError string          `json:"transport_error"`
	ElapsedMS      int64           `json:"elapsed_ms"`
}

// errorBody is the Worker's structured error.
type errorBody struct {
	Phase string `json:"phase"`
	Kind  string `json:"kind"`
	JobID string `json:"job_id"`
}

func (o outcome) errorBody() errorBody {
	var e errorBody
	_ = json.Unmarshal(o.Error, &e)
	return e
}

// liveWorker is the shipped Worker service, running, plus a client container
// on the same internal network that posts bundles to it.
type liveWorker struct {
	t      *testing.T
	name   string
	client string
}

// startWorker runs the Worker image with the shipped flags, the restart
// policy included, and the given environment on top of the file's own.
func startWorker(t *testing.T, c Compose, network func(string) string, env map[string]string, extra ...string) *liveWorker {
	t.Helper()
	w := &liveWorker{t: t, name: containerName(t, "worker"), client: containerName(t, "client")}
	removeOnCleanup(t, w.name)
	removeOnCleanup(t, w.client)
	args := append([]string{"run", "-d"}, workerFlags(t, c, w.name, network, env, false)...)
	mustDocker(t, append(append(args, extra...), sandboxImage(), "worker")...)
	mustDocker(t, "run", "-d", "--name", w.client, "--label", runLabel+"="+runID, "--network", network("worker-net"),
		"--entrypoint", "sandbox-probe", sandboxImage(), "hold")
	w.waitHealthy()
	return w
}

func (w *liveWorker) url(path string) string { return "http://" + w.name + ":" + workerPort + path }

// exec runs a command in a container, with optional standard input.
func execIn(t *testing.T, stdin []byte, args ...string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), dockerTimeout)
	defer cancel()
	var out bytes.Buffer
	cmd := exec.CommandContext(ctx, "docker", append([]string{"exec", "-i"}, args...)...)
	cmd.Stdin = bytes.NewReader(stdin)
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	return out.String(), err
}

func (w *liveWorker) request(stdin []byte, timeout time.Duration, probeArgs ...string) outcome {
	w.t.Helper()
	args := append([]string{w.client, "sandbox-probe", "post", "-timeout", timeout.String()}, probeArgs...)
	out, err := execIn(w.t, stdin, args...)
	if err != nil {
		w.t.Fatalf("docker exec post: %v\n%s", err, out)
	}
	var o outcome
	decodeJSON(w.t, strings.TrimSpace(out), &o)
	return o
}

func (w *liveWorker) render(b bundle.Bundle, timeout time.Duration) outcome {
	w.t.Helper()
	data, err := b.Encode()
	if err != nil {
		w.t.Fatal(err)
	}
	return w.request(data, timeout, "-pdftotext", w.url("/render"))
}

// waitHealthy polls /healthz until the Worker answers, which after a restart
// is the new process.
func (w *liveWorker) waitHealthy() time.Duration {
	w.t.Helper()
	start := time.Now()
	for time.Since(start) < healthTimeout {
		if o := w.request(nil, 3*time.Second, "-get", w.url("/healthz")); o.Status == 200 {
			return time.Since(start)
		}
		time.Sleep(500 * time.Millisecond)
	}
	w.t.Fatalf("the Worker was not healthy again within %s", healthTimeout)
	return 0
}

func (w *liveWorker) restartCount() int {
	w.t.Helper()
	n, err := strconv.Atoi(mustDocker(w.t, "inspect", "-f", "{{.RestartCount}}", w.name))
	if err != nil {
		w.t.Fatal(err)
	}
	return n
}

// waitRestarted waits for the container runtime to have restarted the Worker
// at least once and for the new process to be healthy.
func (w *liveWorker) waitRestarted() {
	w.t.Helper()
	deadline := time.Now().Add(healthTimeout)
	for w.restartCount() < 1 {
		if time.Now().After(deadline) {
			w.t.Fatal("the Worker was not restarted")
		}
		time.Sleep(300 * time.Millisecond)
	}
	w.waitHealthy()
}

func (w *liveWorker) listJSON(args ...string) []string {
	w.t.Helper()
	out, err := execIn(w.t, nil, append([]string{w.name, "sandbox-probe"}, args...)...)
	if err != nil {
		w.t.Fatalf("docker exec %v: %v\n%s", args, err, out)
	}
	var names []string
	decodeJSON(w.t, strings.TrimSpace(out), &names)
	return names
}

// assertClean checks the container holds no job directory and no process
// besides the Worker (and the inspection command itself).
func (w *liveWorker) assertClean() {
	w.t.Helper()
	if left := w.listJSON("ls", "/work"); len(left) != 0 {
		w.t.Errorf("the job directory is not clean: %v", left)
	}
	for _, name := range w.listJSON("procs") {
		if name != "renderer" && name != "sandbox-probe" {
			w.t.Errorf("a process is left over: %s", name)
		}
	}
}

// assertNextJobRenders renders the golden fixture and checks the text.
func (w *liveWorker) assertNextJobRenders() {
	w.t.Helper()
	w.waitHealthy()
	o := w.render(fixtureBundle(w.t, "letter-basic"), 120*time.Second)
	if o.Status != 200 {
		w.t.Fatalf("the next job failed: status %d, %s %s", o.Status, o.TransportError, o.Error)
	}
	w.t.Logf("next job: %d bytes in %dms", o.BodyBytes, o.ElapsedMS)
	for _, want := range []string{"Thank you for your enquiry", "Spring plant sale", "Ada Example"} {
		if !strings.Contains(o.Text, want) {
			w.t.Errorf("the next job's PDF lacks %q:\n%s", want, o.Text)
		}
	}
	w.assertClean()
}

// fixtureBundle builds a bundle from a shared fixture: the body after its
// Metadata Block, with the block's values.
func fixtureBundle(t *testing.T, name string) bundle.Bundle {
	t.Helper()
	data, err := os.ReadFile(fixtureDir + name + "/input.md")
	if err != nil {
		t.Fatal(err)
	}
	_, body, found := strings.Cut(string(data), "\n```\n")
	if !found {
		t.Fatalf("fixture %s has no metadata block", name)
	}
	return letter(body, map[string]any{
		"date": "2 March 2026", "subject": "Spring plant sale",
		"recipient": []string{"Ada Example", "1 Sample Lane", "12345 Exampleton"},
	})
}

func letter(markdown string, values map[string]any) bundle.Bundle {
	return bundle.Bundle{
		Markdown: []byte(markdown), Values: values,
		Template: "neutral", DocumentType: "letter", Configuration: "example",
	}
}

func simpleLetter(markdown string) bundle.Bundle {
	return letter(markdown, map[string]any{"subject": "Test", "recipient": []string{"Ada Example"}})
}
