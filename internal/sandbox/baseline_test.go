package sandbox

import (
	"os"
	"strings"
	"testing"
)

const shippedCompose = "../../deploy/compose/compose.yaml"

func loadShipped(t *testing.T) Compose {
	t.Helper()
	c, err := LoadCompose(shippedCompose)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestShippedComposeMeetsTheWorkerBaseline(t *testing.T) {
	if got := CheckWorkerBaseline(loadShipped(t)); len(got) != 0 {
		t.Errorf("the shipped Worker service violates the baseline:\n  %s", strings.Join(got, "\n  "))
	}
}

// Each weakening must be reported, with the Compose key in the message.
func TestWeakeningTheShippedWorkerBreaksTheBaseline(t *testing.T) {
	data, err := os.ReadFile(shippedCompose)
	if err != nil {
		t.Fatal(err)
	}
	original := string(data)
	workerOnly := func(old, replacement string) string {
		// The worker block comes first; replace its first occurrence only.
		if !strings.Contains(original, old) {
			t.Fatalf("shipped compose no longer contains %q; update the mutation", old)
		}
		return strings.Replace(original, old, replacement, 1)
	}
	mutations := map[string]struct{ text, key string }{
		"root user":        {workerOnly(`user: "10001:10001"`, `user: "0:0"`), "user"},
		"named user":       {workerOnly(`user: "10001:10001"`, `user: "render"`), "user"},
		"no privileges":    {workerOnly("      - no-new-privileges:true\n", ""), "security_opt"},
		"seccomp off":      {workerOnly("      - no-new-privileges:true\n", "      - no-new-privileges:true\n      - seccomp:unconfined\n"), "security_opt"},
		"capabilities":     {workerOnly("cap_drop: [ALL]", "cap_drop: [NET_RAW]"), "cap_drop"},
		"writable root":    {workerOnly("read_only: true", "read_only: false"), "read_only"},
		"bigger tmpfs":     {workerOnly("size=256m", "size=512m"), "tmpfs"},
		"executable tmp":   {workerOnly("/tmp:mode=1777,", "/tmp:exec,mode=1777,"), "tmpfs"},
		"secret env":       {workerOnly(`OUTPRINT_WORK_DIR: /work`, "OUTPRINT_WORK_DIR: /work\n      API_KEY: hunter2"), "environment"},
		"token default":    {workerOnly("${OUTPRINT_WORKER_TOKEN:?set OUTPRINT_WORKER_TOKEN to a shared secret}", "fixed-token"), "environment"},
		"writable mount":   {workerOnly("target: /opt/outprint/templates\n        read_only: true", "target: /opt/outprint/templates\n        read_only: false"), "volumes"},
		"host mount":       {workerOnly("source: ../../templates", "source: /var/run"), "volumes"},
		"open network":     {strings.Replace(original, "worker-net:\n    internal: true", "worker-net:\n    internal: false", 1), "networks"},
		"published port":   {workerOnly("    networks: [worker-net]\n    mem_limit: 1g", "    networks: [worker-net]\n    ports: [\"8081:8081\"]\n    mem_limit: 1g"), "ports"},
		"memory":           {workerOnly("mem_limit: 1g", "mem_limit: 4g"), "mem_limit"},
		"cpus":             {workerOnly("cpus: 1", "cpus: 4"), "cpus"},
		"pids":             {workerOnly("pids_limit: 128", "pids_limit: 4096"), "pids_limit"},
		"no restart":       {workerOnly("restart: always", "restart: \"no\""), "restart"},
		"privileged":       {workerOnly("    read_only: true\n", "    read_only: true\n    privileged: true\n"), "privileged"},
		"host network":     {workerOnly("    read_only: true\n", "    read_only: true\n    network_mode: host\n"), "network_mode"},
		"added capability": {workerOnly("    read_only: true\n", "    read_only: true\n    cap_add: [SYS_ADMIN]\n"), "cap_add"},
		"devices":          {workerOnly("    read_only: true\n", "    read_only: true\n    devices: [\"/dev/kvm\"]\n"), "devices"},
	}
	for name, m := range mutations {
		c, err := ParseCompose([]byte(m.text))
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		got := CheckWorkerBaseline(c)
		if !containsKey(got, m.key) {
			t.Errorf("%s: no violation names %q; got %v", name, m.key, got)
		}
	}
}

func containsKey(violations []string, key string) bool {
	for _, v := range violations {
		if strings.HasPrefix(v, key+":") {
			return true
		}
	}
	return false
}
