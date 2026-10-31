//go:build sandbox

package sandbox

import (
	"fmt"
	"net"
	"strings"
	"testing"
	"time"
)

// adversary is everything a hostile job tries to reach. All values are random
// and generated at test time; none of it is mounted into the Worker.
type adversary struct {
	canaryPath    string
	canaryContent string
	jobDir        string
	jobContent    string
	secret        string
	hostBanner    string
	hostPort      string
	listenerAddr  string
	listenerWord  string
	hostIP        string
	// dial are the addresses the hostile job must not reach.
	dial []string
}

const hostGateway = "host.docker.internal"

var hostAlias = []string{"--add-host", hostGateway + ":host-gateway"}

func newAdversary(t *testing.T) adversary {
	t.Helper()
	a := adversary{
		canaryContent: "canary-" + randomHex(16),
		jobDir:        "/work/job-" + randomHex(4),
		jobContent:    "job-secret-" + randomHex(16),
		secret:        "env-secret-" + randomHex(16),
		hostBanner:    "host-banner-" + randomHex(8),
		listenerWord:  "listener-banner-" + randomHex(8),
	}
	a.canaryPath = writeCanary(t, a.canaryContent)
	a.startHostListener(t)
	a.startOtherContainers(t)
	a.dial = []string{
		a.hostIP + ":" + a.hostPort, hostGateway + ":" + a.hostPort, a.listenerAddr, "1.1.1.1:443", "8.8.8.8:53",
	}
	return a
}

func writeCanary(t *testing.T, content string) string {
	t.Helper()
	path := t.TempDir() + "/canary.txt"
	if err := writeFile(path, content); err != nil {
		t.Fatal(err)
	}
	return path
}

func (a *adversary) startHostListener(t *testing.T) {
	t.Helper()
	ln, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Fatalf("host listener: %v", err)
	}
	t.Cleanup(func() { ln.Close() })
	_, a.hostPort, _ = net.SplitHostPort(ln.Addr().String())
	banner := a.hostBanner
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			fmt.Fprintln(conn, banner)
			conn.Close()
		}
	}()
}

// startOtherContainers starts a container that holds an env secret and one
// that listens on the default bridge, and proves with a control container
// that both and the host listener are reachable from where they are not
// isolated, so "unreachable" below means something.
func (a *adversary) startOtherContainers(t *testing.T) {
	t.Helper()
	other := containerName(t, "other")
	removeOnCleanup(t, other)
	mustDocker(t, "run", "-d", "--name", other, "--label", runLabel+"="+runID, "-e", "SECRET_TOKEN="+a.secret,
		"--entrypoint", "sandbox-probe", sandboxImage(), "hold")
	listener := containerName(t, "listener")
	removeOnCleanup(t, listener)
	mustDocker(t, "run", "-d", "--name", listener, "--label", runLabel+"="+runID,
		"--entrypoint", "sandbox-probe", sandboxImage(), "listen", ":9999", a.listenerWord)
	a.listenerAddr = mustDocker(t, "inspect", "-f", `{{(index .NetworkSettings.Networks "bridge").IPAddress}}`, listener) + ":9999"

	control := func(args ...string) string {
		base := append([]string{"run", "--rm", "--label", runLabel + "=" + runID}, hostAlias...)
		base = append(base, "--entrypoint", "sandbox-probe", sandboxImage())
		return waitForOutput(t, append(base, args...))
	}
	a.hostIP = firstIPv4(t, control("resolve", hostGateway))
	if got := control("dial", hostGateway+":"+a.hostPort); got != "connected "+a.hostBanner {
		t.Fatalf("control: the host listener is not reachable from a plain container: %q", got)
	}
	if got := control("dial", a.listenerAddr); got != "connected "+a.listenerWord {
		t.Fatalf("control: the listener container is not reachable from a plain container: %q", got)
	}
}

// waitForOutput runs docker until it prints something that is not an error,
// so a container that was just started has time to listen.
func waitForOutput(t *testing.T, args []string) string {
	t.Helper()
	var out string
	for i := 0; i < 20; i++ {
		out, _ = docker(t, args...)
		out = strings.TrimSpace(out)
		if out != "" && !strings.HasPrefix(out, "error") {
			return out
		}
		time.Sleep(500 * time.Millisecond)
	}
	return out
}

func (a adversary) checkArgs() []string {
	args := []string{"checks", "--canary", a.canaryPath, "--job-canary", a.jobDir + "/secret", "--secret", a.secret}
	for _, d := range a.dial {
		args = append(args, "--dial", d)
	}
	return args
}

func (a adversary) leaks() []string {
	return []string{a.canaryContent, a.jobContent, a.secret, a.hostBanner, a.listenerWord}
}

func (a adversary) expectedChecks() []string {
	names := []string{
		"read_host_canary", "read_other_job_dir", "env_secret_of_other_container", "dns:example.org",
		"write:/probe-write", "write:/etc/probe-write", "write:/usr/local/bin/probe-write",
		"write:/opt/renderer/templates/probe-write", "write:/opt/renderer/configurations/probe-write",
		"setuid_root", "mount", "mknod", "chown_root", "user_namespace", "exec_from_work", "exec_from_tmp", "ptrace",
		"capabilities:CapInh", "capabilities:CapPrm", "capabilities:CapEff", "capabilities:CapBnd", "capabilities:CapAmb",
		"no_new_privileges", "seccomp_filter", "not_root", "fork_bomb",
	}
	for _, d := range a.dial {
		names = append(names, "connect:"+d)
	}
	return names
}

// plantJob starts job A: a Worker-flag container whose job directory holds a
// secret. Its tmpfs is its own, so job B must not see it.
func (a adversary) plantJob(t *testing.T, c Compose, network func(string) string) string {
	t.Helper()
	name := containerName(t, "jobA")
	removeOnCleanup(t, name)
	args := append([]string{"run", "-d"}, workerFlags(t, c, name, network, nil, false)...)
	args = append(args, "--entrypoint", "sandbox-probe", sandboxImage(), "plant", a.jobDir, a.jobContent)
	mustDocker(t, args...)
	for i := 0; i < 40; i++ {
		if out, _ := docker(t, "logs", name); strings.Contains(out, "planted") {
			return name
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatalf("job A did not plant its secret:\n%s", mustDocker(t, "logs", name))
	return ""
}

func TestHostileJobCannotEscapeTheWorkerSandbox(t *testing.T) {
	requireDocker(t, sandboxImage())
	c := loadCheckedCompose(t)
	network := workerNetwork(t, c)
	a := newAdversary(t)
	jobA := a.plantJob(t, c, network)

	out := probeRun(t, c, network, hostAlias, a.checkArgs()...)

	for _, leak := range a.leaks() {
		if strings.Contains(out, leak) {
			t.Fatalf("the probe output contains a canary value %q", leak)
		}
	}
	var report probeReport
	decodeJSON(t, out, &report)
	logReport(t, report)
	assertContained(t, report, a.expectedChecks())
	assertHostileJobFacts(t, report)
	t.Run("the harness is not vacuous", func(t *testing.T) {
		assertControls(t, c, a, jobA)
	})
}

// assertHostileJobFacts checks the specific errno of each refusal and the
// shipped container limits the probe saw from inside.
func assertHostileJobFacts(t *testing.T, report probeReport) {
	t.Helper()
	assertErrno(t, report, "read_host_canary", "ENOENT")
	assertErrno(t, report, "read_other_job_dir", "ENOENT")
	for _, c := range report.Checks {
		if strings.HasPrefix(c.Name, "write:") {
			assertErrno(t, report, c.Name, "EROFS")
		}
	}
	assertErrno(t, report, "setuid_root", "EPERM")
	assertErrno(t, report, "mount", "EPERM")
	assertErrno(t, report, "mknod", "EPERM")
	assertErrno(t, report, "chown_root", "EPERM")
	assertErrno(t, report, "user_namespace", "EPERM")
	assertErrno(t, report, "exec_from_work", "EACCES")
	assertErrno(t, report, "exec_from_tmp", "EACCES")
	assertErrno(t, report, "ptrace", "EPERM")

	assertInfo(t, report, "memory_max", "1073741824")
	assertInfo(t, report, "pids_max", "128")
	if cpu := report.Info["cpu_max"]; !strings.HasPrefix(cpu, "100000 100000") {
		t.Errorf("cpu_max = %q, want one CPU", cpu)
	}
	assertInfo(t, report, "work_bytes", "268435456")
	assertInfo(t, report, "tmp_bytes", "67108864")
	assertInfo(t, report, "work_writable", "yes")
	assertInfo(t, report, "tmp_writable", "yes")
	assertInfo(t, report, "uid", "10001")
	assertEnvNames(t, report.Info["env_names"])

	bomb, _ := report.find("fork_bomb")
	var spawned int
	if _, err := fmt.Sscanf(bomb.Detail, "spawned %d", &spawned); err != nil || spawned < 10 || spawned >= 128 {
		t.Errorf("fork bomb detail %q: want between 10 and 127 processes", bomb.Detail)
	}
}

func assertInfo(t *testing.T, r probeReport, key, want string) {
	t.Helper()
	if got := r.Info[key]; got != want {
		t.Errorf("%s = %q, want %q", key, got, want)
	}
}

// allowedEnvNames are the names a Worker container may have: the Compose
// file's, the image's and the ones Docker adds.
var allowedEnvNames = map[string]bool{
	"PATH": true, "HOSTNAME": true, "HOME": true, "TERM": true,
	"RENDERER_LISTEN": true, "RENDERER_WORK_DIR": true, "RENDERER_WORKER_TOKEN": true,
	"RENDERER_TEMPLATES_ROOT": true, "RENDERER_CONFIGURATIONS_ROOT": true,
	"RENDERER_SERVICE_CONFIG": true, "RENDERER_TEXCACHE_DIR": true,
}

func assertEnvNames(t *testing.T, joined string) {
	t.Helper()
	for _, name := range strings.Split(joined, ",") {
		if !allowedEnvNames[name] {
			t.Errorf("the Worker environment has an unexpected variable %q", name)
		}
	}
}

// assertControls shows that each attack would have worked without the
// sandbox: the same probe reads a mounted canary, sees an env secret, and
// reads job A from inside job A's own container.
func assertControls(t *testing.T, c Compose, a adversary, jobA string) {
	t.Helper()
	network := func(string) string { return "bridge" }
	mount := []string{"-v", a.canaryPath + ":/canary:ro", "-e", "SECRET_TOKEN=" + a.secret}
	read := probeRun(t, c, network, mount, "read", "/canary")
	if !strings.HasPrefix(read, "read ") {
		t.Errorf("control: a mounted canary is not readable by the probe: %q", read)
	}
	var open probeReport
	decodeJSON(t, probeRun(t, c, network, mount, "checks", "--secret", a.secret, "--lookup", "", "--pids-limit", "128"), &open)
	if env, _ := open.find("env_secret_of_other_container"); !env.Breach {
		t.Error("control: the probe does not detect a secret in its own environment")
	}
	if got := mustDocker(t, "exec", jobA, "sandbox-probe", "read", a.jobDir+"/secret"); !strings.HasPrefix(got, "read ") {
		t.Errorf("control: job A cannot read its own directory: %q", got)
	}
}

func TestMemoryBombIsKilledAtTheContainerLimit(t *testing.T) {
	requireDocker(t, sandboxImage())
	c := loadCheckedCompose(t)
	network := workerNetwork(t, c)
	name := containerName(t, "membomb")
	removeOnCleanup(t, name)
	args := append([]string{"run", "-d"}, workerFlags(t, c, name, network, nil, true)...)
	mustDocker(t, append(args, "--entrypoint", "sandbox-probe", sandboxImage(), "memory-bomb")...)

	done := make(chan string, 1)
	go func() { done <- mustDocker(t, "wait", name) }()
	select {
	case code := <-done:
		if code != "137" {
			t.Errorf("exit code %s, want 137 (killed)", code)
		}
	case <-time.After(90 * time.Second):
		t.Fatal("the memory bomb was not stopped within 90 s")
	}
	if oom := mustDocker(t, "inspect", "-f", "{{.State.OOMKilled}}", name); oom != "true" {
		t.Errorf("OOMKilled = %s, want true", oom)
	}
}

func TestReleaseImageHasNoProbe(t *testing.T) {
	requireDocker(t, releaseImage(), sandboxImage())
	script := `set -e
test ! -e /usr/local/bin/sandbox-probe
test ! -e /usr/local/bin/lualatex
test "$(ls /usr/local/bin)" = renderer
test "$(command -v lualatex)" = /usr/bin/lualatex
test -z "$(find / -xdev -name 'sandbox-probe*' 2>/dev/null)"
echo clean`
	out, err := docker(t, "run", "--rm", "--entrypoint", "sh", releaseImage(), "-c", script)
	if err != nil || strings.TrimSpace(out) != "clean" {
		t.Errorf("the release image contains test tooling: %v\n%s", err, out)
	}
	if _, err := docker(t, "run", "--rm", "--entrypoint", "sh", sandboxImage(), "-c", script); err == nil {
		t.Error("control: the no-probe check passes on the sandbox image, so it checks nothing")
	}
	out, _ = docker(t, "run", "--rm", releaseImage(), "sandbox-probe")
	if !strings.Contains(out, "unknown") && !strings.Contains(out, "usage") {
		t.Errorf("renderer sandbox-probe should be an unknown command, got %q", out)
	}
}

func logReport(t *testing.T, r probeReport) {
	t.Helper()
	for _, c := range r.Checks {
		t.Logf("%-55s breach=%-5v errno=%-12s %s", c.Name, c.Breach, c.Errno, c.Detail)
	}
	t.Logf("info: %v", r.Info)
}

func firstIPv4(t *testing.T, addresses string) string {
	t.Helper()
	for _, field := range strings.Fields(addresses) {
		if ip := net.ParseIP(field); ip != nil && ip.To4() != nil {
			return field
		}
	}
	t.Fatalf("no IPv4 address in %q", addresses)
	return ""
}
