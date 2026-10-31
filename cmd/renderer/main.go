// Command renderer turns Markdown into typeset PDFs.
package main

import (
	"os"

	"github.com/taskmedia/outline-latex-pdf/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stderr))
}
