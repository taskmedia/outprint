package inject

import (
	"encoding/json"
	"fmt"
	"html"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Environment variables of the button options. They are global to the
// installation: where the action sits is the operator's decision.
const (
	envButtonPlacement     = "OUTPRINT_BUTTON_PLACEMENT"
	envButtonStyle         = "OUTPRINT_BUTTON_STYLE"
	envButtonFallback      = "OUTPRINT_BUTTON_FALLBACK"
	envButtonFallbackDelay = "OUTPRINT_BUTTON_FALLBACK_DELAY"
	envButtonLabels        = "OUTPRINT_BUTTON_LABELS"
	envButtonMetadataBlock = "OUTPRINT_BUTTON_METADATA_BLOCK"
)

const (
	placementMenu   = "menu"
	placementButton = "button"
	styleIconLabel  = "icon-label"
	styleIcon       = "icon"
	fallbackOn      = "on"
	fallbackOff     = "off"
	metadataHide    = "hide"
	metadataShow    = "show"

	maxLabelRunes = 80
)

var (
	labelKeys     = map[string]bool{"menu": true, "button": true, "tooltip": true}
	languageTagRE = regexp.MustCompile(`^[a-z]{2,3}$`)
)

// ButtonConfig is how the script places the PDF action. The zero value means
// the defaults: placement menu, style icon-label, fallback on, the script's
// own wait, the built-in texts, the Metadata Block hidden in the reading view.
type ButtonConfig struct {
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
	// MetadataBlock is "hide" or "show": whether the script hides the
	// Metadata Block of a Document in Outline's reading view.
	MetadataBlock string
}

func (c *ButtonConfig) readEnv(getenv func(string) string) error {
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

// attributes checks the values and returns the data attributes the script
// reads, in a fixed order. Defaults produce no attribute.
func (c ButtonConfig) attributes() (string, error) {
	var attrs strings.Builder
	add := func(name, value string) {
		fmt.Fprintf(&attrs, ` data-%s="%s"`, name, html.EscapeString(value))
	}
	if err := oneOf(envButtonPlacement, c.Placement, placementMenu, placementButton); err != nil {
		return "", err
	}
	if err := oneOf(envButtonStyle, c.Style, styleIconLabel, styleIcon); err != nil {
		return "", err
	}
	if err := oneOf(envButtonFallback, c.Fallback, fallbackOn, fallbackOff); err != nil {
		return "", err
	}
	if err := oneOf(envButtonMetadataBlock, c.MetadataBlock, metadataHide, metadataShow); err != nil {
		return "", err
	}
	if c.FallbackDelay < 0 {
		return "", fmt.Errorf("%s must not be negative", envButtonFallbackDelay)
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
	if c.MetadataBlock == metadataShow {
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
	var labels map[string]map[string]string
	if err := json.Unmarshal([]byte(raw), &labels); err != nil {
		return "", fmt.Errorf(`%s must be JSON like {"menu":{"de":"Text"}}: %w`, envButtonLabels, err)
	}
	for key, byLang := range labels {
		if !labelKeys[key] {
			return "", fmt.Errorf("%s: unknown key %q, use menu, button or tooltip", envButtonLabels, key)
		}
		for lang, text := range byLang {
			if !languageTagRE.MatchString(lang) {
				return "", fmt.Errorf("%s: %q is not a language such as en or de", envButtonLabels, lang)
			}
			if n := len([]rune(text)); strings.TrimSpace(text) == "" || n > maxLabelRunes {
				return "", fmt.Errorf("%s: the text for %s/%s must have 1 to %d characters", envButtonLabels, key, lang, maxLabelRunes)
			}
		}
	}
	out, err := json.Marshal(labels)
	if err != nil {
		return "", fmt.Errorf("%s: %w", envButtonLabels, err)
	}
	return string(out), nil
}
