// Package button holds the options of the PDF action (placement, style,
// labels). The Gateway reads them and answers them with the status; the
// injector can still write them as data attributes of the script tag.
package button

import (
	"encoding/json"
	"fmt"
	"html"
	"regexp"
	"strings"

	"github.com/taskmedia/outprint/internal/settings"
)

// Environment variables of the button options. They are global to the
// installation: where the action sits is the operator's decision.
const (
	envButtonPlacement = settings.EnvButtonPlacement
	envButtonLabels    = settings.EnvButtonLabels
)

const (
	placementMenu   = "menu"
	placementButton = "button"

	maxLabelRunes = 80
)

var (
	labelKeys     = map[string]bool{"menu": true, "button": true, "tooltip": true}
	languageTagRE = regexp.MustCompile(`^[a-z]{2,3}$`)
)

// Config is how the script places the PDF action. The zero value means
// the defaults: placement menu, the built-in texts.
type Config struct {
	// Placement is "menu" (Export submenu of the document menu) or "button"
	// (header button before Share).
	Placement string
	// Labels overrides the texts: a JSON object with the keys "menu",
	// "button" and "tooltip", each a map from language ("en", "de") to text.
	Labels string
}

// ReadEnv reads the OUTPRINT_BUTTON_* variables. Validate checks the values.
func (c *Config) ReadEnv(getenv func(string) string) {
	c.Placement = strings.TrimSpace(getenv(envButtonPlacement))
	c.Labels = strings.TrimSpace(getenv(envButtonLabels))
}

// Options is what the Gateway answers with the status and what the script
// reads: only the values that differ from the defaults, so an installation
// without settings sends nothing.
type Options struct {
	Placement string                       `json:"placement,omitempty"`
	Labels    map[string]map[string]string `json:"labels,omitempty"`
}

// IsZero reports whether every option has its default.
func (o Options) IsZero() bool {
	return o.Placement == "" && len(o.Labels) == 0
}

// Validate checks the values.
func (c Config) Validate() error {
	_, err := c.Options()
	return err
}

// Options checks the values and returns them for the status answer.
func (c Config) Options() (Options, error) {
	if err := c.check(); err != nil {
		return Options{}, err
	}
	var o Options
	if c.Placement == placementButton {
		o.Placement = c.Placement
	}
	if c.Labels != "" {
		labels, err := parseLabels(c.Labels)
		if err != nil {
			return Options{}, err
		}
		o.Labels = labels
	}
	return o, nil
}

func (c Config) check() error {
	if err := oneOf(envButtonPlacement, c.Placement, placementMenu, placementButton); err != nil {
		return err
	}
	return nil
}

// Attributes checks the values and returns the data attributes the script
// reads, in a fixed order. Defaults produce no attribute.
func (c Config) Attributes() (string, error) {
	var attrs strings.Builder
	add := func(name, value string) {
		fmt.Fprintf(&attrs, ` data-%s="%s"`, name, html.EscapeString(value))
	}
	if err := c.check(); err != nil {
		return "", err
	}
	if c.Placement == placementButton {
		add("placement", c.Placement)
	}
	if c.Labels != "" {
		labels, err := normalizeLabels(c.Labels)
		if err != nil {
			return "", err
		}
		add("labels", labels)
	}
	return attrs.String(), nil
}

func oneOf(name, value string, allowed ...string) error {
	if value == "" {
		return nil
	}
	for _, a := range allowed {
		if value == a {
			return nil
		}
	}
	return fmt.Errorf("%s must be %s, got %q", name, strings.Join(allowed, " or "), value)
}

// normalizeLabels checks the override texts and returns them as compact JSON
// with sorted keys.
func normalizeLabels(raw string) (string, error) {
	labels, err := parseLabels(raw)
	if err != nil {
		return "", err
	}
	out, err := json.Marshal(labels)
	if err != nil {
		return "", fmt.Errorf("%s: %w", envButtonLabels, err)
	}
	return string(out), nil
}

func parseLabels(raw string) (map[string]map[string]string, error) {
	var labels map[string]map[string]string
	if err := json.Unmarshal([]byte(raw), &labels); err != nil {
		return nil, fmt.Errorf(`%s must be JSON like {"menu":{"de":"Text"}}: %w`, envButtonLabels, err)
	}
	for key, byLang := range labels {
		if !labelKeys[key] {
			return nil, fmt.Errorf("%s: unknown key %q, use menu, button or tooltip", envButtonLabels, key)
		}
		for lang, text := range byLang {
			if !languageTagRE.MatchString(lang) {
				return nil, fmt.Errorf("%s: %q is not a language such as en or de", envButtonLabels, lang)
			}
			if n := len([]rune(text)); strings.TrimSpace(text) == "" || n > maxLabelRunes {
				return nil, fmt.Errorf("%s: the text for %s/%s must have 1 to %d characters", envButtonLabels, key, lang, maxLabelRunes)
			}
		}
	}
	return labels, nil
}
