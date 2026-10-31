package worker

import "strings"

// firstTeXErrorLine returns the first line of a TeX log that starts with "!",
// which is where TeX reports an error, or "" if there is none.
func firstTeXErrorLine(log string) string {
	for _, line := range strings.Split(log, "\n") {
		if strings.HasPrefix(line, "!") {
			return strings.TrimSpace(line)
		}
	}
	return ""
}

// needsRerun reports whether the log asks for another run, for example
// because a table of contents or a reference changed.
func needsRerun(log string) bool {
	return strings.Contains(log, "Rerun to get") || strings.Contains(log, "Label(s) may have changed")
}
