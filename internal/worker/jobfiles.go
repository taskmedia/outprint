package worker

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/taskmedia/outprint/internal/packages"
)

const (
	sharedJobDir  = "shared"
	fontsDirName  = "fonts"
	imagesDirName = "images"
	filtersDir    = "filters"
)

// filterFiles are the Lua filters, in the order Pandoc runs them.
var filterFiles = []string{"sanitize.lua", "layout.lua"}

//go:embed filters/*.lua
var filtersFS embed.FS

var imageNameRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*(\.[A-Za-z0-9_-]+)?$`)

// createJobDir makes a fresh directory below root that only the Worker's user
// can enter. It fails if the directory already exists.
func createJobDir(root, id string) (string, error) {
	if root == "" {
		root = os.TempDir()
	}
	dir := filepath.Join(root, "job-"+id)
	if err := os.Mkdir(dir, 0o700); err != nil {
		return "", fmt.Errorf("create job dir: %w", err)
	}
	// Mkdir is subject to the umask only downwards; be explicit.
	if err := os.Chmod(dir, 0o700); err != nil {
		return "", fmt.Errorf("secure job dir: %w", err)
	}
	return dir, nil
}

func prepareDirs(jobDir string) error {
	for _, d := range []string{homeDir, texmfVarDir, texmfCfgDir, texFontsDir, sharedJobDir, fontsDirName, imagesDirName, filtersDir} {
		if err := os.Mkdir(filepath.Join(jobDir, d), 0o700); err != nil {
			return fmt.Errorf("create job dir: %w", err)
		}
	}
	return nil
}

func writeFilters(jobDir string) ([]string, error) {
	paths := make([]string, 0, len(filterFiles))
	for _, name := range filterFiles {
		data, err := filtersFS.ReadFile("filters/" + name)
		if err != nil {
			return nil, fmt.Errorf("read embedded filter: %w", err)
		}
		path := filepath.Join(jobDir, filtersDir, name)
		if err := os.WriteFile(path, data, 0o600); err != nil {
			return nil, fmt.Errorf("write filter: %w", err)
		}
		paths = append(paths, path)
	}
	return paths, nil
}

func writeImages(jobDir string, images map[string][]byte) error {
	for name, data := range images {
		if !imageNameRE.MatchString(name) {
			return fmt.Errorf("image name %q is not a valid bundle name", name)
		}
		if isSVGName(name) {
			continue // converted to PDF from memory, never written as SVG
		}
		if err := os.WriteFile(filepath.Join(jobDir, imagesDirName, name), data, 0o600); err != nil {
			return fmt.Errorf("write image: %w", err)
		}
	}
	return nil
}

func copyShared(t packages.Template, jobDir string) error {
	names, err := t.SharedFiles()
	if err != nil {
		return err
	}
	for _, name := range names {
		src, err := t.SharedPath(name)
		if err != nil {
			return err
		}
		if err := copyFile(src, filepath.Join(jobDir, sharedJobDir, name)); err != nil {
			return err
		}
	}
	return nil
}

// copyTemplateFonts copies the files of the Template's fonts directory, if
// it has one, to the job directory. A Template loads them with the fontspec
// option Path=./fonts/.
func copyTemplateFonts(t packages.Template, jobDir string) error {
	return copyTree(filepath.Join(t.Dir, fontsDirName), filepath.Join(jobDir, fontsDirName), true)
}

// copyTree copies the regular files below src into dst, keeping the layout.
// Anything that is not a regular file or directory (a symlink, a device) is
// refused. A missing src is not an error when optional is true.
func copyTree(src, dst string, optional bool) error {
	if _, err := os.Lstat(src); os.IsNotExist(err) && optional {
		return nil
	}
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("walk %s: %w", filepath.Base(src), err)
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		switch {
		case d.IsDir():
			return mkdirIfMissing(target)
		case d.Type().IsRegular():
			return copyFile(path, target)
		}
		return fmt.Errorf("%s: only regular files and directories can be copied", strings.TrimPrefix(rel, "./"))
	})
}

func mkdirIfMissing(path string) error {
	if err := os.Mkdir(path, 0o700); err != nil && !os.IsExist(err) {
		return fmt.Errorf("create directory: %w", err)
	}
	return nil
}

// copyFile copies a file and keeps its modification time. luaotfload checks
// the size and time of a font file against its cache, so a copied font only
// hits the pre-built cache if the copy carries the original's time.
func copyFile(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("stat %s: %w", filepath.Base(src), err)
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("read %s: %w", filepath.Base(src), err)
	}
	if err := os.WriteFile(dst, data, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", filepath.Base(dst), err)
	}
	if err := os.Chtimes(dst, info.ModTime(), info.ModTime()); err != nil {
		return fmt.Errorf("set time of %s: %w", filepath.Base(dst), err)
	}
	return nil
}
