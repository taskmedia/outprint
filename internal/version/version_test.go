package version

import "testing"

func setForTest(t *testing.T, v, c string) {
	t.Helper()
	oldV, oldC := Version, Commit
	t.Cleanup(func() { Version, Commit = oldV, oldC })
	Version, Commit = v, c
}

func TestVersionDefaultsToDev(t *testing.T) {
	if Version != "dev" {
		t.Fatalf("Version = %q, want %q", Version, "dev")
	}
}

func TestLineAppendsShortCommitWhenKnown(t *testing.T) {
	setForTest(t, "0.1.0", "abcdef1234567890")
	if got, want := Line("outprint"), "outprint 0.1.0 (commit abcdef123456)"; got != want {
		t.Fatalf("Line = %q, want %q", got, want)
	}
}

func TestLineOmitsCommitWhenUnknown(t *testing.T) {
	setForTest(t, "0.1.0", "")
	if got, want := Line("outprint-injector"), "outprint-injector 0.1.0"; got != want {
		t.Fatalf("Line = %q, want %q", got, want)
	}
}
