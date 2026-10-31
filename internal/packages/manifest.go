package packages

import (
	"slices"

	"gopkg.in/yaml.v3"
)

// supportedManifestVersion is the only schema version this renderer reads.
const supportedManifestVersion = 1

type pair struct{ key, value *yaml.Node }

// parseManifestRoot parses a manifest and checks its manifest_version
// before anything else, so a package written for a newer schema fails with
// that cause instead of a cascade of unknown keys. ok is false if the
// caller should stop.
func parseManifestRoot(data []byte, file string, c *collector) (root *yaml.Node, ok bool) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		c.errorf(file, "invalid yaml: %v", err)
		return nil, false
	}
	if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		c.errorf(file, "manifest must be a mapping")
		return nil, false
	}
	root = doc.Content[0]
	versionNode := valueOf(root, "manifest_version")
	if versionNode == nil {
		c.errorf(at(file, root.Line), "manifest_version is required (supported: %d)", supportedManifestVersion)
		return nil, false
	}
	var version int
	if err := versionNode.Decode(&version); err != nil || version != supportedManifestVersion {
		c.errorf(at(file, versionNode.Line), "unsupported manifest_version %q (supported: %d)", versionNode.Value, supportedManifestVersion)
		return nil, false
	}
	return root, true
}

func valueOf(mapping *yaml.Node, key string) *yaml.Node {
	for _, p := range pairs(mapping) {
		if p.key.Value == key {
			return p.value
		}
	}
	return nil
}

func pairs(mapping *yaml.Node) []pair {
	var out []pair
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		out = append(out, pair{mapping.Content[i], mapping.Content[i+1]})
	}
	return out
}

// checkKeys reports unknown and duplicate keys of a mapping.
func checkKeys(mapping *yaml.Node, allowed []string, where, file string, c *collector) {
	seen := map[string]bool{}
	for _, p := range pairs(mapping) {
		key := p.key.Value
		switch {
		case !slices.Contains(allowed, key):
			c.errorf(at(file, p.key.Line), "%sunknown key %q", where, key)
		case seen[key]:
			c.errorf(at(file, p.key.Line), "%sduplicate key %q", where, key)
		}
		seen[key] = true
	}
}

// mappingOf returns the mapping behind a key, reporting a type error if the
// value is something else. A missing key yields nil without a finding.
func mappingOf(root *yaml.Node, key, file string, c *collector) *yaml.Node {
	node := valueOf(root, key)
	if node == nil {
		return nil
	}
	if node.Kind != yaml.MappingNode {
		c.errorf(at(file, node.Line), "%s must be a mapping", key)
		return nil
	}
	return node
}
