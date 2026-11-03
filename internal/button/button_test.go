package button

import (
	"reflect"
	"strings"
	"testing"
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
		Placement: "menu", Labels: `{"menu":{"de":"PDF mit Briefkopf"}}`,
	}.Options()
	if err != nil {
		t.Fatal(err)
	}
	want := Options{
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
