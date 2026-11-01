package inject

import "strings"

const (
	sourceSelf = "'self'"
	sourceNone = "'none'"
)

// allowSelfScript returns the policy with 'self' allowed for scripts, so the
// script served below the page's own origin can load. A policy that does not
// restrict scripts is returned as is. With 'strict-dynamic' browsers ignore
// 'self'; that case needs a nonce and is not handled.
func allowSelfScript(policy string) string {
	directives := splitDirectives(policy)
	if len(directives) == 0 {
		return policy
	}
	touched := false
	var defaultSrc *directive
	hasScriptSrc := false
	for i := range directives {
		d := &directives[i]
		switch d.key {
		case "script-src", "script-src-elem":
			hasScriptSrc = true
			d.sources = withSelf(d.sources)
			touched = true
		case "default-src":
			defaultSrc = d
		}
	}
	if !hasScriptSrc && defaultSrc != nil && !allows(defaultSrc.sources) {
		directives = append(directives, directive{name: "script-src", key: "script-src", sources: withSelf(defaultSrc.sources)})
		touched = true
	}
	if !touched {
		return policy
	}
	parts := make([]string, len(directives))
	for i, d := range directives {
		parts[i] = d.String()
	}
	return strings.Join(parts, "; ")
}

type directive struct {
	name    string // as written
	key     string // lower case, for comparison
	sources []string
}

func (d directive) String() string {
	return strings.Join(append([]string{d.name}, d.sources...), " ")
}

func splitDirectives(policy string) []directive {
	var out []directive
	for _, raw := range strings.Split(policy, ";") {
		fields := strings.Fields(raw)
		if len(fields) == 0 {
			continue
		}
		out = append(out, directive{name: fields[0], key: strings.ToLower(fields[0]), sources: fields[1:]})
	}
	return out
}

// allows reports whether the sources already let the own origin through.
func allows(sources []string) bool {
	for _, s := range sources {
		if s == sourceSelf || s == "*" {
			return true
		}
	}
	return false
}

func withSelf(sources []string) []string {
	if allows(sources) {
		return sources
	}
	out := make([]string, 0, len(sources)+1)
	for _, s := range sources {
		if s != sourceNone {
			out = append(out, s)
		}
	}
	return append(out, sourceSelf)
}
