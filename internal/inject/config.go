package inject

import (
	"fmt"
	"net"
	"net/url"
	"path"
	"strconv"
	"strings"
)

const (
	// DefaultListen is where the injector listens unless told otherwise.
	DefaultListen = ":8090"
	// DefaultPrefix is the path below which requests go to the Gateway.
	DefaultPrefix = "/pdf"
	// HealthPath answers 200 without touching Outline or the Gateway.
	HealthPath = "/_injector/healthz"
	// DefaultMaxHTMLBytes bounds how much of one page is held in memory.
	DefaultMaxHTMLBytes = 8 << 20

	scriptName = "/button.js"
)

// Environment variables of the injector binary.
const (
	envUpstream     = "OUTPRINT_INJECTOR_UPSTREAM"
	envListen       = "OUTPRINT_INJECTOR_LISTEN"
	envScriptURL    = "OUTPRINT_INJECTOR_SCRIPT_URL"
	envGatewayURL   = "OUTPRINT_INJECTOR_GATEWAY_URL"
	envGatewayPref  = "OUTPRINT_INJECTOR_GATEWAY_PREFIX"
	envCSPAllowSelf = "OUTPRINT_INJECTOR_CSP_ALLOW_SELF"
	envLogLevel     = "OUTPRINT_INJECTOR_LOG_LEVEL"
)

// Config wires the injector.
type Config struct {
	// Upstream is the Outline base URL, the only place a request is sent to
	// besides Gateway.
	Upstream string
	// Gateway is the optional base URL of the Gateway. When set, requests
	// below Prefix go there with the prefix removed.
	Gateway string
	// Prefix is the Gateway path prefix, default "/pdf".
	Prefix string
	// ScriptURL is the src of the injected script, default Prefix +
	// "/button.js". Set it when something else routes the Gateway.
	ScriptURL string
	// AllowScriptSelf adds 'self' to the script-src of Outline's
	// Content-Security-Policy so the script can load from this origin.
	AllowScriptSelf bool
	// MaxHTMLBytes bounds the HTML held in memory per page; a larger page is
	// passed through without the script. Default 8 MiB.
	MaxHTMLBytes int64
	// Button places the PDF action; the zero value is the default.
	Button ButtonConfig

	// Listen and Listener are used by Run only. Listener wins.
	Listen   string
	Listener net.Listener
	LogLevel string
}

// ConfigFromEnv reads the OUTPRINT_INJECTOR_* variables. It does not validate the
// URLs; New does.
func ConfigFromEnv(getenv func(string) string) (Config, error) {
	cfg := Config{
		Upstream:  strings.TrimSpace(getenv(envUpstream)),
		Listen:    firstNonEmpty(getenv(envListen), DefaultListen),
		Gateway:   strings.TrimSpace(getenv(envGatewayURL)),
		Prefix:    firstNonEmpty(getenv(envGatewayPref), DefaultPrefix),
		ScriptURL: strings.TrimSpace(getenv(envScriptURL)),
		LogLevel:  getenv(envLogLevel),
	}
	if cfg.Upstream == "" {
		return Config{}, fmt.Errorf("%s is required: the base URL of Outline", envUpstream)
	}
	if err := cfg.Button.ReadEnv(getenv); err != nil {
		return Config{}, err
	}
	if v := getenv(envCSPAllowSelf); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return Config{}, fmt.Errorf("%s must be true or false, got %q", envCSPAllowSelf, v)
		}
		cfg.AllowScriptSelf = b
	}
	return cfg, nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// settings is a validated Config.
type settings struct {
	upstream, gateway *url.URL
	prefix            string
	scriptURL         string
	allowScriptSelf   bool
	maxHTML           int64
	// buttonAttrs are the data attributes for the script tag, with a leading space.
	buttonAttrs string
}

func (c Config) validate() (settings, error) {
	s := settings{allowScriptSelf: c.AllowScriptSelf, maxHTML: c.MaxHTMLBytes}
	if s.maxHTML <= 0 {
		s.maxHTML = DefaultMaxHTMLBytes
	}
	var err error
	if s.upstream, err = parseBaseURL(envUpstream, c.Upstream); err != nil {
		return settings{}, err
	}
	if c.Gateway != "" {
		if s.gateway, err = parseBaseURL(envGatewayURL, c.Gateway); err != nil {
			return settings{}, err
		}
	}
	s.prefix = firstNonEmpty(c.Prefix, DefaultPrefix)
	if err := checkPrefix(s.prefix); err != nil {
		return settings{}, err
	}
	s.scriptURL = firstNonEmpty(c.ScriptURL, s.prefix+scriptName)
	if s.buttonAttrs, err = c.Button.Attributes(); err != nil {
		return settings{}, err
	}
	return s, nil
}

func parseBaseURL(name, raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	switch {
	case err != nil:
		return nil, fmt.Errorf("%s: %w", name, err)
	case u.Scheme != "http" && u.Scheme != "https":
		return nil, fmt.Errorf("%s must be an http or https URL, got %q", name, raw)
	case u.Host == "":
		return nil, fmt.Errorf("%s has no host: %q", name, raw)
	case u.User != nil:
		return nil, fmt.Errorf("%s must not carry credentials", name)
	}
	return u, nil
}

func checkPrefix(p string) error {
	switch {
	case !strings.HasPrefix(p, "/") || strings.HasSuffix(p, "/"):
		return fmt.Errorf("%s must start with a slash and not end with one, got %q", envGatewayPref, p)
	case path.Clean(p) != p:
		return fmt.Errorf("%s must be a clean path, got %q", envGatewayPref, p)
	case p == path.Dir(HealthPath) || strings.HasPrefix(HealthPath, p+"/"):
		return fmt.Errorf("%s %q would shadow %s", envGatewayPref, p, HealthPath)
	}
	return nil
}
