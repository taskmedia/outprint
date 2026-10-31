package packages

import (
	"fmt"
	"strings"
)

// Severity of a finding. Errors stop startup and fail `validate`.
type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
)

// Finding is one problem or remark about a package or the service setup.
type Finding struct {
	Severity Severity
	Package  string // for example "template/neutral"
	Location string // for example "manifest.yaml:12"
	Message  string
}

// String renders one line: severity, package, location, message.
func (f Finding) String() string {
	return fmt.Sprintf("%-7s %s %s: %s", f.Severity, f.Package, f.Location, oneLine(f.Message))
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// Errors returns the findings with error severity.
func Errors(findings []Finding) []Finding { return filter(findings, SeverityError) }

// Warnings returns the findings with warning severity.
func Warnings(findings []Finding) []Finding { return filter(findings, SeverityWarning) }

func filter(findings []Finding, s Severity) []Finding {
	var out []Finding
	for _, f := range findings {
		if f.Severity == s {
			out = append(out, f)
		}
	}
	return out
}

// collector gathers the findings of one package.
type collector struct {
	pkg      string
	findings []Finding
}

func newCollector(kind, name string) *collector {
	return &collector{pkg: kind + "/" + name}
}

func (c *collector) errorf(location, format string, args ...any) {
	c.add(SeverityError, location, format, args...)
}

func (c *collector) warnf(location, format string, args ...any) {
	c.add(SeverityWarning, location, format, args...)
}

func (c *collector) add(s Severity, location, format string, args ...any) {
	c.findings = append(c.findings, Finding{Severity: s, Package: c.pkg, Location: location, Message: fmt.Sprintf(format, args...)})
}

func (c *collector) hasErrors() bool { return len(Errors(c.findings)) > 0 }

func at(file string, line int) string { return fmt.Sprintf("%s:%d", file, line) }

// dedupe drops exact duplicates, keeping order.
func dedupe(findings []Finding) []Finding {
	seen := map[Finding]bool{}
	var out []Finding
	for _, f := range findings {
		if !seen[f] {
			seen[f] = true
			out = append(out, f)
		}
	}
	return out
}

// downgrade turns errors into warnings.
func downgrade(findings []Finding) []Finding {
	out := make([]Finding, len(findings))
	for i, f := range findings {
		f.Severity = SeverityWarning
		out[i] = f
	}
	return out
}
