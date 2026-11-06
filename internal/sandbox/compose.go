// Package sandbox reads the shipped Compose file and turns its Worker service
// into the flags of docker run, so the security tests run the Worker exactly
// as shipped. It also checks the file against the Worker baseline, so
// weakening the file breaks the build.
//
// Only the keys the Worker service is known to use are understood. Any other
// key is an error: a new key must be translated, and asserted on, before the
// shipped file may use it.
package sandbox

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// WorkerService is the Compose service that holds the Worker baseline.
const WorkerService = "worker"

// Volume is a long-form Compose volume.
type Volume struct {
	Type     string `yaml:"type"`
	Source   string `yaml:"source"`
	Target   string `yaml:"target"`
	ReadOnly bool   `yaml:"read_only"`
}

// Service is the part of a Compose service that the sandbox understands.
type Service struct {
	Image       string            `yaml:"image"`
	Command     []string          `yaml:"command"`
	User        string            `yaml:"user"`
	SecurityOpt []string          `yaml:"security_opt"`
	CapDrop     []string          `yaml:"cap_drop"`
	ReadOnly    bool              `yaml:"read_only"`
	Tmpfs       []string          `yaml:"tmpfs"`
	Environment map[string]string `yaml:"environment"`
	Volumes     []Volume          `yaml:"volumes"`
	Networks    []string          `yaml:"networks"`
	Ports       []any             `yaml:"ports"`
	MemLimit    string            `yaml:"mem_limit"`
	CPUs        float64           `yaml:"cpus"`
	PidsLimit   int               `yaml:"pids_limit"`
	Restart     string            `yaml:"restart"`

	// UnsupportedKeys are keys of the service that the sandbox does not
	// translate, sorted.
	UnsupportedKeys []string `yaml:"-"`
}

// Network is a top-level Compose network.
type Network struct {
	Internal bool `yaml:"internal"`
}

// Compose is a parsed Compose file.
type Compose struct {
	Services map[string]Service `yaml:"services"`
	Networks map[string]Network `yaml:"networks"`
	// Dir is the directory of the file; relative bind sources are relative
	// to it.
	Dir string `yaml:"-"`
}

// understoodKeys are the service keys that are translated. build and
// healthcheck are accepted but have no effect on the container sandbox.
var understoodKeys = map[string]bool{
	"image": true, "build": true, "command": true, "user": true, "security_opt": true,
	"cap_drop": true, "read_only": true, "tmpfs": true, "environment": true, "volumes": true,
	"networks": true, "ports": true, "mem_limit": true, "cpus": true, "pids_limit": true,
	"restart": true, "healthcheck": true,
}

// LoadCompose reads and parses a Compose file.
func LoadCompose(path string) (Compose, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Compose{}, fmt.Errorf("read compose file: %w", err)
	}
	c, err := ParseCompose(data)
	if err != nil {
		return Compose{}, err
	}
	abs, err := filepath.Abs(filepath.Dir(path))
	if err != nil {
		return Compose{}, fmt.Errorf("resolve compose directory: %w", err)
	}
	c.Dir = abs
	return c, nil
}

// ParseCompose parses the text of a Compose file. YAML anchors and merge
// keys are resolved by the YAML library.
func ParseCompose(data []byte) (Compose, error) {
	var c Compose
	if err := yaml.Unmarshal(data, &c); err != nil {
		return Compose{}, fmt.Errorf("parse compose file: %w", err)
	}
	var raw struct {
		Services map[string]map[string]any `yaml:"services"`
	}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return Compose{}, fmt.Errorf("parse compose file: %w", err)
	}
	for name, keys := range raw.Services {
		svc := c.Services[name]
		for key := range keys {
			if !understoodKeys[key] {
				svc.UnsupportedKeys = append(svc.UnsupportedKeys, key)
			}
		}
		slices.Sort(svc.UnsupportedKeys)
		c.Services[name] = svc
	}
	return c, nil
}

// Worker returns the Worker service.
func (c Compose) Worker() (Service, error) {
	svc, ok := c.Services[WorkerService]
	if !ok {
		return Service{}, fmt.Errorf("compose file has no %q service", WorkerService)
	}
	return svc, nil
}

var interpolationRE = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)(?:(:?-|:?\?)([^}]*))?\}`)

// Interpolate expands ${NAME}, ${NAME:-default}, ${NAME-default} and
// ${NAME:?message} using lookup. A required variable that is unset expands to
// empty.
func Interpolate(s string, lookup func(string) (string, bool)) string {
	return interpolationRE.ReplaceAllStringFunc(s, func(m string) string {
		parts := interpolationRE.FindStringSubmatch(m)
		if v, ok := lookup(parts[1]); ok && v != "" {
			return v
		}
		if strings.HasSuffix(parts[2], "?") {
			// ${NAME:?message} is the required form; the harness runs
			// without the variable and treats it as empty.
			return ""
		}
		return parts[3]
	})
}

// ParseSize reads a Compose byte size such as 64m, 1g or 1024.
func ParseSize(s string) (int64, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	multiplier := int64(1)
	for suffix, m := range map[string]int64{"k": 1 << 10, "m": 1 << 20, "g": 1 << 30} {
		if strings.HasSuffix(s, suffix) {
			s, multiplier = strings.TrimSuffix(s, suffix), m
		}
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("%q is not a byte size", s)
	}
	return n * multiplier, nil
}
