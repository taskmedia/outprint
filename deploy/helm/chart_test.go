//go:build helm

// Package helm_test renders the chart with `helm template` and asserts the
// Worker baseline of ADR 0001 on the result, so weakening the chart breaks
// the build (docs/deploy/helm-kubernetes.md). It needs the helm binary; run
// it with `task test-helm`. It is not part of the plain `go test ./...`.
package helm_test

import (
	"bytes"
	"errors"
	"io"
	"os/exec"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const (
	chartDir    = "outprint"
	tokenEnv    = "OUTPRINT_WORKER_TOKEN"
	workerUID   = 10001
	outlineFlag = "outline.url=http://outline.example.invalid:3000"
)

type object map[string]any

func (o object) get(path ...string) any {
	var cur any = map[string]any(o)
	for _, key := range path {
		m, ok := asMap(cur)
		if !ok {
			return nil
		}
		cur = m[key]
	}
	return cur
}

// asMap accepts both map types: yaml.v3 decodes nested maps into the named
// type of the outermost map.
func asMap(v any) (map[string]any, bool) {
	switch m := v.(type) {
	case object:
		return m, true
	case map[string]any:
		return m, true
	}
	return nil, false
}

func (o object) kind() string { s, _ := o["kind"].(string); return s }
func (o object) name() string { s, _ := o.get("metadata", "name").(string); return s }

func render(t *testing.T, sets ...string) []object {
	t.Helper()
	args := []string{"template", "rel", chartDir, "--set", outlineFlag}
	for _, s := range sets {
		args = append(args, "--set", s)
	}
	var stdout, stderr bytes.Buffer
	cmd := exec.Command("helm", args...)
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("helm %v: %v\n%s", args, err, stderr.String())
	}
	var out []object
	dec := yaml.NewDecoder(&stdout)
	for {
		var o object
		err := dec.Decode(&o)
		if errors.Is(err, io.EOF) {
			return out
		}
		if err != nil {
			t.Fatalf("decode rendered chart: %v", err)
		}
		if o != nil {
			out = append(out, o)
		}
	}
}

func find(t *testing.T, objs []object, kind, suffix string) object {
	t.Helper()
	for _, o := range objs {
		if o.kind() == kind && strings.HasSuffix(o.name(), suffix) {
			return o
		}
	}
	t.Fatalf("no %s named *%s in the rendered chart", kind, suffix)
	return nil
}

func mo(v any) object { m, _ := asMap(v); return m }

func list(v any) []any { l, _ := v.([]any); return l }

func TestWorkerKeepsTheBaseline(t *testing.T) {
	objs := render(t)
	dep := find(t, objs, "Deployment", "-worker")
	pod := mo(dep.get("spec", "template", "spec"))

	checks := map[string]bool{
		"runAsNonRoot":                 pod.get("securityContext", "runAsNonRoot") == true,
		"fixed non-root uid":           pod.get("securityContext", "runAsUser") == workerUID,
		"fixed gid":                    pod.get("securityContext", "runAsGroup") == workerUID,
		"runtime default seccomp":      pod.get("securityContext", "seccompProfile", "type") == "RuntimeDefault",
		"no service account token":     pod.get("automountServiceAccountToken") == false,
		"no service links":             pod.get("enableServiceLinks") == false,
		"no host network":              pod.get("hostNetwork") == nil,
		"no host pid":                  pod.get("hostPID") == nil,
		"no host ipc":                  pod.get("hostIPC") == nil,
		"no host path or secret mount": noVolumeOfKind(pod, "hostPath", "secret", "projected"),
	}
	for name, ok := range checks {
		if !ok {
			t.Errorf("worker baseline weakened: %s", name)
		}
	}

	containers := list(pod.get("containers"))
	if len(containers) != 1 {
		t.Fatalf("worker pod has %d containers, want 1", len(containers))
	}
	c := mo(containers[0])
	sc := func(k string) any { return c.get("securityContext", k) }
	if sc("allowPrivilegeEscalation") != false {
		t.Error("worker baseline weakened: privilege escalation allowed")
	}
	if sc("readOnlyRootFilesystem") != true {
		t.Error("worker baseline weakened: root file system writable")
	}
	if sc("privileged") != nil {
		t.Error("worker baseline weakened: privileged set")
	}
	if drop := list(c.get("securityContext", "capabilities", "drop")); len(drop) != 1 || drop[0] != "ALL" {
		t.Errorf("worker baseline weakened: capabilities drop = %v, want [ALL]", drop)
	}
	if c.get("securityContext", "capabilities", "add") != nil {
		t.Error("worker baseline weakened: capabilities added")
	}
	for _, res := range []string{"cpu", "memory"} {
		if c.get("resources", "limits", res) == nil {
			t.Errorf("worker baseline weakened: no %s limit", res)
		}
	}
	if len(list(c.get("ports"))) != 1 {
		t.Error("worker exposes more than one port")
	}
	assertWorkerEnvHoldsNoSecrets(t, c)
	assertWritableVolumesAreMemory(t, pod)
}

func noVolumeOfKind(pod object, kinds ...string) bool {
	for _, v := range list(pod.get("volumes")) {
		for _, k := range kinds {
			if _, found := mo(v)[k]; found {
				return false
			}
		}
	}
	return true
}

func assertWorkerEnvHoldsNoSecrets(t *testing.T, c object) {
	t.Helper()
	for _, e := range list(c.get("env")) {
		env := mo(e)
		name, _ := env["name"].(string)
		if env.get("valueFrom", "secretKeyRef") != nil && name != tokenEnv {
			t.Errorf("worker takes %s from a Secret; only %s may", name, tokenEnv)
		}
		if env.get("valueFrom", "configMapKeyRef") != nil || env.get("valueFrom", "fieldRef") != nil {
			t.Errorf("worker env %s is taken from the cluster; set literals only", name)
		}
	}
	if c.get("envFrom") != nil {
		t.Error("worker uses envFrom; list variables explicitly")
	}
}

func assertWritableVolumesAreMemory(t *testing.T, pod object) {
	t.Helper()
	for _, v := range list(pod.get("volumes")) {
		vol := mo(v)
		if ed := mo(vol["emptyDir"]); ed != nil {
			if ed["medium"] != "Memory" || ed["sizeLimit"] == nil {
				t.Errorf("volume %v: a writable volume must be in memory with a size limit", vol["name"])
			}
		}
	}
	mounts := map[string]bool{}
	for _, m := range list(mo(list(pod.get("containers"))[0]).get("volumeMounts")) {
		mm := mo(m)
		mounts[mm["mountPath"].(string)] = mm["readOnly"] == true
	}
	if ro, ok := mounts["/etc/outprint"]; !ok || !ro {
		t.Error("the service configuration must be mounted read-only")
	}
}

func TestWorkerHasNoNetwork(t *testing.T) {
	objs := render(t)
	np := find(t, objs, "NetworkPolicy", "-worker")
	types := list(np.get("spec", "policyTypes"))
	if !slices.Contains(types, any("Egress")) || !slices.Contains(types, any("Ingress")) {
		t.Errorf("policyTypes = %v, want Ingress and Egress", types)
	}
	if egress := list(np.get("spec", "egress")); len(egress) != 0 {
		t.Errorf("the Worker may not have any egress rule, got %v", egress)
	}
	from := list(np.get("spec", "ingress"))
	if len(from) != 1 {
		t.Fatalf("want one ingress rule, got %d", len(from))
	}
	peers := list(mo(from[0]).get("from"))
	if len(peers) != 1 || mo(peers[0]).get("podSelector", "matchLabels", "app.kubernetes.io/component") != "gateway" {
		t.Errorf("ingress must come from the Gateway only, got %v", peers)
	}
}

func TestOnlyTheTokenIsASecret(t *testing.T) {
	objs := render(t)
	for _, o := range objs {
		if o.kind() == "Secret" && !strings.HasSuffix(o.name(), "-worker-token") {
			t.Errorf("unexpected Secret %s", o.name())
		}
	}
}

func TestInjectorOffRendersNoInjector(t *testing.T) {
	for _, o := range render(t, "injector.enabled=false") {
		if strings.HasSuffix(o.name(), "-injector") {
			t.Errorf("injector.enabled=false still renders %s %s", o.kind(), o.name())
		}
	}
}

func TestSlotsFollowWorkerReplicas(t *testing.T) {
	cm := find(t, render(t, "worker.replicas=3"), "ConfigMap", "-service")
	var cfg object
	text, _ := cm.get("data", "service.yaml").(string)
	if err := yaml.Unmarshal([]byte(text), &cfg); err != nil {
		t.Fatal(err)
	}
	if got := cfg.get("admission", "slots"); got != 3 {
		t.Errorf("admission.slots = %v, want 3", got)
	}
}

func TestOutlineURLIsRequired(t *testing.T) {
	var stderr bytes.Buffer
	cmd := exec.Command("helm", "template", "rel", chartDir)
	cmd.Stderr = &stderr
	if cmd.Run() == nil || !strings.Contains(stderr.String(), "outline.url") {
		t.Errorf("rendering without outline.url should fail naming it, got %q", stderr.String())
	}
}
