package worker

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	treesDir     = "trees"
	cnfDir       = "texmf-cnf"
	texmfVarDir  = "texmf-var"
	texmfCfgDir  = "texmf-config"
	homeDir      = "home"
	texFontsDir  = "texfonts"
	texmfCnfFile = "texmf.cnf"
	cnfSeparator = ":"
)

// texTreeVars are the kpathsea variables that name TeX trees on disk. The
// engine is paranoid about absolute paths (openin_any=p), so each tree is
// reached through a symlink inside the job directory and the variable is
// pointed at the symlink.
var texTreeVars = []string{
	"TEXMFDIST", "TEXMFMAIN", "TEXMFDEBIAN", "TEXMFSYSVAR", "TEXMFSYSCONFIG", "TEXMFLOCAL",
}

// disabledMkTeX stops kpathsea from spawning mktex* helper scripts.
var disabledMkTeX = []string{"MKTEXTEX", "MKTEXPK", "MKTEXMF", "MKTEXTFM", "MKTEXFMT", "MKOCP", "MKOFM"}

// texInstall describes the TeX installation of the image, found once per job
// with kpsewhich.
type texInstall struct {
	// ConfigPath is the installation's own TEXMFCNF search path.
	ConfigPath string
	// Trees maps each variable of texTreeVars to its directory, "" if unset.
	Trees map[string]string
}

func discoverInstall(ctx context.Context, jobDir string) (texInstall, error) {
	env := []string{"PATH=" + toolPath, "HOME=" + jobDir}
	query := func(name string) (string, error) {
		res, err := runTool(ctx, jobDir, env, "kpsewhich", []string{"-var-value", name})
		if err != nil {
			return "", fmt.Errorf("kpsewhich %s: %w", name, err)
		}
		return strings.TrimSpace(res.Output), nil
	}
	cfg, err := query("TEXMFCNF")
	if err != nil {
		return texInstall{}, err
	}
	in := texInstall{ConfigPath: strings.ReplaceAll(cfg, ";", cnfSeparator), Trees: map[string]string{}}
	for _, v := range texTreeVars {
		dir, err := query(v)
		if err != nil {
			return texInstall{}, err
		}
		in.Trees[v] = dir
	}
	return in, nil
}

// engineSetup prepares everything LuaLaTeX needs inside jobDir: symlinks to
// the TeX trees and a texmf.cnf of the Worker's own. It returns the TEXMFCNF
// search path, with the Worker's file first so that its values win.
//
// The settings, in short: no shell escape; reads and writes only below the
// job directory (paranoid mode allows absolute paths only below
// TEXMFOUTPUT); no spawning of mktex* scripts.
func engineSetup(jobDir string, in texInstall) (string, error) {
	links, err := linkTrees(jobDir, in)
	if err != nil {
		return "", err
	}
	cnfPath := filepath.Join(jobDir, cnfDir)
	if err := os.Mkdir(cnfPath, 0o700); err != nil {
		return "", fmt.Errorf("create cnf dir: %w", err)
	}
	if err := os.WriteFile(filepath.Join(cnfPath, texmfCnfFile), []byte(texmfCnf(jobDir, links)), 0o600); err != nil {
		return "", fmt.Errorf("write texmf.cnf: %w", err)
	}
	return cnfPath + cnfSeparator + in.ConfigPath, nil
}

// linkTrees creates jobDir/trees/<name> for each existing tree and returns
// the in-job path to use for every variable. Variables whose tree does not
// exist still get a path in the job directory, so the real directory can
// never be searched.
func linkTrees(jobDir string, in texInstall) (map[string]string, error) {
	root := filepath.Join(jobDir, treesDir)
	if err := os.Mkdir(root, 0o700); err != nil {
		return nil, fmt.Errorf("create trees dir: %w", err)
	}
	linkByTarget := map[string]string{}
	links := map[string]string{}
	for _, v := range texTreeVars {
		target := in.Trees[v]
		name := strings.ToLower(strings.TrimPrefix(v, "TEXMF"))
		if existing, ok := linkByTarget[target]; ok && target != "" {
			links[v] = existing
			continue
		}
		link := filepath.Join(root, name)
		links[v] = link
		if target == "" || !dirExists(target) {
			continue
		}
		if err := os.Symlink(target, link); err != nil {
			return nil, fmt.Errorf("link TeX tree %s: %w", v, err)
		}
		linkByTarget[target] = link
	}
	return links, nil
}

func texmfCnf(jobDir string, links map[string]string) string {
	var b strings.Builder
	b.WriteString("% Written by the Worker for one job. First match wins, so these\n")
	b.WriteString("% values override the installation's texmf.cnf.\n")
	b.WriteString("shell_escape = f\nopenin_any = p\nopenout_any = p\n")
	fmt.Fprintf(&b, "TEXMFOUTPUT = %s\n", jobDir)
	names := make([]string, 0, len(links))
	for v := range links {
		names = append(names, v)
	}
	sort.Strings(names)
	for _, v := range names {
		fmt.Fprintf(&b, "%s = %s\n", v, links[v])
	}
	fmt.Fprintf(&b, "VARTEXFONTS = %s\n", filepath.Join(jobDir, texFontsDir))
	b.WriteString("OSFONTDIR =\n")
	for _, v := range disabledMkTeX {
		fmt.Fprintf(&b, "%s = 0\n", v)
	}
	return b.String()
}

const toolPath = "/usr/local/bin:/usr/bin:/bin"

// buildEnv returns the only environment child processes see. Nothing is
// inherited except the reproducibility variables, and only when set.
func buildEnv(jobDir, texmfcnf string, lookup func(string) (string, bool)) []string {
	env := []string{
		"PATH=" + toolPath,
		"LC_ALL=C.UTF-8",
		"HOME=" + filepath.Join(jobDir, homeDir),
		"TMPDIR=" + jobDir,
		"TEXMFVAR=" + filepath.Join(jobDir, texmfVarDir),
		"TEXMFCACHE=" + filepath.Join(jobDir, texmfVarDir),
		"TEXMFCONFIG=" + filepath.Join(jobDir, texmfCfgDir),
		"TEXMFCNF=" + texmfcnf,
	}
	for _, name := range []string{"SOURCE_DATE_EPOCH", "FORCE_SOURCE_DATE"} {
		if v, ok := lookup(name); ok {
			env = append(env, name+"="+v)
		}
	}
	return env
}

// buildSVGEnv is the environment of rsvg-convert: the same rebuilt-from-
// scratch idea as buildEnv, without any TeX variable. In particular nothing
// like XML_CATALOG_FILES or a proxy variable is inherited.
func buildSVGEnv(jobDir string, lookup func(string) (string, bool)) []string {
	env := []string{
		"PATH=" + toolPath,
		"LC_ALL=C.UTF-8",
		"HOME=" + filepath.Join(jobDir, homeDir),
		"TMPDIR=" + jobDir,
	}
	if v, ok := lookup("SOURCE_DATE_EPOCH"); ok {
		env = append(env, "SOURCE_DATE_EPOCH="+v)
	}
	return env
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
