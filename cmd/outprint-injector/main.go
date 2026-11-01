// Command outprint-injector is a reverse proxy in front of Outline that adds the
// "Formatted PDF" action script to its HTML pages.
package main

import (
	"os"

	"github.com/taskmedia/outprint/internal/inject"
)

func main() {
	os.Exit(inject.Main(os.Args[1:], os.Getenv, os.Stdout, os.Stderr))
}
