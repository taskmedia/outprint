package packages

import (
	_ "embed"
	"fmt"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed standard_fields.yaml
var standardFieldsYAML []byte

type standardFields struct {
	Common        map[string][]FieldType            `yaml:"common"`
	Configuration map[string][]FieldType            `yaml:"configuration"`
	DocumentTypes map[string]map[string][]FieldType `yaml:"document_types"`
}

var standard = mustLoadStandardFields()

func mustLoadStandardFields() standardFields {
	var s standardFields
	if err := yaml.Unmarshal(standardFieldsYAML, &s); err != nil {
		panic(fmt.Sprintf("embedded standard_fields.yaml: %v", err))
	}
	return s
}

// standardTypes returns the types a Standard Field may have when declared
// in a Document Type with the given name. Common and Configuration fields
// apply everywhere; the rest only to Document Types of that name.
func standardTypes(documentType, field string) ([]FieldType, bool) {
	if types, ok := standard.Common[field]; ok {
		return types, true
	}
	if types, ok := standard.Configuration[field]; ok {
		return types, true
	}
	types, ok := standard.DocumentTypes[documentType][field]
	return types, ok
}

// standardConfigurationType returns the Standard Field type of a
// Configuration value, for Configurations validated without a Template.
func standardConfigurationType(field string) (FieldType, bool) {
	types, ok := standard.Configuration[field]
	if !ok || len(types) == 0 {
		return "", false
	}
	return types[0], true
}

func standardLint(documentType string, f Field) string {
	allowed, ok := standardTypes(documentType, f.Name)
	if !ok || slices.Contains(allowed, f.Type) {
		return ""
	}
	names := make([]string, len(allowed))
	for i, t := range allowed {
		names[i] = string(t)
	}
	return fmt.Sprintf("field %q is declared as %s but the Standard Field %q is %s", f.Name, f.Type, f.Name, strings.Join(names, " or "))
}
