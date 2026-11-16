package button

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestOptionsOfTheZeroConfigAreZero(t *testing.T) {
	o, err := Config{}.Options()
	if err != nil {
		t.Fatal(err)
	}
	if !o.IsZero() {
		t.Errorf("Options = %+v, want the zero value", o)
	}
}

func TestOptionsKeepOnlyNonDefaults(t *testing.T) {
	o, err := Config{
		Placement: "menu", Style: "icon", Fallback: "off", FallbackDelay: 1500 * time.Millisecond,
		MetadataBlock: "hide", Labels: `{"menu":{"de":"PDF mit Briefkopf"}}`,
	}.Options()
	if err != nil {
		t.Fatal(err)
	}
	want := Options{
		Style: "icon", Fallback: "off", FallbackDelay: 1500, MetadataBlock: "hide",
		Labels: map[string]map[string]string{"menu": {"de": "PDF mit Briefkopf"}},
	}
	if !reflect.DeepEqual(o, want) {
		t.Errorf("Options = %+v, want %+v", o, want)
	}
}

func TestOptionsRejectBadValues(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		key  string
	}{
		{"placement", Config{Placement: "floating"}, envButtonPlacement},
		{"placement in a list", Config{Placement: "menu,floating"}, envButtonPlacement},
		{"placement twice", Config{Placement: "button,button"}, envButtonPlacement},
		{"empty placement in a list", Config{Placement: "menu,"}, envButtonPlacement},
		{"style", Config{Style: "label"}, envButtonStyle},
		{"fallback", Config{Fallback: "maybe"}, envButtonFallback},
		{"metadata block", Config{MetadataBlock: "fold"}, envButtonMetadataBlock},
		{"delay", Config{FallbackDelay: -time.Second}, envButtonFallbackDelay},
		{"labels", Config{Labels: "menu=PDF"}, envButtonLabels},
		{"labels key", Config{Labels: `{"title":{"en":"x"}}`}, envButtonLabels},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := tc.cfg.Options(); err == nil || !strings.Contains(err.Error(), tc.key) {
				t.Errorf("err = %v, want mention of %s", err, tc.key)
			}
		})
	}
}

func TestOptionsAndAttributesNormalizeSeveralPlacements(t *testing.T) {
	tests := []struct {
		placement string
		want      string
	}{
		{"menu", ""},
		{"button", "button"},
		{"menu,button", "menu,button"},
		{" button , menu ", "menu,button"},
	}
	for _, tc := range tests {
		t.Run(tc.placement, func(t *testing.T) {
			o, err := Config{Placement: tc.placement}.Options()
			if err != nil {
				t.Fatal(err)
			}
			if o.Placement != tc.want {
				t.Errorf("Options().Placement = %q, want %q", o.Placement, tc.want)
			}
			attrs, err := Config{Placement: tc.placement}.Attributes()
			if err != nil {
				t.Fatal(err)
			}
			wantAttrs := ""
			if tc.want != "" {
				wantAttrs = ` data-placement="` + tc.want + `"`
			}
			if attrs != wantAttrs {
				t.Errorf("Attributes() = %q, want %q", attrs, wantAttrs)
			}
		})
	}
}

func TestReadEnvRejectsAnUnparsableDelay(t *testing.T) {
	var c Config
	err := c.ReadEnv(func(k string) string {
		if k == envButtonFallbackDelay {
			return "soon"
		}
		return ""
	})
	if err == nil || !strings.Contains(err.Error(), envButtonFallbackDelay) {
		t.Errorf("err = %v", err)
	}
}
