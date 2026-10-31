package packages

import (
	"fmt"
	"os"
	"slices"

	"github.com/taskmedia/outline-latex-pdf/internal/config"
)

// Roots are the two directories packages are installed in.
type Roots struct {
	Templates      string
	Configurations string
}

// Registry is the immutable result of a successful load: the Templates,
// the Configurations and the allowed Bindings. Accessors hand out copies.
type Registry struct {
	templates      map[string]Template
	configurations map[string]Configuration
	bindings       []config.Binding
	defaultBinding *config.Binding
}

// Template returns the Template called name.
func (r *Registry) Template(name string) (Template, bool) {
	t, ok := r.templates[name]
	return t.clone(), ok
}

// Configuration returns the Configuration called name.
func (r *Registry) Configuration(name string) (Configuration, bool) {
	c, ok := r.configurations[name]
	return c.clone(), ok
}

// TemplateNames lists the loaded Templates, sorted.
func (r *Registry) TemplateNames() []string { return sortedKeys(r.templates) }

// ConfigurationNames lists the loaded Configurations, sorted.
func (r *Registry) ConfigurationNames() []string { return sortedKeys(r.configurations) }

// Bindings returns the allowed Bindings.
func (r *Registry) Bindings() []config.Binding { return slices.Clone(r.bindings) }

// Allowed reports whether a Binding is on the allow-list.
func (r *Registry) Allowed(b config.Binding) bool { return slices.Contains(r.bindings, b) }

// DefaultBinding returns the default Binding, if one is configured.
func (r *Registry) DefaultBinding() (config.Binding, bool) {
	if r.defaultBinding == nil {
		return config.Binding{}, false
	}
	return *r.defaultBinding, true
}

// loaded collects what loading produced, per package.
type loaded struct {
	templates      map[string]*Template
	configurations map[string]*Configuration
	attempted      map[string]bool // packages that were found on disk and loaded
	findings       []Finding
}

func newLoaded() loaded {
	return loaded{
		templates:      map[string]*Template{},
		configurations: map[string]*Configuration{},
		attempted:      map[string]bool{},
	}
}

// Load loads and fully validates every package named by the allow-list.
// Packages on disk but not on the allow-list are loaded too, with a
// warning; their problems are reported as warnings because nothing can
// use them. The registry is nil if any error was found.
func Load(svc config.Service) (*Registry, []Finding) {
	roots := Roots{Templates: svc.TemplatesRoot, Configurations: svc.ConfigurationsRoot}
	listedT, listedC := listedNames(svc.Bindings)

	var findings []Finding
	onDiskT, f := discover(roots.Templates, "template")
	findings = append(findings, f...)
	onDiskC, f := discover(roots.Configurations, "configuration")
	findings = append(findings, f...)

	l := newLoaded()
	for _, name := range union(listedT, onDiskT) {
		listed := slices.Contains(listedT, name)
		l.loadTemplate(roots.Templates, name, listed, slices.Contains(onDiskT, name))
	}
	for _, name := range union(listedC, onDiskC) {
		listed := slices.Contains(listedC, name)
		l.loadConfiguration(roots.Configurations, name, listed, slices.Contains(onDiskC, name))
	}
	findings = append(findings, l.findings...)
	findings = append(findings, l.checkBindings(svc.Bindings)...)
	findings = dedupe(findings)

	if len(Errors(findings)) > 0 {
		return nil, findings
	}
	return &Registry{
		templates:      derefTemplates(l.templates),
		configurations: derefConfigurations(l.configurations),
		bindings:       slices.Clone(svc.Bindings),
		defaultBinding: svc.DefaultBinding,
	}, findings
}

// Inspect validates packages without a service configuration, for the
// author's `validate`. With no names it checks everything in the roots.
// With names it checks just those and, if both are given, their pairing.
func Inspect(roots Roots, template, configuration string) (*Registry, []Finding) {
	l := newLoaded()
	var templates, configurations []string
	var findings []Finding
	switch {
	case template == "" && configuration == "":
		var f []Finding
		templates, f = discover(roots.Templates, "template")
		findings = append(findings, f...)
		configurations, f = discover(roots.Configurations, "configuration")
		findings = append(findings, f...)
	default:
		if template != "" {
			templates = []string{template}
		}
		if configuration != "" {
			configurations = []string{configuration}
		}
	}
	for _, name := range templates {
		l.loadTemplate(roots.Templates, name, true, true)
	}
	for _, name := range configurations {
		l.loadConfiguration(roots.Configurations, name, true, true)
	}
	var bindings []config.Binding
	if template != "" && configuration != "" {
		bindings = []config.Binding{{Template: template, Configuration: configuration}}
	}
	findings = append(findings, l.findings...)
	findings = append(findings, l.checkBindings(bindings)...)
	findings = dedupe(findings)
	if len(Errors(findings)) > 0 {
		return nil, findings
	}
	return &Registry{
		templates:      derefTemplates(l.templates),
		configurations: derefConfigurations(l.configurations),
		bindings:       bindings,
	}, findings
}

func (l *loaded) loadTemplate(root, name string, listed, onDisk bool) {
	if !onDisk && listed {
		return // reported as a missing package by the binding check
	}
	l.attempted["template/"+name] = true
	t, findings := loadTemplate(root, name)
	l.record(findings, listed, "template", name)
	if t != nil {
		l.templates[name] = t
	}
}

func (l *loaded) loadConfiguration(root, name string, listed, onDisk bool) {
	if !onDisk && listed {
		return
	}
	l.attempted["configuration/"+name] = true
	c, findings := loadConfiguration(root, name)
	l.record(findings, listed, "configuration", name)
	if c != nil {
		l.configurations[name] = c
	}
}

// record keeps a package's findings; an unlisted package only warns.
func (l *loaded) record(findings []Finding, listed bool, kind, name string) {
	if listed {
		l.findings = append(l.findings, findings...)
		return
	}
	l.findings = append(l.findings, downgrade(findings)...)
	l.findings = append(l.findings, Finding{
		Severity: SeverityWarning, Package: kind + "/" + name, Location: ".",
		Message: "package is on disk but not on the allow-list; it is loaded but no Binding can use it",
	})
}

// checkBindings verifies the allow-list against what loaded and validates
// each pairing.
func (l *loaded) checkBindings(bindings []config.Binding) []Finding {
	var findings []Finding
	for _, b := range bindings {
		label := fmt.Sprintf("binding/%s/%s", b.Template, b.Configuration)
		c := &collector{pkg: label}
		t, tOK := l.templates[b.Template]
		cfg, cOK := l.configurations[b.Configuration]
		if !tOK && !l.attempted["template/"+b.Template] {
			c.errorf("bindings", "allow-list entry points at missing template %q", b.Template)
		}
		if !cOK && !l.attempted["configuration/"+b.Configuration] {
			c.errorf("bindings", "allow-list entry points at missing configuration %q", b.Configuration)
		}
		if tOK && cOK {
			checkBinding(*t, *cfg, c)
		}
		findings = append(findings, c.findings...)
	}
	return findings
}

func listedNames(bindings []config.Binding) (templates, configurations []string) {
	for _, b := range bindings {
		if !slices.Contains(templates, b.Template) {
			templates = append(templates, b.Template)
		}
		if !slices.Contains(configurations, b.Configuration) {
			configurations = append(configurations, b.Configuration)
		}
	}
	return templates, configurations
}

func union(a, b []string) []string {
	out := slices.Clone(a)
	for _, s := range b {
		if !slices.Contains(out, s) {
			out = append(out, s)
		}
	}
	slices.Sort(out)
	return out
}

// discover lists the package directories in root. Directories that are not
// valid package names only warn.
func discover(root, kind string) (names []string, findings []Finding) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, []Finding{{Severity: SeverityError, Package: "service", Location: kind + " root", Message: fmt.Sprintf("read %s: %v", root, err)}}
	}
	for _, e := range entries {
		info, err := os.Stat(root + string(os.PathSeparator) + e.Name())
		if err != nil || !info.IsDir() || e.Name()[0] == '.' {
			continue
		}
		if !config.NamePattern.MatchString(e.Name()) {
			findings = append(findings, Finding{Severity: SeverityWarning, Package: kind + "/" + e.Name(), Location: ".", Message: fmt.Sprintf("ignored: directory name must match %s", config.NamePattern)})
			continue
		}
		names = append(names, e.Name())
	}
	return names, findings
}

func derefTemplates(in map[string]*Template) map[string]Template {
	out := make(map[string]Template, len(in))
	for k, v := range in {
		out[k] = *v
	}
	return out
}

func derefConfigurations(in map[string]*Configuration) map[string]Configuration {
	out := make(map[string]Configuration, len(in))
	for k, v := range in {
		out[k] = *v
	}
	return out
}

// LoadTemplate loads one Template by name and fails if it has any error.
func LoadTemplate(root, name string) (Template, error) {
	if !config.NamePattern.MatchString(name) {
		return Template{}, fmt.Errorf("template %q: invalid name (must match %s)", name, config.NamePattern)
	}
	t, findings := loadTemplate(root, name)
	if t == nil {
		return Template{}, findingsError(findings)
	}
	return *t, nil
}

// LoadConfiguration loads one Configuration by name and fails if it has
// any error.
func LoadConfiguration(root, name string) (Configuration, error) {
	if !config.NamePattern.MatchString(name) {
		return Configuration{}, fmt.Errorf("configuration %q: invalid name (must match %s)", name, config.NamePattern)
	}
	c, findings := loadConfiguration(root, name)
	if c == nil {
		return Configuration{}, findingsError(findings)
	}
	return *c, nil
}

// FindingsError is an error carrying the error-level findings that caused it.
type FindingsError struct{ Findings []Finding }

func (e *FindingsError) Error() string {
	msg := ""
	for i, f := range e.Findings {
		if i > 0 {
			msg += "\n"
		}
		msg += f.String()
	}
	return msg
}

func findingsError(findings []Finding) error {
	return &FindingsError{Findings: Errors(findings)}
}
