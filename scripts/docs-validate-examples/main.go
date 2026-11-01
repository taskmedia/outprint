// Command docs-validate-examples is the gate for the YAML examples in the
// documentation. Every fenced yaml block (and every markdown block that shows
// a Metadata Block or a Binding block) under docs/ must be preceded by a line
//
//	<!-- validate: MODE key=value ... -->    checked by the real code
//	<!-- validate: skip REASON -->           consciously not checked
//
// The comment is invisible on GitHub and on the site. A block with neither
// marker fails, so a new example is either validated or consciously skipped.
//
// Modes (see check.go): metadata, binding, configuration, service-config,
// service-fragment. The package modes run the built `outprint validate`; the
// Metadata Block and Binding modes call the same resolvers the product uses,
// because the binary has no command for them.
//
// Run through `task docs:examples-check`.
package main

import (
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// generatedDir holds pages that scripts/docs-catalogue writes after running the
// same checks; `docs:catalogue-check` covers them.
const generatedDir = "templates/catalogue"

func main() {
	docs := flag.String("docs", "docs", "directory with the Markdown pages")
	root := flag.String("root", ".", "repository root (templates, configurations, config/service.yaml)")
	bin := flag.String("bin", "bin/outprint", "the built outprint binary")
	verbose := flag.Bool("v", false, "print the disposition of every block")
	flag.Parse()

	files, err := markdownFiles(*docs)
	if err != nil {
		die(err)
	}
	c, err := newChecker(*root, *bin)
	if err != nil {
		die(err)
	}
	defer c.cleanup()

	var findings []string
	validated, skipped := 0, 0
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			die(err)
		}
		blocks, problems := scan(f, string(data))
		findings = append(findings, problems...)
		for _, b := range blocks {
			status, problem := c.check(b)
			if *verbose {
				fmt.Printf("%s:%d %s\n", b.file, b.line, status)
			}
			switch {
			case problem != "":
				findings = append(findings, problem)
			case strings.HasPrefix(status, "skip"):
				skipped++
			default:
				validated++
			}
		}
	}
	sort.Strings(findings)
	for _, f := range findings {
		fmt.Println("FAIL: " + f)
	}
	if len(findings) > 0 {
		fmt.Printf("docs-validate-examples: %d finding(s)\n", len(findings))
		c.cleanup()
		os.Exit(1)
	}
	fmt.Printf("docs-validate-examples: ok, %d validated, %d skipped\n", validated, skipped)
}

func die(err error) {
	fmt.Fprintf(os.Stderr, "docs-validate-examples: %v\n", err)
	os.Exit(2)
}

func markdownFiles(dir string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && strings.HasSuffix(filepath.ToSlash(path), generatedDir) {
			return filepath.SkipDir
		}
		if !d.IsDir() && strings.HasSuffix(path, ".md") {
			out = append(out, path)
		}
		return nil
	})
	sort.Strings(out)
	return out, err
}
