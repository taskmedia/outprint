// Package binding resolves the Binding of a Collection: which Template and
// which Template Configuration render the Documents in it. The Collection
// admin chooses by placing a fenced `yaml` block with the root key `binding`
// in the Collection overview; the choice is validated against the loaded packages and the
// installation's allow-list on every call.
//
// There is no silent fallback. The default Binding applies only when the
// overview holds no block at all; a broken block is reported, never
// replaced by the default.
package binding

import (
	"fmt"

	"github.com/taskmedia/outline-latex-pdf/internal/config"
	"github.com/taskmedia/outline-latex-pdf/internal/metadata"
	"github.com/taskmedia/outline-latex-pdf/internal/packages"
)

// Codes of a Binding error. Their source is always "collection".
const (
	// CodeMalformed: the block is not closed, is not YAML, does not hold
	// exactly the key binding with a mapping of exactly the keys template and
	// configuration, or a value is not a name.
	CodeMalformed = "binding_malformed"
	// CodeUnknown: a Template or Configuration of that name is not installed.
	CodeUnknown = "binding_unknown"
	// CodeNotAllowed: both names exist but the pair is not on the allow-list.
	CodeNotAllowed = "binding_not_allowed"
	// CodeDuplicate: a second block with the root key `binding`.
	CodeDuplicate = "binding_duplicate"
)

// Registry is what the resolver needs from the loaded packages.
type Registry interface {
	Template(name string) (packages.Template, bool)
	Configuration(name string) (packages.Configuration, bool)
	Allowed(b config.Binding) bool
	DefaultBinding() (config.Binding, bool)
}

// Kind says where the outcome of a resolution comes from.
type Kind int

const (
	// FromBlock: the overview holds a block. Binding is set when the block
	// is valid; otherwise the errors say what is wrong and Binding is zero.
	FromBlock Kind = iota
	// UseDefault: the overview holds no block and the installation has a
	// default Binding, which is in Binding.
	UseDefault
	// NoBinding: no block and no default Binding.
	NoBinding
)

// Outcome is the result of a resolution.
type Outcome struct {
	Kind    Kind
	Binding config.Binding
}

// Resolve reads the Collection overview (Markdown) and returns the Binding
// to use. The returned list is nil when there is nothing to report and
// holds every problem otherwise. Lines count from 1 in the overview.
func Resolve(overview string, reg Registry) (Outcome, *metadata.ErrorList) {
	blocks := bindingBlocks(overview)
	if len(blocks) == 0 {
		return resolveWithoutBlock(reg), nil
	}
	b, errs := parseBlock(blocks[0])
	if len(errs) == 0 {
		errs = checkAgainst(b, blocks[0], reg)
	}
	errs = append(errs, duplicateErrors(blocks[1:])...)
	if len(errs) > 0 {
		return Outcome{Kind: FromBlock}, &metadata.ErrorList{Errors: errs}
	}
	return Outcome{Kind: FromBlock, Binding: b}, nil
}

func resolveWithoutBlock(reg Registry) Outcome {
	if def, ok := reg.DefaultBinding(); ok {
		return Outcome{Kind: UseDefault, Binding: def}
	}
	return Outcome{Kind: NoBinding}
}

func bindingBlocks(overview string) []codeBlock {
	var out []codeBlock
	for _, b := range fencedBlocks(overview) {
		if b.info == bindingInfo && b.hasRootKey(bindingKey) {
			out = append(out, b)
		}
	}
	return out
}

func duplicateErrors(extra []codeBlock) []metadata.Error {
	var errs []metadata.Error
	for _, b := range extra {
		errs = append(errs, newError("", CodeDuplicate, b.openLine,
			"second binding block: a Collection overview has at most one"))
	}
	return errs
}

// checkAgainst verifies the names and the pair. Lines point at the key.
func checkAgainst(b config.Binding, blk codeBlock, reg Registry) []metadata.Error {
	var errs []metadata.Error
	_, okT := reg.Template(b.Template)
	_, okC := reg.Configuration(b.Configuration)
	if !okT {
		errs = append(errs, newError(keyTemplate, CodeUnknown, keyLine(blk, keyTemplate),
			fmt.Sprintf("unknown template %q", b.Template)))
	}
	if !okC {
		errs = append(errs, newError(keyConfiguration, CodeUnknown, keyLine(blk, keyConfiguration),
			fmt.Sprintf("unknown configuration %q", b.Configuration)))
	}
	if okT && okC && !reg.Allowed(b) {
		errs = append(errs, newError("", CodeNotAllowed, keyLine(blk, keyTemplate),
			fmt.Sprintf("template %q with configuration %q is not allowed on this installation", b.Template, b.Configuration)))
	}
	return errs
}

func newError(field, code string, line int, msg string) metadata.Error {
	return metadata.Error{Field: field, Source: metadata.SourceCollection, Code: code, Message: msg, Line: &line}
}
