// Command outprint turns Markdown into typeset PDFs.
package main

import (
	"os"

	"github.com/taskmedia/outprint/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))
}
