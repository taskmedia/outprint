package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/taskmedia/outprint/internal/binding"
	"github.com/taskmedia/outprint/internal/config"
	"github.com/taskmedia/outprint/internal/metadata"
	"github.com/taskmedia/outprint/internal/packages"
)

const sampleDate = "2026-03-02"

// checker validates blocks against the packages of the repository.
type checker struct {
	root, bin string
	tmp       string
	registry  *packages.Registry
}

func newChecker(root, bin string) (*checker, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	bin, err = filepath.Abs(bin)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(bin); err != nil {
		return nil, fmt.Errorf("binary %s not found (build it first: go build -o bin/outprint ./cmd/outprint)", bin)
	}
	tmp, err := os.MkdirTemp("", "docs-validate-")
	if err != nil {
		return nil, err
	}
	c := &checker{root: root, bin: bin, tmp: tmp}
	return c, nil
}

func (c *checker) cleanup() { os.RemoveAll(c.tmp) }

func (c *checker) templatesRoot() string      { return filepath.Join(c.root, "templates") }
func (c *checker) configurationsRoot() string { return filepath.Join(c.root, "configurations") }
func (c *checker) servicePath() string        { return filepath.Join(c.root, "config", "service.yaml") }

// check returns the disposition of a block (for -v) and, when the block
// fails the gate, the finding to print.
func (c *checker) check(b block) (status, problem string) {
	at := fmt.Sprintf("%s:%d", b.file, b.line)
	if !b.hasMarker {
		return "unmarked", at + ": yaml block has no `<!-- validate: ... -->` marker line directly before it (use a mode, or `skip REASON`)"
	}
	fields := strings.Fields(b.directive)
	if len(fields) == 0 {
		return "invalid", at + ": empty validate marker"
	}
	mode, args := fields[0], fields[1:]
	if mode == "skip" {
		if len(args) == 0 {
			return "invalid", at + ": `validate: skip` needs a reason"
		}
		return "skip " + strings.Join(args, " "), ""
	}
	opts, err := parseOptions(args)
	if err != nil {
		return "invalid", fmt.Sprintf("%s: %v", at, err)
	}
	var msg string
	switch mode {
	case "metadata":
		msg, err = c.metadata(b, opts)
	case "binding":
		msg, err = c.binding(b)
	case "configuration":
		msg, err = c.configuration(b, opts)
	case "service-config":
		msg, err = c.serviceConfig(b.content)
	case "service-fragment":
		msg, err = c.serviceFragment(b.content)
	default:
		return "invalid", fmt.Sprintf("%s: unknown validate mode %q", at, mode)
	}
	label := mode + " " + strings.Join(args, " ")
	if err != nil {
		return label, fmt.Sprintf("%s: [%s] %v", at, strings.TrimSpace(label), err)
	}
	if msg != "" {
		return label, fmt.Sprintf("%s: [%s] %s", at, strings.TrimSpace(label), msg)
	}
	return label, ""
}

func parseOptions(args []string) (map[string]string, error) {
	opts := map[string]string{}
	for _, a := range args {
		k, v, ok := strings.Cut(a, "=")
		if !ok || k == "" || v == "" {
			return nil, fmt.Errorf("option %q is not key=value", a)
		}
		opts[k] = v
	}
	return opts, nil
}

func need(opts map[string]string, keys ...string) error {
	for _, k := range keys {
		if opts[k] == "" {
			return fmt.Errorf("the marker needs %s=NAME", k)
		}
	}
	return nil
}

func (c *checker) loadRegistry() (*packages.Registry, error) {
	if c.registry != nil {
		return c.registry, nil
	}
	reg, findings := packages.Inspect(packages.Roots{Templates: c.templatesRoot(), Configurations: c.configurationsRoot()}, "", "")
	if errs := packages.Errors(findings); len(errs) > 0 {
		return nil, fmt.Errorf("the packages under %s do not validate: %v", c.root, errs[0])
	}
	c.registry = reg
	return reg, nil
}

// yamlAsDocument wraps a yaml block the way an author writes it: as the first
// block of a Document or of a Collection overview.
func yamlAsDocument(b block) string {
	if b.lang == "markdown" {
		return b.content
	}
	return "```yaml\n" + b.content + "```\nBody.\n"
}

func (c *checker) metadata(b block, opts map[string]string) (string, error) {
	if err := need(opts, "template", "configuration"); err != nil {
		return "", err
	}
	reg, err := c.loadRegistry()
	if err != nil {
		return "", err
	}
	t, ok := reg.Template(opts["template"])
	if !ok {
		return "", fmt.Errorf("template %q not found", opts["template"])
	}
	cfg, ok := reg.Configuration(opts["configuration"])
	if !ok {
		return "", fmt.Errorf("configuration %q not found", opts["configuration"])
	}
	_, err = metadata.Resolve(metadata.Input{
		Markdown: []byte(yamlAsDocument(b)), Title: "Untitled", Modified: sampleDate,
		Template: t, Configuration: cfg,
	})
	return "", errorList(err)
}

func (c *checker) binding(b block) (string, error) {
	svc, errs := config.Load(c.servicePath(), c.rootsEnv)
	if len(errs) > 0 {
		return "", fmt.Errorf("config/service.yaml: %w", errors.Join(errs...))
	}
	reg, findings := packages.Load(svc)
	if errs := packages.Errors(findings); len(errs) > 0 {
		return "", fmt.Errorf("the packages do not validate: %v", errs[0])
	}
	_, list := binding.Resolve(yamlAsDocument(b), reg)
	if list == nil {
		return "", nil
	}
	return "", errorList(list)
}

// errorList turns a metadata error list into one error with a line per problem.
func errorList(err error) error {
	if err == nil {
		return nil
	}
	var list *metadata.ErrorList
	if errors.As(err, &list) {
		var lines []string
		for _, e := range list.Errors {
			lines = append(lines, "\n    "+e.String())
		}
		return errors.New(strings.Join(lines, ""))
	}
	return err
}

// configuration replaces configuration.yaml of a copy of a Configuration
// package and runs the real `outprint validate` on it.
func (c *checker) configuration(b block, opts map[string]string) (string, error) {
	if err := need(opts, "template", "configuration"); err != nil {
		return "", err
	}
	root, err := os.MkdirTemp(c.tmp, "configs-")
	if err != nil {
		return "", err
	}
	name := opts["configuration"]
	if err := copyDir(filepath.Join(c.configurationsRoot(), name), filepath.Join(root, name)); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(root, name, "configuration.yaml"), []byte(b.content), 0o600); err != nil {
		return "", err
	}
	return "", c.validate("--templates-root", c.templatesRoot(), "--configs-root", root,
		"--template", opts["template"], "--config", name)
}

func (c *checker) serviceConfig(text string) (string, error) {
	f, err := os.CreateTemp(c.tmp, "service-*.yaml")
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := f.WriteString(text); err != nil {
		return "", err
	}
	return "", c.validate("--service-config", f.Name())
}

// serviceFragment completes a fragment (admission, imageHosts) with the
// shipped config/service.yaml, which supplies the roots and the allow-list.
func (c *checker) serviceFragment(fragment string) (string, error) {
	base, err := os.ReadFile(c.servicePath())
	if err != nil {
		return "", err
	}
	return c.serviceConfig(string(base) + "\n" + fragment)
}

func (c *checker) rootsEnv(key string) string {
	switch key {
	case config.EnvTemplatesRoot:
		return c.templatesRoot()
	case config.EnvConfigurationsRoot:
		return c.configurationsRoot()
	}
	return os.Getenv(key)
}

// validate runs `outprint validate` and returns its output as the error.
func (c *checker) validate(args ...string) error {
	cmd := exec.Command(c.bin, append([]string{"validate"}, args...)...)
	cmd.Env = append(os.Environ(),
		config.EnvTemplatesRoot+"="+c.templatesRoot(),
		config.EnvConfigurationsRoot+"="+c.configurationsRoot())
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("outprint validate failed:\n    %s", strings.ReplaceAll(strings.TrimSpace(out.String()), "\n", "\n    "))
	}
	return nil
}

func copyDir(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dst, 0o700); err != nil {
		return err
	}
	for _, e := range entries {
		from, to := filepath.Join(src, e.Name()), filepath.Join(dst, e.Name())
		if e.IsDir() {
			if err := copyDir(from, to); err != nil {
				return err
			}
			continue
		}
		data, err := os.ReadFile(from)
		if err != nil {
			return err
		}
		if err := os.WriteFile(to, data, 0o600); err != nil {
			return err
		}
	}
	return nil
}
