// Command docs-catalogue generates the Template catalogue pages of the
// documentation from the Template packages under templates/.
//
// It loads the packages with the same loader as `outprint validate`, builds a
// ready-to-copy Metadata Block for every Document Type, checks that block with
// the real metadata resolver, and records the real messages of typical
// mistakes. A page can therefore not drift from the packages: `-check`
// regenerates in memory and fails if a file on disk differs.
//
// Run it from the repository root through `task docs:catalogue` and
// `task docs:catalogue-check`.
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

type options struct {
	templates      string
	configurations string
	configuration  string
	out            string
	check          bool
	printExample   string
}

func main() {
	var o options
	flag.StringVar(&o.templates, "templates", "templates", "Templates root")
	flag.StringVar(&o.configurations, "configurations", "configurations", "Configurations root")
	flag.StringVar(&o.configuration, "config", "example", "Configuration that the examples are built against")
	flag.StringVar(&o.out, "out", "docs/templates/catalogue", "output directory")
	flag.BoolVar(&o.check, "check", false, "do not write; fail if a generated file differs from disk")
	flag.StringVar(&o.printExample, "print-example", "", "print the example document of TEMPLATE[:TYPE] and exit (the preview input)")
	flag.Parse()
	if err := run(o); err != nil {
		fmt.Fprintf(os.Stderr, "docs-catalogue: %v\n", err)
		os.Exit(1)
	}
}

func run(o options) error {
	cat, err := load(o)
	if err != nil {
		return err
	}
	if o.printExample != "" {
		doc, err := cat.exampleDocument(o.printExample)
		if err != nil {
			return err
		}
		_, err = os.Stdout.WriteString(doc)
		return err
	}
	files, err := cat.pages(o.out)
	if err != nil {
		return err
	}
	if o.check {
		return checkDrift(files)
	}
	return writeFiles(files)
}

// writeFiles writes every page. Files in the output directory that no longer
// belong to a Template are removed, but only generated ones: a hand-written
// intro (a file whose name starts with an underscore) is never touched.
func writeFiles(files map[string][]byte) error {
	dirs := map[string]bool{}
	for path, content := range files {
		dirs[filepath.Dir(path)] = true
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, content, 0o644); err != nil {
			return err
		}
	}
	for dir := range dirs {
		stale, err := staleGenerated(dir, files)
		if err != nil {
			return err
		}
		for _, path := range stale {
			if err := os.Remove(path); err != nil {
				return err
			}
		}
	}
	return nil
}

func checkDrift(files map[string][]byte) error {
	var drift []string
	for path, want := range files {
		got, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(got, want) {
			drift = append(drift, path)
		}
	}
	for dir := range dirsOf(files) {
		stale, err := staleGenerated(dir, files)
		if err != nil {
			return err
		}
		drift = append(drift, stale...)
	}
	if len(drift) == 0 {
		return nil
	}
	sort.Strings(drift)
	for _, path := range drift {
		fmt.Fprintf(os.Stderr, "  out of date: %s\n", path)
	}
	return errors.New("generated pages differ from the packages; run `task docs:catalogue` and commit the result")
}

func dirsOf(files map[string][]byte) map[string]bool {
	dirs := map[string]bool{}
	for path := range files {
		dirs[filepath.Dir(path)] = true
	}
	return dirs
}

// staleGenerated lists generated pages in dir that the run did not produce.
func staleGenerated(dir string, files map[string][]byte) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var stale []string
	for _, e := range entries {
		path := filepath.Join(dir, e.Name())
		if e.IsDir() || filepath.Ext(path) != ".md" || e.Name()[0] == '_' {
			continue
		}
		if _, ok := files[path]; ok {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		if bytes.Contains(data, []byte(generatedMarker)) {
			stale = append(stale, path)
		}
	}
	return stale, nil
}
