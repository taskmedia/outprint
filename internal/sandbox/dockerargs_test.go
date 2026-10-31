package sandbox

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestWorkerRunArgsCarryEveryShippedFlag(t *testing.T) {
	args, err := loadShipped(t).WorkerRunArgs(RunOptions{
		Name:        "w1",
		NetworkName: func(n string) string { return "t-" + n },
		Env:         map[string]string{"RENDERER_TIMEOUT": "5s"},
	})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, " ")
	templates, _ := filepath.Abs("../../templates")
	for _, want := range []string{
		"--user 10001:10001", "--security-opt no-new-privileges:true", "--cap-drop ALL", "--read-only",
		"--tmpfs /tmp:mode=1777,size=64m,uid=10001,gid=10001",
		"--tmpfs /work:mode=0700,size=256m,uid=10001,gid=10001",
		"-v " + templates + ":/opt/renderer/templates:ro",
		"-e RENDERER_WORK_DIR=/work", "-e RENDERER_WORKER_TOKEN= ", "-e RENDERER_TIMEOUT=5s",
		"--network t-worker-net", "--memory 1g", "--cpus 1", "--pids-limit 128", "--restart always",
	} {
		if !strings.Contains(joined+" ", want) {
			t.Errorf("flags lack %q:\n%s", want, joined)
		}
	}
	for _, forbidden := range []string{"--privileged", "--cap-add", "-p ", "--publish", "seccomp"} {
		if slices.Contains(args, forbidden) || strings.Contains(joined, forbidden) {
			t.Errorf("flags contain %q", forbidden)
		}
	}
}

func TestWorkerRunArgsRefuseWhatTheyCannotTranslate(t *testing.T) {
	c, err := ParseCompose([]byte("services:\n  worker:\n    image: x\n    privileged: true\n"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.WorkerRunArgs(RunOptions{NetworkName: func(string) string { return "" }}); err == nil ||
		!strings.Contains(err.Error(), "privileged") {
		t.Errorf("err = %v, want one naming privileged", err)
	}
}

func TestInterpolateAndSize(t *testing.T) {
	none := func(string) (string, bool) { return "", false }
	if got := Interpolate("a-${X:-b}-${Y}", none); got != "a-b-" {
		t.Errorf("got %q", got)
	}
	if got := Interpolate("${X:-b}", func(string) (string, bool) { return "set", true }); got != "set" {
		t.Errorf("got %q", got)
	}
	for in, want := range map[string]int64{"1g": 1 << 30, "256m": 256 << 20, "10": 10, "2K": 2048} {
		if got, err := ParseSize(in); err != nil || got != want {
			t.Errorf("ParseSize(%q) = %d, %v", in, got, err)
		}
	}
	if _, err := ParseSize("lots"); err == nil {
		t.Error("ParseSize(lots) succeeded")
	}
}
