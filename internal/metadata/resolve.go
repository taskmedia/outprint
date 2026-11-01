package metadata

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/taskmedia/outprint/internal/packages"
)

const (
	typeKey            = "type"
	configTypeKey      = "document_type"
	derivedTitleName   = "title"
	derivedSubjectName = "subject"
	derivedDateName    = "date"
)

// Input is everything Resolve needs. Title and Modified are the derived
// values: the Document's title and its last-modified date (ISO, YYYY-MM-DD).
// Both may be empty. The title also defaults the `subject` field.
type Input struct {
	Markdown      []byte
	Title         string
	Modified      string
	Template      packages.Template
	Configuration packages.Configuration
}

// Result is the Document without its Metadata Block and the merged field
// values of its Document Type. Values hold a string for text-like fields, a
// []string for lists and a bool for bools. A date is the ISO date or the
// free text the author wrote; the Template formats it by `language`.
type Result struct {
	Body         []byte
	DocumentType string
	Values       map[string]any
}

// Resolve parses the Metadata Block, picks the Document Type and merges the
// layers: manifest default, then the derived value, then Configuration
// Defaults, then the Document, with Fixed Values on top. An empty Document
// value suppresses everything below it. The error, if any, is an
// *ErrorList holding every problem found.
func Resolve(in Input) (Result, error) {
	r := &resolver{in: in, values: map[string]any{}}
	res := r.run()
	if len(r.errs) > 0 {
		return Result{}, &ErrorList{Errors: r.errs}
	}
	return res, nil
}

type resolver struct {
	in      Input
	errs    []Error
	entries map[string]entry
	values  map[string]any
}

func (r *resolver) run() Result {
	split := splitBlock(string(r.in.Markdown))
	r.errs = append(r.errs, split.errs...)
	var entries []entry
	if split.hasBlock {
		var errs []Error
		entries, errs = parseEntries(split.yaml, split.yamlOffset)
		r.errs = append(r.errs, errs...)
	}
	r.entries = indexEntries(entries)
	r.checkDerivedDate()

	docType, ok := r.resolveDocumentType()
	if !ok {
		// Without a type the fields are unknown; still catch typos and
		// Fixed Values against everything the Template declares.
		r.checkKeys(entries, r.allFields(), "")
		return Result{}
	}
	fields, _ := r.in.Template.FieldsOf(docType)
	r.checkKeys(entries, fields, docType)
	for _, f := range fields {
		r.resolveField(f)
	}
	return Result{Body: []byte(split.body), DocumentType: docType, Values: r.values}
}

func (r *resolver) add(field, source, code, msg string, line int) {
	e := Error{Field: field, Source: source, Code: code, Message: msg}
	if line > 0 {
		e.Line = lineRef(line)
	}
	r.errs = append(r.errs, e)
}

func (r *resolver) addProblem(field, source string, p *problem, fallbackLine int) {
	line := p.line
	if line == 0 {
		line = fallbackLine
	}
	r.add(field, source, p.code, p.msg, line)
}

func (r *resolver) checkDerivedDate() {
	if r.in.Modified == "" {
		return
	}
	if _, err := time.Parse(isoDateLayout, r.in.Modified); err != nil {
		r.add(derivedDateName, SourceDocument, CodeType, fmt.Sprintf("the last-modified date %q is not an ISO date (YYYY-MM-DD)", r.in.Modified), 0)
	}
}

// resolveDocumentType picks the Document's `type`, else the Configuration's
// default. ok is false if there is none or it is unknown.
func (r *resolver) resolveDocumentType() (string, bool) {
	name, source, line, ok := r.requestedType()
	if !ok {
		return "", false
	}
	if _, known := r.in.Template.DocumentTypes[name]; !known {
		available := sortedNames(r.in.Template.DocumentTypes)
		r.add(typeKey, source, CodeUnknown,
			fmt.Sprintf("unknown document type %q (available: %s)", name, strings.Join(available, ", ")), line)
		return "", false
	}
	return name, true
}

func (r *resolver) requestedType() (name, source string, line int, ok bool) {
	if e, has := r.entries[typeKey]; has {
		c, p := textFromNode(packages.Field{Type: packages.TypeString}, e.node, e.valueLine)
		switch {
		case p != nil:
			r.addProblem(typeKey, SourceDocument, p, e.valueLine)
		case c.empty:
			r.add(typeKey, SourceDocument, CodeRequired, "the document type is empty; set it to one of the Template's document types", e.valueLine)
		default:
			return c.value.(string), SourceDocument, e.valueLine, true
		}
		return "", "", 0, false
	}
	if def, has := r.in.Configuration.Defaults[configTypeKey].(string); has && def != "" {
		return def, SourceConfiguration, 0, true
	}
	r.add(typeKey, SourceDocument, CodeRequired, "no document type: set `type` in the metadata block or a document_type default in the Configuration", 0)
	return "", "", 0, false
}

// checkKeys rejects keys of the block that are not fields of the Document
// Type: unknown names (with a hint when one is close) and Fixed Values.
func (r *resolver) checkKeys(entries []entry, fields []packages.Field, docType string) {
	names := make([]string, len(fields))
	known := map[string]bool{typeKey: true}
	for i, f := range fields {
		names[i], known[f.Name] = f.Name, true
	}
	for _, e := range entries {
		_, fixed := r.in.Configuration.Fixed[e.key]
		switch {
		case e.key == typeKey:
		case fixed:
			r.add(e.key, SourceDocument, CodeUnknown,
				fmt.Sprintf("%q is a Fixed Value of the Configuration and cannot be set in a Document", e.key), e.line)
		case !known[e.key]:
			r.add(e.key, SourceDocument, CodeUnknown, unknownKeyMessage(e.key, docType, names), e.line)
		}
	}
}

func unknownKeyMessage(key, docType string, names []string) string {
	msg := fmt.Sprintf("unknown key %q: not a field of this Template", key)
	if docType != "" {
		msg = fmt.Sprintf("unknown key %q: not a field of document type %q", key, docType)
	}
	if hint := suggest(key, append(names, typeKey)); hint != "" {
		msg += fmt.Sprintf(" (did you mean %q?)", hint)
	}
	return msg
}

func (r *resolver) allFields() []packages.Field {
	fields := make([]packages.Field, 0, len(r.in.Template.Fields))
	for _, name := range sortedNames(r.in.Template.Fields) {
		fields = append(fields, r.in.Template.Fields[name])
	}
	return fields
}

// layer is a candidate value for a field and where it came from.
type layer struct {
	source  string
	value   coerced
	line    int
	problem *problem
}

// resolveField takes the highest layer that has anything to say, checks
// only that one, and stores the result. An empty Document value wins and
// removes the field.
func (r *resolver) resolveField(f packages.Field) {
	chosen, found := r.pick(f)
	if found && chosen.problem != nil {
		r.addProblem(f.Name, chosen.source, chosen.problem, chosen.line)
		return
	}
	if found && !chosen.value.empty {
		value, p := finalize(f, chosen.value)
		if p != nil {
			r.addProblem(f.Name, chosen.source, p, chosen.line)
			return
		}
		r.values[f.Name] = value
		return
	}
	if f.Required {
		r.reportMissing(f, found)
	}
}

func (r *resolver) reportMissing(f packages.Field, suppressed bool) {
	msg := fmt.Sprintf("required field %q is missing", f.Name)
	line := 0
	if suppressed {
		msg = fmt.Sprintf("required field %q is empty", f.Name)
		line = r.entries[f.Name].valueLine
	}
	r.add(f.Name, SourceDocument, CodeRequired, msg, line)
}

func (r *resolver) pick(f packages.Field) (layer, bool) {
	if v, ok := r.in.Configuration.Fixed[f.Name]; ok {
		return nativeLayer(f, v, SourceConfiguration), true
	}
	if e, ok := r.entries[f.Name]; ok {
		c, p := fromNode(f, e)
		return layer{source: SourceDocument, value: c, line: e.valueLine, problem: p}, true
	}
	if v, ok := r.in.Configuration.Defaults[f.Name]; ok {
		return nativeLayer(f, v, SourceConfiguration), true
	}
	if derived, ok := r.derived(f); ok {
		return layer{source: SourceDocument, value: coerced{value: derived}}, true
	}
	if f.HasDefault {
		return nativeLayer(f, f.Default, SourceTemplate), true
	}
	return layer{}, false
}

func nativeLayer(f packages.Field, raw any, source string) layer {
	c, p := fromNative(f, raw)
	return layer{source: source, value: c, problem: p}
}

func (r *resolver) derived(f packages.Field) (string, bool) {
	switch {
	case (f.Name == derivedTitleName || f.Name == derivedSubjectName) && r.in.Title != "":
		return r.in.Title, true
	case f.Name == derivedDateName && r.in.Modified != "":
		return r.in.Modified, true
	}
	return "", false
}

func sortedNames[V any](m map[string]V) []string {
	names := make([]string, 0, len(m))
	for n := range m {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
