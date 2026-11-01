package inject

import (
	"strings"
	"testing"
	"time"
)

func TestButtonEnvDefaultsToNothing(t *testing.T) {
	cfg, err := ConfigFromEnv(envOf(map[string]string{"OUTPRINT_INJECTOR_UPSTREAM": "http://outline.example.invalid"}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Button != (ButtonConfig{}) {
		t.Errorf("Button = %+v, want the zero value", cfg.Button)
	}
}

func TestButtonEnvIsRead(t *testing.T) {
	cfg, err := ConfigFromEnv(envOf(map[string]string{
		"OUTPRINT_INJECTOR_UPSTREAM":     "http://outline.example.invalid",
		"OUTPRINT_BUTTON_PLACEMENT":      "button",
		"OUTPRINT_BUTTON_STYLE":          "icon",
		"OUTPRINT_BUTTON_FALLBACK":       "off",
		"OUTPRINT_BUTTON_FALLBACK_DELAY": "5s",
		"OUTPRINT_BUTTON_LABELS":         `{"menu":{"de":"PDF mit Briefkopf"}}`,
		"OUTPRINT_BUTTON_METADATA_BLOCK": "show",
	}))
	if err != nil {
		t.Fatal(err)
	}
	want := ButtonConfig{
		Placement: "button", Style: "icon", Fallback: "off", FallbackDelay: 5 * time.Second,
		Labels: `{"menu":{"de":"PDF mit Briefkopf"}}`, MetadataBlock: "show",
	}
	if cfg.Button != want {
		t.Errorf("Button = %+v, want %+v", cfg.Button, want)
	}
}

func TestButtonEnvRejectsBadValues(t *testing.T) {
	tests := []struct{ name, key, value string }{
		{"placement", "OUTPRINT_BUTTON_PLACEMENT", "floating"},
		{"style", "OUTPRINT_BUTTON_STYLE", "label"},
		{"fallback", "OUTPRINT_BUTTON_FALLBACK", "maybe"},
		{"metadata block", "OUTPRINT_BUTTON_METADATA_BLOCK", "off"},
		{"delay not a duration", "OUTPRINT_BUTTON_FALLBACK_DELAY", "soon"},
		{"delay negative", "OUTPRINT_BUTTON_FALLBACK_DELAY", "-1s"},
		{"labels not json", "OUTPRINT_BUTTON_LABELS", "menu=PDF"},
		{"labels unknown key", "OUTPRINT_BUTTON_LABELS", `{"title":{"en":"x"}}`},
		{"labels bad language", "OUTPRINT_BUTTON_LABELS", `{"menu":{"German!":"x"}}`},
		{"labels empty text", "OUTPRINT_BUTTON_LABELS", `{"menu":{"de":""}}`},
		{"labels too long", "OUTPRINT_BUTTON_LABELS", `{"menu":{"de":"` + strings.Repeat("x", 81) + `"}}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := ConfigFromEnv(envOf(map[string]string{
				"OUTPRINT_INJECTOR_UPSTREAM": "http://outline.example.invalid",
				tc.key:                       tc.value,
			}))
			if err == nil {
				_, err = New(cfg)
			}
			if err == nil || !strings.Contains(err.Error(), tc.key) {
				t.Errorf("err = %v, want mention of %s", err, tc.key)
			}
		})
	}
}

func TestTagCarriesOnlyNonDefaultOptions(t *testing.T) {
	srv := newProxy(t, serveHTML("text/html", "<body></body>"), func(c *Config) {
		c.Button = ButtonConfig{
			Placement: "button", Style: "icon", Fallback: "off", FallbackDelay: 1500 * time.Millisecond,
			Labels: `{"menu":{"de":"A \"B\""},"button":{"en":"Doc PDF"}}`, MetadataBlock: "show",
		}
	})
	_, body := get(t, srv.URL+"/", nil)
	want := `<body><script src="/pdf/button.js" defer data-outline-pdf-injector` +
		` data-placement="button" data-style="icon" data-fallback="off" data-fallback-delay="1500" data-metadata-block="show"` +
		` data-labels="{&#34;button&#34;:{&#34;en&#34;:&#34;Doc PDF&#34;},&#34;menu&#34;:{&#34;de&#34;:&#34;A \&#34;B\&#34;&#34;}}"></script></body>`
	if string(body) != want {
		t.Errorf("body = %s\nwant   %s", body, want)
	}
}

func TestDefaultOptionsLeaveTheTagUnchanged(t *testing.T) {
	srv := newProxy(t, serveHTML("text/html", "<body></body>"), func(c *Config) {
		c.Button = ButtonConfig{Placement: "menu", Style: "icon-label", Fallback: "on", MetadataBlock: "hide"}
	})
	_, body := get(t, srv.URL+"/", nil)
	if want := "<body>" + scriptTag + "</body>"; string(body) != want {
		t.Errorf("body = %s, want %s", body, want)
	}
}
