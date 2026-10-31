package packages

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	manifestFile      = "manifest.yaml"
	configurationFile = "configuration.yaml"
	sharedDir         = "shared"
)

var (
	documentTypeNameRE = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)
	// extensions of TeX source the deny-list scans
	texExtensions = []string{".tex", ".sty", ".cls", ".def", ".cfg", ".ltx", ".clo"}
)

// Template is a style family offering one or more Document Types.
type Template struct {
	Name          string
	Dir           string
	Fields        map[string]Field
	DocumentTypes map[string]DocumentType
}

// DocumentType is a kind of output of a Template: a Pandoc template file
// and the fields it uses, with optional overrides of required and default.
type DocumentType struct {
	Template string
	Fields   []DocumentTypeField
}

// DocumentTypeField is a field a Document Type uses. Required and Default
// override the manifest's declaration for this Document Type only.
type DocumentTypeField struct {
	Name       string
	Required   *bool
	Default    any
	HasDefault bool
}

// FieldsOf returns the fields of a Document Type with its overrides
// applied, in the order the manifest lists them.
func (t Template) FieldsOf(documentType string) ([]Field, bool) {
	dt, ok := t.DocumentTypes[documentType]
	if !ok {
		return nil, false
	}
	out := make([]Field, 0, len(dt.Fields))
	for _, ref := range dt.Fields {
		f := t.Fields[ref.Name]
		if ref.Required != nil {
			f.Required = *ref.Required
		}
		if ref.HasDefault {
			f.Default, f.HasDefault = ref.Default, true
		}
		out = append(out, f)
	}
	return out, true
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
	if os.IsNotExist(err) {
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

func (t Template) clone() Template {
	out := t
	out.Fields = make(map[string]Field, len(t.Fields))
	for k, v := range t.Fields {
		out.Fields[k] = v
	}
	out.DocumentTypes = make(map[string]DocumentType, len(t.DocumentTypes))
	for k, v := range t.DocumentTypes {
		v.Fields = slices.Clone(v.Fields)
		out.DocumentTypes[k] = v
	}
	return out
}

// templateLoad is the working state while one Template is loaded.
type templateLoad struct {
	t          Template
	c          *collector
	fieldLines map[string]int
	used       map[string]bool
	scanned    map[string]bool
}

// loadTemplate loads and fully checks one Template package. The Template
// is non-nil only if no error was found; findings are always returned.
func loadTemplate(root, name string) (*Template, []Finding) {
	c := newCollector("template", name)
	dir := filepath.Join(root, name)
	data, err := os.ReadFile(filepath.Join(dir, manifestFile))
	if err != nil {
		c.errorf(manifestFile, "read: %v", err)
		return nil, c.findings
	}
	l := &templateLoad{
		t:          Template{Name: name, Dir: dir, Fields: map[string]Field{}, DocumentTypes: map[string]DocumentType{}},
		c:          c,
		fieldLines: map[string]int{},
		used:       map[string]bool{},
		scanned:    map[string]bool{},
	}
	if mroot, ok := parseManifestRoot(data, manifestFile, c); ok {
		l.parseManifest(mroot)
		l.checkDocumentTypes()
		l.warnUnusedFields()
		l.lintStandardFields()
		l.scanRemainingFiles(checkTree(dir, c))
	}
	if c.hasErrors() {
		return nil, c.findings
	}
	tpl := l.t.clone()
	return &tpl, c.findings
}

func (l *templateLoad) parseManifest(root *yaml.Node) {
	checkKeys(root, []string{"manifest_version", "fields", "document_types"}, "", manifestFile, l.c)
	if fields := mappingOf(root, "fields", manifestFile, l.c); fields != nil {
		l.parseFields(fields)
	}
	types := mappingOf(root, "document_types", manifestFile, l.c)
	if types == nil {
		l.c.errorf(manifestFile, "document_types is required: a Template offers at least one Document Type")
		return
	}
	l.parseDocumentTypes(types)
	if len(l.t.DocumentTypes) == 0 {
		l.c.errorf(at(manifestFile, types.Line), "document_types is empty: a Template offers at least one Document Type")
	}
}

func (l *templateLoad) parseFields(mapping *yaml.Node) {
	checkDuplicates(mapping, "fields.", manifestFile, l.c)
	for _, p := range pairs(mapping) {
		f, problems := parseFieldNode(p.key.Value, p.value)
		for _, msg := range problems {
			l.c.errorf(at(manifestFile, p.key.Line), "fields.%s: %s", p.key.Value, msg)
		}
		if len(problems) == 0 {
			l.t.Fields[f.Name] = f
		}
		l.fieldLines[f.Name] = p.key.Line
	}
}

func checkDuplicates(mapping *yaml.Node, prefix, file string, c *collector) {
	seen := map[string]bool{}
	for _, p := range pairs(mapping) {
		if seen[p.key.Value] {
			c.errorf(at(file, p.key.Line), "duplicate key %s%s", prefix, p.key.Value)
		}
		seen[p.key.Value] = true
	}
}

func (l *templateLoad) parseDocumentTypes(mapping *yaml.Node) {
	checkDuplicates(mapping, "document_types.", manifestFile, l.c)
	for _, p := range pairs(mapping) {
		name := p.key.Value
		where := fmt.Sprintf("document_types.%s: ", name)
		if !documentTypeNameRE.MatchString(name) {
			l.c.errorf(at(manifestFile, p.key.Line), "%sinvalid document type name (must match %s)", where, documentTypeNameRE)
			continue
		}
		if p.value.Kind != yaml.MappingNode {
			l.c.errorf(at(manifestFile, p.value.Line), "%smust be a mapping", where)
			continue
		}
		l.t.DocumentTypes[name] = l.parseDocumentType(p.value, where)
	}
}

func (l *templateLoad) parseDocumentType(node *yaml.Node, where string) DocumentType {
	checkKeys(node, []string{"template", "fields", "description"}, where, manifestFile, l.c)
	var dt DocumentType
	tpl := valueOf(node, "template")
	if tpl == nil || tpl.Kind != yaml.ScalarNode || tpl.Value == "" {
		l.c.errorf(at(manifestFile, node.Line), "%stemplate (the Pandoc template file) is required", where)
	} else {
		dt.Template = tpl.Value
	}
	fields := valueOf(node, "fields")
	if fields == nil {
		return dt
	}
	if fields.Kind != yaml.SequenceNode {
		l.c.errorf(at(manifestFile, fields.Line), "%sfields must be a list", where)
		return dt
	}
	seen := map[string]bool{}
	for _, item := range fields.Content {
		ref, ok := l.parseFieldRef(item, where)
		if !ok {
			continue
		}
		if seen[ref.Name] {
			l.c.errorf(at(manifestFile, item.Line), "%sfield %q listed twice", where, ref.Name)
			continue
		}
		seen[ref.Name] = true
		dt.Fields = append(dt.Fields, ref)
	}
	return dt
}

// parseFieldRef reads a Document Type field entry: a name, or a one-key
// mapping from the name to its overrides (required, default).
func (l *templateLoad) parseFieldRef(item *yaml.Node, where string) (DocumentTypeField, bool) {
	switch item.Kind {
	case yaml.ScalarNode:
		return DocumentTypeField{Name: item.Value}, true
	case yaml.MappingNode:
		if len(item.Content) == 2 && item.Content[1].Kind == yaml.MappingNode {
			return l.parseOverrides(item.Content[0].Value, item.Content[1], where)
		}
	}
	l.c.errorf(at(manifestFile, item.Line), "%sa field entry is a name or a mapping name: {required, default}", where)
	return DocumentTypeField{}, false
}

func (l *templateLoad) parseOverrides(name string, node *yaml.Node, where string) (DocumentTypeField, bool) {
	ref := DocumentTypeField{Name: name}
	checkKeys(node, []string{"required", "default"}, where+name+": ", manifestFile, l.c)
	if v := valueOf(node, "required"); v != nil {
		var required bool
		if err := v.Decode(&required); err != nil {
			l.c.errorf(at(manifestFile, v.Line), "%s%s: required must be true or false", where, name)
			return ref, false
		}
		ref.Required = &required
	}
	if v := valueOf(node, "default"); v != nil {
		if err := v.Decode(&ref.Default); err != nil {
			l.c.errorf(at(manifestFile, v.Line), "%s%s: default: %v", where, name, err)
			return ref, false
		}
		ref.HasDefault = true
	}
	return ref, true
}

// checkDocumentTypes verifies the field lists against the declarations,
// then reads, scans and cross-checks every Pandoc template.
func (l *templateLoad) checkDocumentTypes() {
	for _, name := range sortedKeys(l.t.DocumentTypes) {
		dt := l.t.DocumentTypes[name]
		where := fmt.Sprintf("document_types.%s: ", name)
		l.checkFieldRefs(dt, where)
		if dt.Template != "" {
			l.checkPandocTemplate(name, dt)
		}
	}
}

func (l *templateLoad) checkFieldRefs(dt DocumentType, where string) {
	for _, ref := range dt.Fields {
		field, declared := l.t.Fields[ref.Name]
		if !declared {
			if _, known := l.fieldLines[ref.Name]; !known {
				l.c.errorf(manifestFile, "%slists field %q which is not declared in fields", where, ref.Name)
			}
			continue
		}
		if ref.HasDefault {
			if err := field.Check(ref.Default); err != nil {
				l.c.errorf(manifestFile, "%sfield %q default: %v", where, ref.Name, err)
			}
		}
	}
}

func (l *templateLoad) checkPandocTemplate(docType string, dt DocumentType) {
	path, err := confine(l.t.Dir, dt.Template)
	if err != nil {
		l.c.errorf(manifestFile, "document_types.%s: template: %v", docType, err)
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		l.c.errorf(manifestFile, "document_types.%s: template: %v", docType, err)
		return
	}
	l.scanned[filepath.Clean(dt.Template)] = true
	parsed := parsePandoc(string(data))
	l.reportIssues(dt.Template, parsed.Issues)
	l.reportIssues(dt.Template, scanTeX(parsed.Skeleton, l.sharedHas))
	l.checkVariables(docType, dt, parsed)
}

func (l *templateLoad) reportIssues(file string, issues []texIssue) {
	for _, issue := range issues {
		l.c.errorf(at(file, issue.Line), "%s", issue.Msg)
	}
}

func (l *templateLoad) sharedHas(name string) bool {
	_, err := confine(filepath.Join(l.t.Dir, sharedDir), name)
	return err == nil
}

// checkVariables is the drift guard for one Pandoc template: every
// variable it uses must be declared and listed for the Document Type, and
// a variable in a non-text position needs a strict pattern.
func (l *templateLoad) checkVariables(docType string, dt DocumentType, parsed pandocParse) {
	listed := map[string]bool{}
	for _, ref := range dt.Fields {
		listed[ref.Name] = true
	}
	for _, use := range parsed.Uses {
		l.used[use.Name] = true
		if pandocBuiltins[use.Name] {
			continue
		}
		loc := at(dt.Template, use.Line)
		field, declared := l.t.Fields[use.Name]
		switch {
		case !declared && !l.hasDeclaration(use.Name):
			l.c.errorf(loc, "drift: variable %q is used but not declared in the manifest fields", use.Name)
		case !listed[use.Name]:
			l.c.errorf(loc, "drift: variable %q is used but document type %q does not list it", use.Name, docType)
		case declared && use.Kind == useValue:
			l.checkPosition(field, parsed, use, loc)
		}
	}
}

// hasDeclaration is true for a field whose definition failed to parse; the
// definition error is already reported, so no drift error is added.
func (l *templateLoad) hasDeclaration(name string) bool {
	_, known := l.fieldLines[name]
	return known
}

func (l *templateLoad) checkPosition(f Field, parsed pandocParse, use varUse, loc string) {
	if f.Type.IsNonText() || f.Type == TypeBool {
		return // colour, asset, enum: strictness already enforced on the definition
	}
	cmd, nonText := nonTextCommand(parsed.Skeleton, use.SkeletonPos)
	if !nonText {
		return
	}
	if problem := f.strictPatternProblem(); problem != "" {
		l.c.errorf(loc, "field %q is used in a non-text position (%s) and needs a strict typed pattern: %s", f.Name, positionName(cmd), problem)
	}
}

func positionName(cmd string) string {
	if cmd == "" {
		return "an option list"
	}
	return `\` + cmd
}

func (l *templateLoad) warnUnusedFields() {
	for _, name := range sortedKeys(l.fieldLines) {
		if !l.used[name] {
			l.c.warnf(at(manifestFile, l.fieldLines[name]), "field %q is declared but never used in a Pandoc template", name)
		}
	}
}

func (l *templateLoad) lintStandardFields() {
	for _, docType := range sortedKeys(l.t.DocumentTypes) {
		for _, ref := range l.t.DocumentTypes[docType].Fields {
			f, ok := l.t.Fields[ref.Name]
			if !ok {
				continue
			}
			if msg := standardLint(docType, f); msg != "" {
				l.c.warnf(at(manifestFile, l.fieldLines[f.Name]), "%s", msg)
			}
		}
	}
}

// scanRemainingFiles applies the deny-list to every TeX file that is not
// a Document Type's Pandoc template, and rejects Lua files outright.
func (l *templateLoad) scanRemainingFiles(files []string) {
	for _, rel := range files {
		ext := strings.ToLower(filepath.Ext(rel))
		switch {
		case l.scanned[filepath.Clean(rel)]:
		case ext == ".lua":
			l.c.errorf(rel, "Lua files are not allowed in a Template")
		case ext == ".latex":
			l.scanFile(rel, true)
		case slices.Contains(texExtensions, ext):
			l.scanFile(rel, false)
		}
	}
}

func (l *templateLoad) scanFile(rel string, pandoc bool) {
	data, err := os.ReadFile(filepath.Join(l.t.Dir, rel))
	if err != nil {
		l.c.errorf(rel, "cannot read: %v", err)
		return
	}
	text := string(data)
	if pandoc {
		parsed := parsePandoc(text)
		l.reportIssues(rel, parsed.Issues)
		text = parsed.Skeleton
	}
	l.reportIssues(rel, scanTeX(text, l.sharedHas))
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
