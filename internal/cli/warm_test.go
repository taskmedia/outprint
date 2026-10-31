package cli

import "testing"

func TestParseWarmNeedsTemplateConfigAndOut(t *testing.T) {
	o, err := parseWarm([]string{"--template", "neutral", "--config", "example", "--out", "/cache"}, noEnv)
	if err != nil {
		t.Fatal(err)
	}
	if o.Template != "neutral" || o.Config != "example" || o.Out != "/cache" || o.TemplatesRoot != "./templates" {
		t.Errorf("got %+v", o)
	}
	for name, args := range map[string][]string{
		"no out":      {"--template", "a", "--config", "b"},
		"no template": {"--config", "b", "--out", "/c"},
		"no config":   {"--template", "a", "--out", "/c"},
		"extra arg":   {"--template", "a", "--config", "b", "--out", "/c", "x"},
	} {
		if _, err := parseWarm(args, noEnv); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}
