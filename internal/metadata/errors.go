// Package metadata resolves the Metadata Block of a Document into typed
// field values. It owns block detection, the layering of manifest
// defaults, Configuration Defaults and the Document's own values, the
// choice of Document Type, strict typing, and the structured error list
// that reports every problem at once.
package metadata

import (
	"fmt"
	"strings"
)

// Sources of an error: who has to fix it.
const (
	SourceDocument      = "document"
	SourceConfiguration = "configuration"
	SourceTemplate      = "template"
	SourceCollection    = "collection"
)

// Codes of an error. Consumers must tolerate codes they do not know:
// later stages add their own (for example the image kinds).
const (
	CodeRequired = "required"
	CodeUnknown  = "unknown"
	CodeType     = "type"
	CodeEnum     = "enum"
	// CodeBlock reports a structural problem of the Metadata Block itself:
	// an unclosed fence, broken YAML.
	CodeBlock = "block"
)

// Error is one problem. Field is empty when the problem is not about a
// single field; Line is the line in the Document and is nil when the
// problem has no position (for example a bad Configuration Default).
type Error struct {
	Field   string `json:"field"`
	Source  string `json:"source"`
	Code    string `json:"code"`
	Message string `json:"message"`
	Line    *int   `json:"line"`
}

// String renders one readable line, for example
// "document line 4: subjct: unknown key (unknown)".
func (e Error) String() string {
	var b strings.Builder
	b.WriteString(e.Source)
	if e.Line != nil {
		fmt.Fprintf(&b, " line %d", *e.Line)
	}
	b.WriteString(": ")
	if e.Field != "" {
		b.WriteString(e.Field + ": ")
	}
	fmt.Fprintf(&b, "%s (%s)", e.Message, e.Code)
	return b.String()
}

// ErrorList is the error body: all problems of one Document, collected.
// It marshals to {"errors":[...]}.
type ErrorList struct {
	Errors []Error `json:"errors"`
}

func (l *ErrorList) Error() string {
	lines := make([]string, len(l.Errors))
	for i, e := range l.Errors {
		lines[i] = e.String()
	}
	return strings.Join(lines, "\n")
}

func lineRef(n int) *int { return &n }
