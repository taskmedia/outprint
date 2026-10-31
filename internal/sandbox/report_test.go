//go:build sandbox

package sandbox

import "testing"

type probeCheck struct {
	Name   string `json:"name"`
	Breach bool   `json:"breach"`
	Errno  string `json:"errno"`
	Detail string `json:"detail"`
}

type probeReport struct {
	Checks []probeCheck      `json:"checks"`
	Info   map[string]string `json:"info"`
}

func (r probeReport) find(name string) (probeCheck, bool) {
	for _, c := range r.Checks {
		if c.Name == name {
			return c, true
		}
	}
	return probeCheck{}, false
}

// assertContained fails for every breach and for every expected check that is
// missing, so a probe that silently stops checking cannot pass.
func assertContained(t *testing.T, r probeReport, expected []string) {
	t.Helper()
	for _, name := range expected {
		if _, ok := r.find(name); !ok {
			t.Errorf("the report has no check %q", name)
		}
	}
	for _, c := range r.Checks {
		if c.Breach {
			t.Errorf("BREACH %s: %s (errno %s)", c.Name, c.Detail, c.Errno)
		}
	}
}

func assertErrno(t *testing.T, r probeReport, name string, want ...string) {
	t.Helper()
	c, ok := r.find(name)
	if !ok {
		t.Errorf("the report has no check %q", name)
		return
	}
	for _, w := range want {
		if c.Errno == w {
			return
		}
	}
	t.Errorf("%s: errno %q, want one of %v", name, c.Errno, want)
}
