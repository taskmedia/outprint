// Package config reads the service configuration: where Templates and
// Template Configurations live and which pairings (Bindings) are allowed.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/taskmedia/outprint/internal/admission"
	"github.com/taskmedia/outprint/internal/limits"
	"github.com/taskmedia/outprint/internal/settings"
)

// Environment variables. The roots override the file so that an image can
// ship one configuration and a deployment relocates the packages.
const (
	EnvTemplatesRoot      = settings.EnvTemplatesRoot
	EnvConfigurationsRoot = settings.EnvConfigsRoot
	EnvServiceConfig      = settings.EnvServiceConfig
)

// imageHostPattern is a lower-case host name or IPv4 address, with an
// optional port. It has no scheme, path, credentials or wildcard.
var imageHostPattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9.-]*[a-z0-9])?(:[0-9]{1,5})?$`)

// NamePattern is what a Template or Configuration directory name must match.
var NamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// Binding pairs one Template with one Template Configuration.
type Binding struct {
	Template      string `yaml:"template"`
	Configuration string `yaml:"configuration"`
}

// Service is the service configuration.
type Service struct {
	TemplatesRoot      string    `yaml:"templatesRoot"`
	ConfigurationsRoot string    `yaml:"configurationsRoot"`
	Bindings           []Binding `yaml:"bindings"`
	DefaultBinding     *Binding  `yaml:"defaultBinding"`
	// ImageHosts are the hosts besides the Outline origin that an image
	// redirect may lead to, for example the file storage of the Outline.
	// An entry is a host name or host:port.
	ImageHosts []string `yaml:"imageHosts"`
	// Admission sizes the Gateway's slot limit, queue and per-user limit.
	// Keys left out keep their defaults; the environment overrides the file.
	Admission admission.Config `yaml:"admission"`
}

// Errors is a list of configuration problems.
type Errors []error

func (e Errors) Error() string {
	parts := make([]string, len(e))
	for i, err := range e {
		parts[i] = err.Error()
	}
	return strings.Join(parts, "; ")
}

// Load reads and validates the service configuration file at path.
func Load(path string, getenv func(string) string) (Service, []error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Service{}, []error{fmt.Errorf("read service configuration: %w", err)}
	}
	return Parse(data, getenv)
}

// Parse decodes strictly (unknown keys are errors), applies the environment
// overrides and validates. It reports every problem it finds.
func Parse(data []byte, getenv func(string) string) (Service, []error) {
	s := Service{Admission: admission.Defaults()}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&s); err != nil {
		if errors.Is(err, io.EOF) {
			return Service{}, []error{errors.New("service configuration is empty")}
		}
		return Service{}, []error{fmt.Errorf("service configuration yaml: %w", err)}
	}
	if v := getenv(EnvTemplatesRoot); v != "" {
		s.TemplatesRoot = v
	}
	if v := getenv(EnvConfigurationsRoot); v != "" {
		s.ConfigurationsRoot = v
	}
	envErrs := applyAdmissionEnv(&s.Admission, getenv)
	return s, append(envErrs, s.validate()...)
}

// applyAdmissionEnv overrides the admission settings from the environment.
// A malformed value is an error, never a silent fallback.
func applyAdmissionEnv(a *admission.Config, getenv func(string) string) []error {
	var errs []error
	for _, o := range []struct {
		env string
		set func(int)
	}{
		{limits.EnvAdmissionSlots, func(n int) { a.Slots = n }},
		{limits.EnvAdmissionQueueDepth, func(n int) { a.QueueDepth = n }},
		{limits.EnvAdmissionPerUser, func(n int) { a.PerUser = n }},
	} {
		raw := getenv(o.env)
		if raw == "" {
			continue
		}
		n, err := strconv.Atoi(raw)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s must be an integer, got %q", o.env, raw))
			continue
		}
		o.set(n)
	}
	if raw := getenv(limits.EnvAdmissionMaxWait); raw != "" {
		d, err := time.ParseDuration(raw)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s must be a duration such as 30s, got %q", limits.EnvAdmissionMaxWait, raw))
		} else {
			a.MaxWait = d
		}
	}
	return errs
}

func (s Service) validate() []error {
	var errs []error
	if s.TemplatesRoot == "" {
		errs = append(errs, fmt.Errorf("templatesRoot is required (file or %s)", EnvTemplatesRoot))
	}
	if s.ConfigurationsRoot == "" {
		errs = append(errs, fmt.Errorf("configurationsRoot is required (file or %s)", EnvConfigurationsRoot))
	}
	if len(s.Bindings) == 0 {
		errs = append(errs, errors.New("bindings: at least one allowed binding is required"))
	}
	seen := map[Binding]bool{}
	for i, b := range s.Bindings {
		errs = append(errs, validateBinding(fmt.Sprintf("bindings[%d]", i), b)...)
		if seen[b] {
			errs = append(errs, fmt.Errorf("bindings[%d]: duplicate binding %s/%s", i, b.Template, b.Configuration))
		}
		seen[b] = true
	}
	if s.DefaultBinding != nil {
		errs = append(errs, validateBinding("defaultBinding", *s.DefaultBinding)...)
		if !seen[*s.DefaultBinding] {
			errs = append(errs, fmt.Errorf("defaultBinding %s/%s is not in bindings", s.DefaultBinding.Template, s.DefaultBinding.Configuration))
		}
	}
	for i, h := range s.ImageHosts {
		if !imageHostPattern.MatchString(h) {
			errs = append(errs, fmt.Errorf("imageHosts[%d]: %q is not a lower-case host name or host:port (no scheme, path or wildcard)", i, h))
		}
	}
	if err := s.Admission.Validate(); err != nil {
		errs = append(errs, fmt.Errorf("admission: %w", err))
	}
	return errs
}

func validateBinding(where string, b Binding) []error {
	var errs []error
	if !NamePattern.MatchString(b.Template) {
		errs = append(errs, fmt.Errorf("%s: invalid template name %q (must match %s)", where, b.Template, NamePattern))
	}
	if !NamePattern.MatchString(b.Configuration) {
		errs = append(errs, fmt.Errorf("%s: invalid configuration name %q (must match %s)", where, b.Configuration, NamePattern))
	}
	return errs
}
