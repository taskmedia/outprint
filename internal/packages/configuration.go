package packages

import (
	"maps"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// defaultDocumentTypeKey is the one Defaults key that is not a Template
// field: it names the Document Type a Configuration selects by default.
const defaultDocumentTypeKey = "document_type"

// Configuration adapts a Template. Defaults may be overridden by a
// Document, Fixed values may not.
type Configuration struct {
	Name     string
	Dir      string
	Defaults map[string]any
	Fixed    map[string]any
}

// AssetPath returns the absolute path of an asset inside the Configuration.
func (c Configuration) AssetPath(rel string) (string, error) {
	return confine(c.Dir, rel)
}

func (c Configuration) clone() Configuration {
	out := c
	out.Defaults = maps.Clone(c.Defaults)
	out.Fixed = maps.Clone(c.Fixed)
	return out
}

// loadConfiguration loads and checks one Configuration package on its own.
// Checks that need the Template (types, patterns) happen per Binding.
func loadConfiguration(root, name string) (*Configuration, []Finding) {
	c := newCollector("configuration", name)
	dir := filepath.Join(root, name)
	data, err := os.ReadFile(filepath.Join(dir, configurationFile))
	if err != nil {
		c.errorf(configurationFile, "read: %v", err)
		return nil, c.findings
	}
	cfg := Configuration{Name: name, Dir: dir}
	if mroot, ok := parseManifestRoot(data, configurationFile, c); ok {
		cfg.Defaults, cfg.Fixed = parseConfigurationBody(mroot, c)
		checkTree(dir, c)
		checkStandardAssets(cfg, c)
		warnShadowedDefaults(cfg, c)
	}
	if c.hasErrors() {
		return nil, c.findings
	}
	out := cfg.clone()
	return &out, c.findings
}

func parseConfigurationBody(root *yaml.Node, c *collector) (defaults, fixed map[string]any) {
	if ext := valueOf(root, "extends"); ext != nil {
		c.errorf(at(configurationFile, ext.Line), "extends is not supported in v1")
	}
	checkKeys(root, []string{"manifest_version", "extends", "defaults", "fields"}, "", configurationFile, c)
	return readValues(root, "defaults", c), readValues(root, "fields", c)
}

func readValues(root *yaml.Node, key string, c *collector) map[string]any {
	mapping := mappingOf(root, key, configurationFile, c)
	if mapping == nil {
		return nil
	}
	checkDuplicates(mapping, key+".", configurationFile, c)
	values := map[string]any{}
	for _, p := range pairs(mapping) {
		name := p.key.Value
		if name != defaultDocumentTypeKey && !fieldNameRE.MatchString(name) {
			c.errorf(at(configurationFile, p.key.Line), "%s.%s: invalid field name (must match %s)", key, name, fieldNameRE)
			continue
		}
		var v any
		if err := p.value.Decode(&v); err != nil || !simpleValue(v) {
			c.errorf(at(configurationFile, p.value.Line), "%s.%s: value must be a string, bool, date or a list of strings", key, name)
			continue
		}
		values[name] = v
	}
	if key == "defaults" {
		if dt, ok := values[defaultDocumentTypeKey]; ok {
			if s, isString := dt.(string); !isString || !documentTypeNameRE.MatchString(s) {
				c.errorf(at(configurationFile, mapping.Line), "defaults.%s must be a document type name", defaultDocumentTypeKey)
			}
		}
	}
	return values
}

func simpleValue(v any) bool {
	switch x := v.(type) {
	case string, bool, time.Time:
		return true
	case []any:
		for _, item := range x {
			if _, ok := item.(string); !ok {
				return false
			}
		}
		return true
	}
	return false
}

// checkStandardAssets confines the values of Standard asset fields (the
// logo) when no Template is at hand to say which fields are assets.
func checkStandardAssets(cfg Configuration, c *collector) {
	for _, name := range sortedKeys(cfg.Fixed) {
		if t, ok := standardConfigurationType(name); ok && t == TypeAsset {
			checkAssetValue(cfg, "fields."+name, cfg.Fixed[name], c)
		}
	}
}

func checkAssetValue(cfg Configuration, where string, v any, c *collector) {
	rel, ok := v.(string)
	if !ok {
		c.errorf(configurationFile, "%s: asset must be a path string", where)
		return
	}
	if _, err := cfg.AssetPath(rel); err != nil {
		c.errorf(configurationFile, "%s: %v", where, err)
	}
}

func warnShadowedDefaults(cfg Configuration, c *collector) {
	for _, name := range sortedKeys(cfg.Defaults) {
		if _, fixed := cfg.Fixed[name]; fixed {
			c.warnf(configurationFile, "defaults.%s is shadowed by the Fixed Value fields.%s", name, name)
		}
	}
}

// checkBinding validates a Configuration against the Template it is bound
// to: every value of a field the Template declares must have the right
// type and match its pattern. Defaults may carry fields the Template does
// not declare (they are needed for sharing); Fixed Values may not.
func checkBinding(t Template, cfg Configuration, c *collector) {
	for _, name := range sortedKeys(cfg.Defaults) {
		if name == defaultDocumentTypeKey {
			checkDefaultDocumentType(t, cfg, c)
			continue
		}
		if f, ok := t.Fields[name]; ok {
			checkBoundValue(cfg, f, "defaults."+name, cfg.Defaults[name], c)
		}
	}
	for _, name := range sortedKeys(cfg.Fixed) {
		f, ok := t.Fields[name]
		if !ok {
			c.warnf(configurationFile, "fields.%s is not declared by template %q and has no effect", name, t.Name)
			continue
		}
		checkBoundValue(cfg, f, "fields."+name, cfg.Fixed[name], c)
	}
}

func checkDefaultDocumentType(t Template, cfg Configuration, c *collector) {
	name, _ := cfg.Defaults[defaultDocumentTypeKey].(string)
	if _, ok := t.DocumentTypes[name]; !ok {
		known := sortedKeys(t.DocumentTypes)
		c.errorf(configurationFile, "defaults.%s %q is not a document type of template %q (has: %v)", defaultDocumentTypeKey, name, t.Name, known)
	}
}

func checkBoundValue(cfg Configuration, f Field, where string, v any, c *collector) {
	if err := f.Check(v); err != nil {
		c.errorf(configurationFile, "%s: %v", where, err)
		return
	}
	if f.Type == TypeAsset {
		checkAssetValue(cfg, where, v, c)
	}
}
