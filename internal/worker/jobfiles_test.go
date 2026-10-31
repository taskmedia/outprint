package worker

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"
)

func TestCreateJobDirIsFreshAndPrivate(t *testing.T) {
	root := t.TempDir()
	dir, err := createJobDir(root, "abc")
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Errorf("job dir mode = %v, want 0700", info.Mode().Perm())
	}
	if filepath.Dir(dir) != root {
		t.Errorf("job dir %s is not directly below %s", dir, root)
	}
	if _, err := createJobDir(root, "abc"); err == nil {
		t.Error("an existing job directory must be refused")
	}
}

func TestNewJobIDIsRandomHex(t *testing.T) {
	a, err := newJobID()
	if err != nil {
		t.Fatal(err)
	}
	b, _ := newJobID()
	if !regexp.MustCompile(`^[0-9a-f]{16}$`).MatchString(a) || a == b {
		t.Errorf("ids %q and %q are not distinct 16-digit hex strings", a, b)
	}
}

func TestWriteImagesAcceptsOnlyBundleNames(t *testing.T) {
	for _, name := range []string{"../x.png", "a/b.png", ".hidden", "", "a b.png", "x.png.exe/..", "a\\b.png"} {
		dir := t.TempDir()
		if err := os.Mkdir(filepath.Join(dir, "images"), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := writeImages(dir, map[string][]byte{name: []byte("x")}); err == nil {
			t.Errorf("name %q accepted", name)
		}
	}
	dir := t.TempDir()
	_ = os.Mkdir(filepath.Join(dir, "images"), 0o700)
	if err := writeImages(dir, map[string][]byte{"img-1.png": []byte("x")}); err != nil {
		t.Fatal(err)
	}
}

func TestCopyTreeRefusesSymlinksAndKeepsLayout(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	if err := os.MkdirAll(filepath.Join(src, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "sub", "f.txt"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := copyTree(src, dst, false); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(filepath.Join(dst, "sub", "f.txt")); string(got) != "data" {
		t.Errorf("copied content = %q", got)
	}

	if err := os.Symlink("/etc/hostname", filepath.Join(src, "link")); err != nil {
		t.Fatal(err)
	}
	if err := copyTree(src, t.TempDir(), false); err == nil {
		t.Error("a symlink in the tree must be refused")
	}
}

func TestCopyTreeOptionalMissingSource(t *testing.T) {
	if err := copyTree(filepath.Join(t.TempDir(), "nope"), t.TempDir(), true); err != nil {
		t.Errorf("optional missing source: %v", err)
	}
	if err := copyTree(filepath.Join(t.TempDir(), "nope"), t.TempDir(), false); err == nil {
		t.Error("required missing source must fail")
	}
}

func TestCopyFileKeepsTheModificationTime(t *testing.T) {
	dir := t.TempDir()
	src, dst := filepath.Join(dir, "font.otf"), filepath.Join(dir, "copy.otf")
	if err := os.WriteFile(src, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Date(2018, 6, 15, 12, 0, 0, 0, time.UTC)
	if err := os.Chtimes(src, old, old); err != nil {
		t.Fatal(err)
	}

	if err := copyFile(src, dst); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(dst)
	if err != nil {
		t.Fatal(err)
	}
	if !info.ModTime().Equal(old) {
		t.Errorf("copy time = %v, want %v (a changed time invalidates the font cache)", info.ModTime(), old)
	}
}
