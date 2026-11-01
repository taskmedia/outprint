// Package version holds the build identity of the Outprint binaries.
//
// Release builds set Version (and optionally Commit) at link time:
//
//	go build -ldflags "-X github.com/taskmedia/outprint/internal/version.Version=1.2.3"
//
// Without it Version is "dev", and Commit falls back to the VCS revision the
// Go toolchain embeds when building inside a git checkout.
package version

import "runtime/debug"

// Version is the release version without a leading "v", or "dev".
var Version = "dev"

// Commit is the git revision of the build, empty when unknown.
var Commit = ""

const shortCommitLength = 12

func init() {
	if Commit != "" {
		return
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, s := range info.Settings {
			if s.Key == "vcs.revision" {
				Commit = s.Value
			}
		}
	}
}

// Line renders "<binary> <version>" with the short commit appended when known.
func Line(binary string) string {
	line := binary + " " + Version
	if Commit == "" {
		return line
	}
	short := Commit
	if len(short) > shortCommitLength {
		short = short[:shortCommitLength]
	}
	return line + " (commit " + short + ")"
}
