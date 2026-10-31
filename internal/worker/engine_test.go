package worker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fakeInstall(t *testing.T) texInstall {
	t.Helper()
	root := t.TempDir()
	dist := filepath.Join(root, "texmf-dist")
	debian := filepath.Join(root, "share-texmf")
	for _, d := range []string{dist, debian} {
		if err := os.Mkdir(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return texInstall{
		ConfigPath: "/sys/cnf:/sys/dist/web2c",
		Trees: map[string]string{
			"TEXMFDIST": dist, "TEXMFMAIN": dist, "TEXMFDEBIAN": debian,
			"TEXMFSYSVAR": filepath.Join(root, "missing"), "TEXMFSYSCONFIG": "", "TEXMFLOCAL": "",
		},
	}
}

func TestEngineSetupWritesParanoidConfigFirstInSearchPath(t *testing.T) {
	jobDir := t.TempDir()
	in := fakeInstall(t)

	path, err := engineSetup(jobDir, in)
	if err != nil {
		t.Fatal(err)
	}

	wantPath := filepath.Join(jobDir, "texmf-cnf") + ":/sys/cnf:/sys/dist/web2c"
	if path != wantPath {
		t.Errorf("TEXMFCNF = %q, want %q", path, wantPath)
	}
	data, err := os.ReadFile(filepath.Join(jobDir, "texmf-cnf", "texmf.cnf"))
	if err != nil {
		t.Fatal(err)
	}
	cnf := string(data)
	for _, want := range []string{
		"shell_escape = f\n", "openin_any = p\n", "openout_any = p\n",
		"TEXMFOUTPUT = " + jobDir + "\n",
		"OSFONTDIR =\n", "MKTEXPK = 0\n", "MKTEXTFM = 0\n",
	} {
		if !strings.Contains(cnf, want) {
			t.Errorf("texmf.cnf lacks %q:\n%s", want, cnf)
		}
	}
	for _, loose := range []string{"openin_any = a", "openin_any = r", "openout_any = a", "shell_escape = p", "shell_escape = t"} {
		if strings.Contains(cnf, loose) {
			t.Errorf("texmf.cnf loosens a setting: %q", loose)
		}
	}
}

func TestTreesAreReachedThroughLinksInsideTheJobDir(t *testing.T) {
	jobDir := t.TempDir()
	in := fakeInstall(t)

	if _, err := engineSetup(jobDir, in); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(jobDir, "texmf-cnf", "texmf.cnf"))
	cnf := string(data)

	dist := filepath.Join(jobDir, "trees", "dist")
	for _, want := range []string{
		"TEXMFDIST = " + dist,
		"TEXMFMAIN = " + dist,
		"TEXMFDEBIAN = " + filepath.Join(jobDir, "trees", "debian"),
	} {
		if !strings.Contains(cnf, want+"\n") {
			t.Errorf("texmf.cnf lacks %q", want)
		}
	}
	target, err := os.Readlink(dist)
	if err != nil || target != in.Trees["TEXMFDIST"] {
		t.Errorf("link %s -> %q (%v), want %q", dist, target, err, in.Trees["TEXMFDIST"])
	}
	for _, v := range texTreeVars {
		line := ""
		for _, l := range strings.Split(cnf, "\n") {
			if strings.HasPrefix(l, v+" = ") {
				line = l
			}
		}
		if !strings.Contains(line, jobDir) {
			t.Errorf("%s does not point inside the job dir: %q", v, line)
		}
	}
	if _, err := os.Lstat(filepath.Join(jobDir, "trees", "sysvar")); err == nil {
		t.Error("a missing tree must not get a link")
	}
}

func TestMkTeXHelpersAreDisabled(t *testing.T) {
	cnf := texmfCnf("/job", map[string]string{})
	for _, name := range disabledMkTeX {
		if !strings.Contains(cnf, name+" = 0\n") {
			t.Errorf("%s is not disabled", name)
		}
	}
}
