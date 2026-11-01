//go:build sandbox

// Security layers 2 and 3: the Worker image, run with the container flags of
// the shipped Compose file, against a hostile pipeline. Nothing here skips: a
// missing Docker daemon or image is a failure, because a skipped security test
// proves nothing.
package sandbox

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

const (
	defaultSandboxImage = "outprint-sandbox"
	defaultReleaseImage = "outprint"
	composePath         = "../../deploy/compose/compose.yaml"
	runLabel            = "outprint-sandbox-test"
	dockerTimeout       = 3 * time.Minute
)

var runID = randomHex(4)

func sandboxImage() string { return envOr("SANDBOX_IMAGE", defaultSandboxImage) }
func releaseImage() string { return envOr("RELEASE_IMAGE", defaultReleaseImage) }

func envOr(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

func TestMain(m *testing.M) {
	code := m.Run()
	removeLeftovers()
	os.Exit(code)
}

// removeLeftovers deletes what a crashed test left behind.
func removeLeftovers() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	out, _ := exec.CommandContext(ctx, "docker", "ps", "-aq", "--filter", "label="+runLabel+"="+runID).Output()
	if ids := strings.Fields(string(out)); len(ids) > 0 {
		_ = exec.CommandContext(ctx, "docker", append([]string{"rm", "-f"}, ids...)...).Run()
	}
}

// docker runs the docker CLI and returns its combined output.
func docker(t *testing.T, args ...string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), dockerTimeout)
	defer cancel()
	var out bytes.Buffer
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	return out.String(), err
}

func mustDocker(t *testing.T, args ...string) string {
	t.Helper()
	out, err := docker(t, args...)
	if err != nil {
		t.Fatalf("docker %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(out)
}

// requireDocker fails the test when the daemon or an image is missing.
func requireDocker(t *testing.T, images ...string) {
	t.Helper()
	if out, err := docker(t, "version", "--format", "{{.Server.Version}}"); err != nil {
		t.Fatalf("Docker is required for the sandbox suite and is not available: %v\n%s", err, out)
	}
	for _, image := range images {
		if out, err := docker(t, "image", "inspect", image); err != nil {
			t.Fatalf("image %q is required (build it with make test-sandbox): %v\n%s", image, err, out)
		}
	}
}

func loadCheckedCompose(t *testing.T) Compose {
	t.Helper()
	c, err := LoadCompose(composePath)
	if err != nil {
		t.Fatal(err)
	}
	if violations := CheckWorkerBaseline(c); len(violations) > 0 {
		t.Fatalf("the shipped Worker service violates the baseline:\n  %s", strings.Join(violations, "\n  "))
	}
	return c
}

// testNetwork creates the Docker network that stands for a Compose network.
func testNetwork(t *testing.T, composeName string, internal bool) string {
	t.Helper()
	name := fmt.Sprintf("sbx-%s-%s-%s", runID, composeName, randomHex(3))
	args := []string{"network", "create", "--label", runLabel + "=" + runID}
	if internal {
		args = append(args, "--internal")
	}
	mustDocker(t, append(args, name)...)
	t.Cleanup(func() { _, _ = docker(t, "network", "rm", name) })
	return name
}

// workerNetwork creates the Worker's network as the Compose file declares it.
func workerNetwork(t *testing.T, c Compose) func(string) string {
	t.Helper()
	declared, err := c.InternalNetworks()
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]string{}
	for composeName, internal := range declared {
		names[composeName] = testNetwork(t, composeName, internal)
	}
	return func(n string) string { return names[n] }
}

func containerName(t *testing.T, role string) string {
	return fmt.Sprintf("sbx-%s-%s-%s", runID, role, randomHex(3))
}

// removeOnCleanup force-removes a container at the end of the test and, if
// the test failed, prints its logs first.
func removeOnCleanup(t *testing.T, name string) {
	t.Cleanup(func() {
		if t.Failed() {
			logs, _ := docker(t, "logs", "--tail", "40", name)
			t.Logf("container %s log tail:\n%s", name, logs)
		}
		_, _ = docker(t, "rm", "-f", name)
	})
}

// workerFlags are the docker run flags of the shipped Worker service. oneShot
// drops the restart policy, which cannot be combined with --rm.
func workerFlags(t *testing.T, c Compose, name string, network func(string) string, env map[string]string, oneShot bool) []string {
	t.Helper()
	args, err := c.WorkerRunArgs(RunOptions{Name: name, NetworkName: network, Env: env})
	if err != nil {
		t.Fatal(err)
	}
	args = append(args, "--label", runLabel+"="+runID)
	if oneShot {
		args = dropFlag(args, "--restart")
	}
	return args
}

func dropFlag(args []string, flag string) []string {
	var out []string
	for i := 0; i < len(args); i++ {
		if args[i] == flag {
			i++
			continue
		}
		out = append(out, args[i])
	}
	return out
}

// probeRun runs the probe once with the shipped Worker flags and returns its
// standard output.
func probeRun(t *testing.T, c Compose, network func(string) string, extra []string, probeArgs ...string) string {
	t.Helper()
	name := containerName(t, "probe")
	args := []string{"run", "--rm"}
	args = append(args, workerFlags(t, c, name, network, nil, true)...)
	args = append(args, extra...)
	args = append(args, "--entrypoint", "sandbox-probe", sandboxImage())
	args = append(args, probeArgs...)
	removeOnCleanup(t, name)
	return mustDocker(t, args...)
}

func decodeJSON(t *testing.T, text string, into any) {
	t.Helper()
	if err := json.Unmarshal([]byte(text), into); err != nil {
		t.Fatalf("not JSON (%v): %q", err, text)
	}
}

func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o600)
}
