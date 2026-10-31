package workerserver

import (
	"bytes"
	"errors"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func fakeProc(t *testing.T, status, mountinfo string) string {
	t.Helper()
	dir := t.TempDir()
	self := filepath.Join(dir, "self")
	if err := os.Mkdir(self, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{"status": status, "mountinfo": mountinfo} {
		if err := os.WriteFile(filepath.Join(self, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func ifaces(flags ...net.Flags) func() ([]net.Interface, error) {
	return func() ([]net.Interface, error) {
		out := make([]net.Interface, len(flags))
		for i, f := range flags {
			out[i] = net.Interface{Flags: f}
		}
		return out, nil
	}
}

func TestRefuseRoot(t *testing.T) {
	if !errors.Is(RefuseRoot(0), ErrRoot) {
		t.Error("UID 0 was not refused")
	}
	if err := RefuseRoot(10001); err != nil {
		t.Errorf("UID 10001: %v", err)
	}
}

const hardenedStatus = "Name:\tx\nCapEff:\t0000000000000000\nNoNewPrivs:\t1\n"
const hardenedMounts = "1 0 0:1 / / ro,relatime - overlay overlay rw\n2 1 0:2 / /work rw,nosuid - tmpfs tmpfs rw\n"

func TestCollectPostureHardened(t *testing.T) {
	p := CollectPosture(PostureSource{
		UID: 10001, GID: 10001, ProcDir: fakeProc(t, hardenedStatus, hardenedMounts),
		Environ:    []string{"PATH=/bin", "RENDERER_WORKER_TOKEN=very-secret-value"},
		Interfaces: ifaces(net.FlagLoopback, net.FlagUp),
	})
	if p.NoNewPrivs != "true" || p.ReadOnlyRoot != "true" || p.CapEff != "0000000000000000" || p.NetInterfaces != "1" {
		t.Errorf("posture = %+v", p)
	}
	if !slices.Equal(p.SecretEnvNames, []string{"RENDERER_WORKER_TOKEN"}) {
		t.Errorf("secret names = %v", p.SecretEnvNames)
	}
	if w := p.Weaknesses(); len(w) != 0 {
		t.Errorf("the shipped baseline must not warn: %v", w)
	}
}

func TestPostureWeaknessesAndNoSecretValues(t *testing.T) {
	p := CollectPosture(PostureSource{
		UID: 10001, ProcDir: fakeProc(t, "CapEff:\t00000000a80425fb\nNoNewPrivs:\t0\n", "1 0 0:1 / / rw,relatime - ext4 /dev/x rw\n"),
		Environ: []string{"DB_PASSWORD=hunter2"},
	})
	if len(p.Weaknesses()) != 4 {
		t.Errorf("weaknesses = %v", p.Weaknesses())
	}
	var buf bytes.Buffer
	p.Log(slog.New(slog.NewTextHandler(&buf, nil)))
	if strings.Contains(buf.String(), "hunter2") || !strings.Contains(buf.String(), "DB_PASSWORD") {
		t.Errorf("log must hold names, never values:\n%s", buf.String())
	}
}

func TestPostureUnknownWhenProcMissing(t *testing.T) {
	p := CollectPosture(PostureSource{UID: 10001, ProcDir: t.TempDir()})
	if p.NoNewPrivs != "unknown" || p.ReadOnlyRoot != "unknown" || p.CapEff != "unknown" || p.NetInterfaces != "unknown" {
		t.Errorf("posture = %+v", p)
	}
	if len(p.Weaknesses()) != 0 {
		t.Errorf("unknown must not warn: %v", p.Weaknesses())
	}
}
