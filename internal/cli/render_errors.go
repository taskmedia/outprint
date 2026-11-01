package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/taskmedia/outprint/internal/metadata"
	"github.com/taskmedia/outprint/internal/worker"
)

// reportRenderError prints metadata problems readably, or as the JSON error
// body with --json-errors, worker failures with a hint, and any other
// failure as a single line.
func reportRenderError(err error, opts renderOptions, stdout, stderr io.Writer) {
	var list *metadata.ErrorList
	var failure *worker.Error
	switch {
	case errors.As(err, &list):
		reportMetadataErrors(list, opts, stdout, stderr)
	case errors.As(err, &failure):
		reportWorkerError(failure, opts, stderr)
	default:
		fmt.Fprintf(stderr, "renderer: %v\n", err)
	}
}

func reportMetadataErrors(list *metadata.ErrorList, opts renderOptions, stdout, stderr io.Writer) {
	if opts.JSONErrors {
		if err := json.NewEncoder(stdout).Encode(list); err != nil {
			fmt.Fprintf(stderr, "renderer: encode errors: %v\n", err)
		}
		return
	}
	fmt.Fprintf(stderr, "renderer: %s has %d problem(s):\n", opts.Input, len(list.Errors))
	for _, e := range list.Errors {
		fmt.Fprintf(stderr, "  %s\n", e)
	}
}

func reportWorkerError(e *worker.Error, opts renderOptions, stderr io.Writer) {
	fmt.Fprintf(stderr, "renderer: %s\n", e)
	if hint := workerHint(e, opts); hint != "" {
		fmt.Fprintf(stderr, "  %s\n", hint)
	}
}

func workerHint(e *worker.Error, opts renderOptions) string {
	switch e.Kind {
	case worker.KindTimeout:
		return fmt.Sprintf("the render did not finish in time; raise --timeout (now %s) or shorten the document", opts.Timeout)
	case worker.KindTeXError:
		return "LaTeX could not typeset the document; --debug-tex-log prints the full log (it may contain document content)"
	case worker.KindRawTeX, worker.KindRawFormat, worker.KindMath:
		return "this is an internal error: content that must never reach LaTeX got past the reader; please report it"
	case worker.KindSVGConvert, worker.KindOutputTooLarge:
		return "an SVG image could not be converted to a PDF; simplify it, or export it as PNG"
	case worker.KindImagePath:
		return "an image does not refer to a file of the job; images must be uploaded to Outline"
	}
	return ""
}
