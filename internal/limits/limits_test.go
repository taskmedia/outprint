package limits

import (
	"strings"
	"testing"
	"time"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestStartingLimits(t *testing.T) {
	if WallClock != 60*time.Second || MaxPages != 500 || MaxPDFBytes != 50<<20 ||
		MaxMarkdownBytes != 1<<20 || MaxTeXRuns != 2 {
		t.Errorf("per-job limits changed: %v %d %d %d %d", WallClock, MaxPages, MaxPDFBytes, MaxMarkdownBytes, MaxTeXRuns)
	}
	if ContainerMemoryBytes != 1<<30 || ContainerCPUs != 1 || ContainerPIDs != 128 || ContainerTmpfsBytes != 256<<20 {
		t.Error("container limits changed")
	}
}

func TestFromEnv(t *testing.T) {
	got, err := FromEnv(env(map[string]string{EnvMaxPages: "12", EnvMaxPDFBytes: "4096"}))
	if err != nil || got.MaxPages != 12 || got.MaxPDFBytes != 4096 {
		t.Errorf("got %+v, %v", got, err)
	}
	got, err = FromEnv(env(nil))
	if err != nil || got != Defaults() {
		t.Errorf("unset: got %+v, %v", got, err)
	}
}

func TestFromEnvRefusesBadValues(t *testing.T) {
	cases := map[string]map[string]string{
		EnvMaxPages:    {EnvMaxPages: "many"},
		"zero pages":   {EnvMaxPages: "0"},
		"negative pdf": {EnvMaxPDFBytes: "-1"},
		"pdf above":    {EnvMaxPDFBytes: "999999999999"},
	}
	for name, vars := range cases {
		_, err := FromEnv(env(vars))
		if err == nil || !strings.Contains(err.Error(), "RENDERER_MAX_") {
			t.Errorf("%s: err = %v, want one naming the variable", name, err)
		}
	}
}

func TestWithDefaults(t *testing.T) {
	if got := (Job{}).WithDefaults(); got != Defaults() {
		t.Errorf("got %+v", got)
	}
	if got := (Job{MaxPages: 3}).WithDefaults(); got.MaxPages != 3 || got.MaxPDFBytes != MaxPDFBytes {
		t.Errorf("got %+v", got)
	}
}
