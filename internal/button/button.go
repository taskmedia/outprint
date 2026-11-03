// Package button holds the options of the PDF action (placement, style,
// labels). The Gateway reads them and answers them with the status; the
// injector can still write them as data attributes of the script tag.
package button

import (
	"encoding/json"
	"fmt"
	"html"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/taskmedia/outprint/internal/settings"
)

// Environment variables of the button options. They are global to the
// installation: where the action sits is the operator's decision.
const (
	envButtonPlacement     = settings.EnvButtonPlacement
	envButtonStyle         = settings.EnvButtonStyle
	envButtonFallback      = settings.EnvButtonFallback
	envButtonFallbackDelay = settings.EnvButtonFallbackDelay
	envButtonLabels        = settings.EnvButtonLabels
	envButtonMetadataBlock = settings.EnvButtonMetadataBlock
)

const (
	placementMenu    = "menu"
	placementButton  = "button"
	styleIconLabel   = "icon-label"
	styleIcon        = "icon"
	fallbackOn       = "on"
	fallbackOff      = "off"
	metadataCollapse = "collapse"
	metadataHide     = "hide"
	metadataShow     = "show"

	maxLabelRunes = 80
)

var (
	labelKeys     = map[string]bool{"menu": true, "button": true, "tooltip": true}
	languageTagRE = regexp.MustCompile(`^[a-z]{2,3}$`)
)

// Config is how the script places the PDF action. The zero value means
// the defaults: placement menu, style icon-label, fallback on, the script's
// own wait, the built-in texts, the Metadata Block hidden in the reading view.
type Config struct {
	// Placement is "menu" (Export submenu of the document menu) or "button"
	// (header button before Share).
	Placement string
	// Style is "icon-label" or "icon" and applies to the header button.
	Style string
	// Fallback is "on" or "off": show the floating button when the anchor
	// for Placement cannot be found.
	Fallback string
	// FallbackDelay is how long the script waits for the anchor before it
	// falls back. Zero means the script's default.
	FallbackDelay time.Duration
	// Labels overrides the texts: a JSON object with the keys "menu",
	// "button" and "tooltip", each a map from language ("en", "de") to text.
	Labels string
	// MetadataBlock is "collapse", "hide" or "show": how the script treats the
	// Metadata Block of a Document. Readers never see it unless "show".
	// Authors see it folded behind a toggle ("collapse"), as is ("hide"), or
	// as is ("show").
	MetadataBlock string
}

// ReadEnv reads the OUTPRINT_BUTTON_* variables. It rejects only a duration
// it cannot parse; Validate checks the rest.
func (c *Config) ReadEnv(getenv func(string) string) error {
	c.Placement = strings.TrimSpace(getenv(envButtonPlacement))
	c.Style = strings.TrimSpace(getenv(envButtonStyle))
	c.Fallback = strings.TrimSpace(getenv(envButtonFallback))
	c.Labels = strings.TrimSpace(getenv(envButtonLabels))
	c.MetadataBlock = strings.TrimSpace(getenv(envButtonMetadataBlock))
	if v := strings.TrimSpace(getenv(envButtonFallbackDelay)); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return fmt.Errorf("%s must be a duration such as 3s, got %q", envButtonFallbackDelay, v)
		}
		c.FallbackDelay = d
	}
	return nil
}

// Options is what the Gateway answers with the status and what the script
// reads: only the values that differ from the defaults, so an installation
// without settings sends nothing.
type Options struct {
	Placement     string                       `json:"placement,omitempty"`
	Style         string                       `json:"style,omitempty"`
	Fallback      string                       `json:"fallback,omitempty"`
	FallbackDelay int64                        `json:"fallbackDelayMs,omitempty"`
	MetadataBlock string                       `json:"metadataBlock,omitempty"`
	Labels        map[string]map[string]string `json:"labels,omitempty"`
}

// IsZero reports whether every option has its default.
func (o Options) IsZero() bool {
	return o.Placement == "" && o.Style == "" && o.Fallback == "" && o.FallbackDelay == 0 &&
		o.MetadataBlock == "" && len(o.Labels) == 0
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
	if c.Style == styleIcon {
		o.Style = c.Style
	}
	if c.Fallback == fallbackOff {
		o.Fallback = c.Fallback
	}
	if c.FallbackDelay > 0 {
		o.FallbackDelay = c.FallbackDelay.Milliseconds()
	}
	if c.MetadataBlock == metadataHide || c.MetadataBlock == metadataShow {
		o.MetadataBlock = c.MetadataBlock
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
	if err := oneOf(envButtonStyle, c.Style, styleIconLabel, styleIcon); err != nil {
		return err
	}
	if err := oneOf(envButtonFallback, c.Fallback, fallbackOn, fallbackOff); err != nil {
		return err
	}
	if err := oneOf(envButtonMetadataBlock, c.MetadataBlock, metadataCollapse, metadataHide, metadataShow); err != nil {
		return err
	}
	if c.FallbackDelay < 0 {
		return fmt.Errorf("%s must not be negative", envButtonFallbackDelay)
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
	if c.Style == styleIcon {
		add("style", c.Style)
	}
	if c.Fallback == fallbackOff {
		add("fallback", c.Fallback)
	}
	if c.FallbackDelay > 0 {
		add("fallback-delay", strconv.FormatInt(c.FallbackDelay.Milliseconds(), 10))
	}
	if c.MetadataBlock == metadataHide || c.MetadataBlock == metadataShow {
		add("metadata-block", c.MetadataBlock)
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
