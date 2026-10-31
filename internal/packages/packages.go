// Package packages loads Templates and Template Configurations from their
// two roots. It does the minimum a render needs: resolve a package by name,
// read its manifest, and hand out paths that are confined to the package.
package packages

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	manifestVersion   = 1
	manifestFile      = "manifest.yaml"
	configurationFile = "configuration.yaml"
	sharedDir         = "shared"
)

var nameRE = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// Roots are the two directories packages are installed in.
type Roots struct {
	Templates      string
	Configurations string
}

// Template is a style family offering one or more Document Types.
type Template struct {
	Name          string
	Dir           string
	DocumentTypes map[string]DocumentType
}

// DocumentType names the Pandoc template file of one kind of output.
type DocumentType struct {
	Template string `yaml:"template"`
}

// Configuration adapts a Template. Defaults may be overridden by a Document,
// Fixed values may not.
type Configuration struct {
	Name     string
	Dir      string
	Defaults map[string]any
	Fixed    map[string]any
}

type templateManifest struct {
	ManifestVersion int                     `yaml:"manifest_version"`
	DocumentTypes   map[string]DocumentType `yaml:"document_types"`
}

type configurationManifest struct {
	ManifestVersion int            `yaml:"manifest_version"`
	Extends         string         `yaml:"extends"`
	Defaults        map[string]any `yaml:"defaults"`
	Fields          map[string]any `yaml:"fields"`
}

// LoadTemplate loads the Template called name from root.
func LoadTemplate(root, name string) (Template, error) {
	var m templateManifest
	dir, err := readPackage(root, name, manifestFile, &m)
	if err != nil {
		return Template{}, fmt.Errorf("template %q: %w", name, err)
	}
	if m.ManifestVersion != manifestVersion {
		return Template{}, fmt.Errorf("template %q: unsupported manifest_version %d", name, m.ManifestVersion)
	}
	return Template{Name: name, Dir: dir, DocumentTypes: m.DocumentTypes}, nil
}

// LoadConfiguration loads the Configuration called name from root.
func LoadConfiguration(root, name string) (Configuration, error) {
	var m configurationManifest
	dir, err := readPackage(root, name, configurationFile, &m)
	if err != nil {
		return Configuration{}, fmt.Errorf("configuration %q: %w", name, err)
	}
	if m.ManifestVersion != manifestVersion {
		return Configuration{}, fmt.Errorf("configuration %q: unsupported manifest_version %d", name, m.ManifestVersion)
	}
	if m.Extends != "" {
		return Configuration{}, fmt.Errorf("configuration %q: extends is not supported", name)
	}
	return Configuration{Name: name, Dir: dir, Defaults: m.Defaults, Fixed: m.Fields}, nil
}

// DocumentTypeFile returns the absolute path of the Pandoc template of a
// Document Type.
func (t Template) DocumentTypeFile(documentType string) (string, error) {
	dt, ok := t.DocumentTypes[documentType]
	if !ok {
		return "", fmt.Errorf("template %q has no document type %q", t.Name, documentType)
	}
	return confine(t.Dir, dt.Template)
}

// SharedFiles lists the file names in the Template's shared directory.
func (t Template) SharedFiles() ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(t.Dir, sharedDir))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("template %q: %w", t.Name, err)
	}
	var names []string
	for _, e := range entries {
		if e.Type().IsRegular() {
			names = append(names, e.Name())
		}
	}
	return names, nil
}

// SharedPath returns the absolute path of a file in the shared directory.
func (t Template) SharedPath(name string) (string, error) {
	return confine(filepath.Join(t.Dir, sharedDir), name)
}

// AssetPath returns the absolute path of an asset inside the Configuration.
func (c Configuration) AssetPath(rel string) (string, error) {
	return confine(c.Dir, rel)
}

func readPackage(root, name, file string, out any) (string, error) {
	if !nameRE.MatchString(name) {
		return "", fmt.Errorf("invalid name: must match %s", nameRE)
	}
	dir := filepath.Join(root, name)
	data, err := os.ReadFile(filepath.Join(dir, file))
	if err != nil {
		return "", fmt.Errorf("read %s: %w", file, err)
	}
	if err := yaml.Unmarshal(data, out); err != nil {
		return "", fmt.Errorf("parse %s: %w", file, err)
	}
	return dir, nil
}

// confine resolves rel inside base and rejects anything that is absolute,
// escapes base, or reaches outside it through a symlink.
func confine(base, rel string) (string, error) {
	if rel == "" || filepath.IsAbs(rel) {
		return "", fmt.Errorf("path %q must be relative and non-empty", rel)
	}
	realBase, err := filepath.EvalSymlinks(base)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", base, err)
	}
	joined := filepath.Join(realBase, rel)
	if !within(realBase, joined) {
		return "", fmt.Errorf("path %q escapes its package", rel)
	}
	real, err := filepath.EvalSymlinks(joined)
	if err != nil {
		return "", fmt.Errorf("resolve %q: %w", rel, err)
	}
	if !within(realBase, real) {
		return "", fmt.Errorf("path %q escapes its package", rel)
	}
	return real, nil
}

func within(base, path string) bool {
	rel, err := filepath.Rel(base, path)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
