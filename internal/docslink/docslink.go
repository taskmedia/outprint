// Package docslink builds the links from the product to the documentation
// site, so that an error points at the docs of the version that is running.
//
// The base URL is a constant, not a setting: the docs live at one address.
// The site serves the newest release at its root, every release at
// /<major>.<minor>/ (the root of the newest one redirects there, see
// website/docusaurus.config.js) and the unreleased docs at /dev/.
//
// Mapping of the embedded version (internal/version) to a docs path:
//
//	0.3.2, v0.3.2          0.3     a release: the docs of its minor
//	dev, 0.3.2-dev,        dev     a development build: the unreleased docs
//	*-dirty, v0.3.2-5-gabc1234,
//	abc1234 (commit only)
//	0.4.0-rc.1             latest  a pre-release has no docs version of its own
//	"", banana, 0.3        latest  unknown or unparsable: the site root
package docslink

import (
	"regexp"
	"strings"
)

const (
	// BaseURL is the address of the documentation site (no trailing slash).
	BaseURL = "https://outprint.task.media"

	// ErrorsPage is the docs page with one heading, and so one anchor, per
	// error code: docs/reference/error-messages.md.
	ErrorsPage = "reference/error-messages"

	devPath = "dev"
)

var (
	releaseRE = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
	// A build from a git checkout: "-dev", "-dirty", the "-<n>-g<sha>" that
	// `git describe` appends after a tag, or a bare abbreviated commit.
	devBuildRE = regexp.MustCompile(`(-dev|-dirty|-[0-9]+-g[0-9a-f]{4,40})$|^[0-9a-f]{7,40}$`)
	// The shape of an error code: the same tokens the Gateway puts in its
	// error bodies. Anything else gets no link.
	codeRE = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)
)

// VersionPath returns the folder of the docs for a version, without slashes:
// "0.3", "dev", or "" for the latest docs at the site root.
func VersionPath(version string) string {
	v := strings.TrimPrefix(strings.TrimSpace(version), "v")
	if v == "dev" || devBuildRE.MatchString(v) {
		return devPath
	}
	if m := releaseRE.FindStringSubmatch(v); m != nil {
		return m[1] + "." + m[2]
	}
	return ""
}

// Home is the start page of the docs for a version, with a trailing slash.
func Home(version string) string {
	if p := VersionPath(version); p != "" {
		return BaseURL + "/" + p + "/"
	}
	return BaseURL + "/"
}

// ErrorURL is the link to the entry for an error code in the docs for a
// version. It is empty for a code that is not a plain token, so nothing odd
// ends up in a URL.
func ErrorURL(version, code string) string {
	if !codeRE.MatchString(code) {
		return ""
	}
	return Home(version) + ErrorsPage + "/#" + code
}
