// Command injector is a reverse proxy in front of Outline that adds the
// "Download PDF" button script to its HTML pages.
package main

import (
	"os"

	"github.com/taskmedia/outline-latex-pdf/internal/inject"
)

func main() {
	os.Exit(inject.Main(os.Args[1:], os.Getenv, os.Stdout, os.Stderr))
}
