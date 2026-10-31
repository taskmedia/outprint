package bundle

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func valid() Bundle {
	return Bundle{
		Markdown:      []byte("Hello **world**\n"),
		Values:        map[string]any{"title": "Greeting", "recipient": []string{"A", "B"}, "toc": true},
		Template:      "neutral",
		DocumentType:  "letter",
		Configuration: "example",
		Images:        map[string][]byte{"fig-1.png": {1, 2, 3}},
	}
}

func TestRoundTripKeepsTypes(t *testing.T) {
	data, err := valid().Encode()
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if !reflect.DeepEqual(got, valid()) {
		t.Errorf("round trip changed the bundle:\n got %#v\nwant %#v", got, valid())
	}
}

func TestEncodedFieldsAreTheContractAndNothingElse(t *testing.T) {
	data, err := valid().Encode()
	if err != nil {
		t.Fatal(err)
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil {
		t.Fatal(err)
	}
	var keys []string
	for k := range top {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	want := []string{"configuration", "documentType", "images", "markdown", "template", "values"}
	if !slices.Equal(keys, want) {
		t.Errorf("bundle fields = %v, want %v: a new field is a contract change", keys, want)
	}
}

func TestValidateRejects(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Bundle)
		limit  bool
	}{
		{"template path", func(b *Bundle) { b.Template = "../etc" }, false},
		{"template upper", func(b *Bundle) { b.Template = "Neutral" }, false},
		{"configuration empty", func(b *Bundle) { b.Configuration = "" }, false},
		{"document type", func(b *Bundle) { b.DocumentType = "a b" }, false},
		{"value name", func(b *Bundle) { b.Values["bad-name"] = "x" }, false},
		{"value type", func(b *Bundle) { b.Values["n"] = 3 }, false},
		{"image name path", func(b *Bundle) { b.Images["../x.png"] = []byte{1} }, false},
		{"image name dotfile", func(b *Bundle) { b.Images[".hidden"] = []byte{1} }, false},
		{"markdown too large", func(b *Bundle) { b.Markdown = make([]byte, MaxMarkdownBytes+1) }, true},
		{"image too large", func(b *Bundle) { b.Images["big.png"] = make([]byte, MaxImageBytes+1) }, true},
		{"too many images", func(b *Bundle) {
			b.Images = map[string][]byte{}
			for i := 0; i <= MaxImages; i++ {
				b.Images[strings.Repeat("a", 1)+string(rune('a'+i%26))+strings.Repeat("b", i/26)+".png"] = []byte{1}
			}
		}, true},
		{"value too large", func(b *Bundle) { b.Values["title"] = strings.Repeat("x", MaxValueBytes+1) }, true},
		{"list too long", func(b *Bundle) { b.Values["recipient"] = make([]string, MaxListItems+1) }, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := valid()
			tt.mutate(&b)
			err := b.Validate()
			if err == nil {
				t.Fatal("Validate accepted the bundle")
			}
			var limit *LimitError
			if errors.As(err, &limit) != tt.limit {
				t.Errorf("limit error = %v, want %v (%v)", !tt.limit, tt.limit, err)
			}
		})
	}
}

func TestImagesTotalLimit(t *testing.T) {
	b := valid()
	b.Images = map[string][]byte{}
	for i := 0; i < 6; i++ {
		b.Images["img"+string(rune('a'+i))+".png"] = make([]byte, MaxImageBytes)
	}
	var limit *LimitError
	if err := b.Validate(); !errors.As(err, &limit) {
		t.Errorf("Validate = %v, want a limit error for the total", err)
	}
}

func TestDecodeRejects(t *testing.T) {
	tests := map[string]string{
		"unknown field":   `{"markdown":"","template":"neutral","documentType":"letter","configuration":"example","cookie":"x"}`,
		"trailing data":   `{"markdown":"","template":"neutral","documentType":"letter","configuration":"example"} {}`,
		"not json":        `nope`,
		"number value":    `{"markdown":"","values":{"n":1},"template":"neutral","documentType":"letter","configuration":"example"}`,
		"list of numbers": `{"markdown":"","values":{"n":[1]},"template":"neutral","documentType":"letter","configuration":"example"}`,
		"bad name":        `{"markdown":"","template":"x/y","documentType":"letter","configuration":"example"}`,
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := Decode(strings.NewReader(body)); err == nil {
				t.Error("Decode accepted the bundle")
			}
		})
	}
}

func TestStringHasNoContent(t *testing.T) {
	b := valid()
	b.Markdown = []byte("secret body")
	b.Values["title"] = "secret title"
	if s := b.String(); strings.Contains(s, "secret") {
		t.Errorf("String() leaks content: %s", s)
	}
}
