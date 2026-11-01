// Package bundle is the job bundle: the only contract between the Gateway
// and the Worker. It carries the Document body, the merged field values, the
// names of the Template, Document Type and Configuration, and the Document's
// images. It has no field for a cookie, a URL or a secret, so none can be
// sent by accident.
package bundle

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"

	"github.com/taskmedia/outline-latex-pdf/internal/config"
	"github.com/taskmedia/outline-latex-pdf/internal/limits"
)

// Limits of a bundle. The Gateway pre-checks them to answer 413; the Worker
// enforces them again because it trusts nobody.
const (
	MaxMarkdownBytes   = limits.MaxMarkdownBytes
	MaxImages          = limits.MaxImages
	MaxImageBytes      = limits.MaxImageBytes
	MaxImagesTotal     = limits.MaxImagesTotal
	MaxValues          = 200
	MaxValueBytes      = 64 << 10
	MaxListItems       = 200
	MaxMetadataBytes   = 1 << 20
	maxJSONEscapeRatio = 6
	base64Numerator    = 4
	base64Denominator  = 3
	envelopeSlack      = 1 << 20

	// MaxEncodedBytes is the most an encoded bundle can take: Markdown with
	// every byte JSON-escaped, base64 images, values and some slack. The
	// Worker caps the request body here.
	MaxEncodedBytes = maxJSONEscapeRatio*MaxMarkdownBytes +
		MaxImagesTotal*base64Numerator/base64Denominator + MaxImages*256 +
		MaxMetadataBytes + envelopeSlack
)

var (
	valueNameRE = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	// ImageNameRE is what a bundle image name must match; it is the same
	// rule the Worker applies when writing the job directory.
	ImageNameRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*(\.[A-Za-z0-9_-]+)?$`)
	typeNameRE  = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)
)

// Bundle is one render job as the Gateway hands it to the Worker. Values
// hold a string, a []string or a bool per field, as metadata.Resolve
// produces them.
type Bundle struct {
	Markdown      []byte
	Values        map[string]any
	Template      string
	DocumentType  string
	Configuration string
	Images        map[string][]byte
}

// wire is the JSON form: the Markdown is a plain string so a bundle can be
// read by a human; images are base64.
type wire struct {
	Markdown      string            `json:"markdown"`
	Values        map[string]any    `json:"values"`
	Template      string            `json:"template"`
	DocumentType  string            `json:"documentType"`
	Configuration string            `json:"configuration"`
	Images        map[string][]byte `json:"images,omitempty"`
}

// LimitError reports that a bundle exceeds one of the limits. It is the
// difference between "your Document is too large" (413) and "the bundle is
// malformed" (400).
type LimitError struct{ What string }

func (e *LimitError) Error() string { return "bundle exceeds limit: " + e.What }

// Encode serialises the bundle as JSON.
func (b Bundle) Encode() ([]byte, error) {
	data, err := json.Marshal(wire{
		Markdown: string(b.Markdown), Values: b.Values, Template: b.Template,
		DocumentType: b.DocumentType, Configuration: b.Configuration, Images: b.Images,
	})
	if err != nil {
		return nil, fmt.Errorf("encode bundle: %w", err)
	}
	return data, nil
}

// Decode reads one bundle, rejects unknown fields and trailing data,
// normalises the value types and validates the result. A read error from r
// (including http.MaxBytesError) is returned as is.
func Decode(r io.Reader) (Bundle, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return Bundle{}, err
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var w wire
	if err := dec.Decode(&w); err != nil {
		return Bundle{}, fmt.Errorf("decode bundle: %w", err)
	}
	b := Bundle{
		Markdown: []byte(w.Markdown), Values: w.Values, Template: w.Template,
		DocumentType: w.DocumentType, Configuration: w.Configuration, Images: w.Images,
	}
	if dec.More() {
		return Bundle{}, errors.New("decode bundle: trailing data")
	}
	if b.Values, err = normaliseValues(b.Values); err != nil {
		return Bundle{}, err
	}
	if err := b.Validate(); err != nil {
		return Bundle{}, err
	}
	return b, nil
}

// Validate checks names, value types and sizes. It returns a *LimitError
// for a size problem and a plain error for anything else.
func (b Bundle) Validate() error {
	if err := b.validateNames(); err != nil {
		return err
	}
	if err := b.validateValues(); err != nil {
		return err
	}
	if len(b.Markdown) > MaxMarkdownBytes {
		return &LimitError{What: fmt.Sprintf("markdown is %d bytes, at most %d", len(b.Markdown), MaxMarkdownBytes)}
	}
	return b.validateImages()
}

func (b Bundle) validateNames() error {
	for _, n := range []struct{ what, name string }{
		{"template", b.Template}, {"configuration", b.Configuration},
	} {
		if !config.NamePattern.MatchString(n.name) {
			return fmt.Errorf("%s name %q is not a valid identifier", n.what, n.name)
		}
	}
	if !typeNameRE.MatchString(b.DocumentType) {
		return fmt.Errorf("document type %q is not a valid identifier", b.DocumentType)
	}
	return nil
}

func (b Bundle) validateValues() error {
	if len(b.Values) > MaxValues {
		return &LimitError{What: fmt.Sprintf("%d values, at most %d", len(b.Values), MaxValues)}
	}
	total := 0
	for _, name := range sortedKeys(b.Values) {
		if !valueNameRE.MatchString(name) {
			return fmt.Errorf("value name %q is not a valid identifier", name)
		}
		size, err := valueSize(name, b.Values[name])
		if err != nil {
			return err
		}
		total += size
		if size > MaxValueBytes {
			return &LimitError{What: fmt.Sprintf("value %q is %d bytes, at most %d", name, size, MaxValueBytes)}
		}
	}
	if total > MaxMetadataBytes {
		return &LimitError{What: fmt.Sprintf("values are %d bytes, at most %d", total, MaxMetadataBytes)}
	}
	return nil
}

func valueSize(name string, v any) (int, error) {
	switch v := v.(type) {
	case string:
		return len(v), nil
	case bool:
		return 1, nil
	case []string:
		if len(v) > MaxListItems {
			return 0, &LimitError{What: fmt.Sprintf("value %q has %d items, at most %d", name, len(v), MaxListItems)}
		}
		size := 0
		for _, s := range v {
			size += len(s)
		}
		return size, nil
	}
	return 0, fmt.Errorf("value %q has an unsupported type %T", name, v)
}

func (b Bundle) validateImages() error {
	if len(b.Images) > MaxImages {
		return &LimitError{What: fmt.Sprintf("%d images, at most %d", len(b.Images), MaxImages)}
	}
	total := 0
	for _, name := range sortedKeys(b.Images) {
		if !ImageNameRE.MatchString(name) {
			return fmt.Errorf("image name %q is not valid", name)
		}
		size := len(b.Images[name])
		if size > MaxImageBytes {
			return &LimitError{What: fmt.Sprintf("image %q is %d bytes, at most %d", name, size, MaxImageBytes)}
		}
		total += size
	}
	if total > MaxImagesTotal {
		return &LimitError{What: fmt.Sprintf("images are %d bytes, at most %d", total, MaxImagesTotal)}
	}
	return nil
}

// normaliseValues turns what JSON decoding produces ([]any of strings) back
// into the types metadata.Resolve produces.
func normaliseValues(in map[string]any) (map[string]any, error) {
	out := make(map[string]any, len(in))
	for name, v := range in {
		list, isList := v.([]any)
		if !isList {
			out[name] = v
			continue
		}
		strs := make([]string, len(list))
		for i, item := range list {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("value %q: list items must be strings", name)
			}
			strs[i] = s
		}
		out[name] = strs
	}
	return out, nil
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// String describes the bundle for logs without any of its content.
func (b Bundle) String() string {
	return fmt.Sprintf("bundle{%s/%s/%s, %d bytes, %d values, %d images}",
		b.Template, b.DocumentType, b.Configuration, len(b.Markdown), len(b.Values), len(b.Images))
}
