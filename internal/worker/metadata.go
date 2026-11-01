package worker

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"sort"
	"strings"
	"time"

	"github.com/taskmedia/outprint/internal/packages"
)

const (
	languageField    = "language"
	fallbackLanguage = "en"
	isoDateLayout    = "2006-01-02"
)

var monthNames = map[string][12]string{
	"en": {"January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"},
	"de": {"Januar", "Februar", "März", "April", "Mai", "Juni", "Juli", "August", "September", "Oktober", "November", "Dezember"},
}

var dateLayouts = map[string]string{"en": "%d %s %d", "de": "%d. %s %d"}

// formatDates returns a copy of values in which ISO dates in date fields are
// written out in the document's language. Any other text in a date field is
// left as the author wrote it.
func formatDates(t packages.Template, values map[string]any) (map[string]any, error) {
	out := maps.Clone(values)
	language := fallbackLanguage
	if l, ok := values[languageField].(string); ok && l != "" {
		language = l
	}
	for name, v := range values {
		if f, ok := t.Fields[name]; !ok || f.Type != packages.TypeDate {
			continue
		}
		s, _ := v.(string)
		day, err := time.Parse(isoDateLayout, s)
		if err != nil {
			continue
		}
		formatted, err := formatDate(day, language)
		if err != nil {
			return nil, fmt.Errorf("field %q: %w", name, err)
		}
		out[name] = formatted
	}
	return out, nil
}

func formatDate(day time.Time, language string) (string, error) {
	months, ok := monthNames[language]
	if !ok {
		return "", fmt.Errorf("cannot write a date in language %q (supported: %s)", language, strings.Join(supportedLanguages(), ", "))
	}
	return fmt.Sprintf(dateLayouts[language], day.Day(), months[day.Month()-1], day.Year()), nil
}

func supportedLanguages() []string {
	names := make([]string, 0, len(monthNames))
	for l := range monthNames {
		names = append(names, l)
	}
	sort.Strings(names)
	return names
}

// pandocMetadata encodes field values as the YAML Pandoc reads with
// --metadata-file. This is the only way Document values reach the Template.
//
// Pandoc parses YAML strings as Markdown. Text fields are meant to be
// Markdown and pass through. Everything else is plain text, so each ASCII
// punctuation character is backslash-escaped, which Pandoc Markdown reads
// back as the literal character.
func pandocMetadata(t packages.Template, values map[string]any) ([]byte, error) {
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)

	var out bytes.Buffer
	for _, name := range names {
		field, ok := t.Fields[name]
		if !ok {
			return nil, fmt.Errorf("value for %q, which the Template does not declare", name)
		}
		encoded, err := encodeValue(field, values[name])
		if err != nil {
			return nil, fmt.Errorf("field %q: %w", name, err)
		}
		key, _ := jsonString(name)
		fmt.Fprintf(&out, "%s: %s\n", key, encoded)
	}
	return out.Bytes(), nil
}

func encodeValue(f packages.Field, v any) (string, error) {
	switch x := v.(type) {
	case bool:
		return fmt.Sprintf("%t", x), nil
	case string:
		return jsonString(plainOrMarkdown(f, x))
	case []string:
		items := make([]string, len(x))
		for i, item := range x {
			s, err := jsonString(escapeMarkdown(strings.TrimSpace(item)))
			if err != nil {
				return "", err
			}
			items[i] = s
		}
		return "[" + strings.Join(items, ", ") + "]", nil
	}
	return "", fmt.Errorf("unsupported value of type %T", v)
}

func plainOrMarkdown(f packages.Field, s string) string {
	if f.Type == packages.TypeText {
		return s
	}
	return escapeMarkdown(strings.TrimSpace(s))
}

// jsonString writes a YAML double-quoted scalar. JSON strings are valid
// YAML, and quoting means no value can be read as anything but a string.
func jsonString(s string) (string, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(s); err != nil {
		return "", err
	}
	return strings.TrimSuffix(b.String(), "\n"), nil
}

func escapeMarkdown(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r < 0x80 && isASCIIPunctuation(byte(r)) {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

func isASCIIPunctuation(c byte) bool {
	return c >= '!' && c <= '/' || c >= ':' && c <= '@' || c >= '[' && c <= '`' || c >= '{' && c <= '~'
}
