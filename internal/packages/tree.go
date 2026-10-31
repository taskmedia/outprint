package packages

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const (
	lfsPointerPrefix = "version https://git-lfs.github.com/spec/v1"
	gitDir           = ".git"
)

// confine resolves rel inside base and rejects anything that is absolute,
// escapes base, or reaches outside it through a symlink.
func confine(base, rel string) (string, error) {
	if rel == "" || filepath.IsAbs(rel) {
		return "", fmt.Errorf("path %q must be relative and non-empty", rel)
	}
	realBase, err := filepath.EvalSymlinks(base)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", base, err)
	}
	joined := filepath.Join(realBase, rel)
	if !within(realBase, joined) {
		return "", fmt.Errorf("path %q escapes its package", rel)
	}
	real, err := filepath.EvalSymlinks(joined)
	if err != nil {
		return "", fmt.Errorf("resolve %q: %w", rel, err)
	}
	if !within(realBase, real) {
		return "", fmt.Errorf("path %q escapes its package", rel)
	}
	return real, nil
}

func within(base, path string) bool {
	rel, err := filepath.Rel(base, path)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// checkTree walks a package directory and reports symlinks that point
// outside it and Git LFS pointer stubs. It returns the package-relative
// paths of all regular files.
func checkTree(dir string, c *collector) []string {
	realDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		c.errorf(".", "resolve package directory: %v", err)
		return nil
	}
	var files []string
	walkErr := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		rel, _ := filepath.Rel(dir, path)
		if err != nil {
			c.errorf(rel, "cannot read: %v", err)
			return nil
		}
		switch {
		case d.IsDir() && d.Name() == gitDir:
			return filepath.SkipDir
		case d.Type()&fs.ModeSymlink != 0:
			checkSymlink(realDir, path, rel, c)
		case d.Type().IsRegular():
			files = append(files, rel)
			checkLFSStub(path, rel, c)
		}
		return nil
	})
	if walkErr != nil {
		c.errorf(".", "walk package: %v", walkErr)
	}
	return files
}

func checkSymlink(realDir, path, rel string, c *collector) {
	target, err := filepath.EvalSymlinks(path)
	if err != nil {
		c.errorf(rel, "symlink cannot be resolved: %v", err)
		return
	}
	if !within(realDir, target) {
		c.errorf(rel, "symlink escapes its package")
	}
}

func checkLFSStub(path, rel string, c *collector) {
	f, err := os.Open(path)
	if err != nil {
		c.errorf(rel, "cannot read: %v", err)
		return
	}
	defer f.Close()
	head := make([]byte, len(lfsPointerPrefix))
	n, err := io.ReadFull(f, head)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		c.errorf(rel, "cannot read: %v", err)
		return
	}
	if bytes.Equal(head[:n], []byte(lfsPointerPrefix)) {
		c.errorf(rel, "file is a Git LFS pointer stub, not the real content (fetch it with git lfs pull)")
	}
}
