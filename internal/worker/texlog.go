package worker

import (
	"regexp"
	"strconv"
	"strings"
)

var outputWrittenRE = regexp.MustCompile(`(?m)^Output written on doc\.pdf \((\d+) pages?, \d+ bytes\)`)

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

// pagesFromTeXLog returns the page count LuaLaTeX reports for the job's PDF.
func pagesFromTeXLog(log string) (int, bool) {
	m := outputWrittenRE.FindStringSubmatch(log)
	if m == nil {
		return 0, false
	}
	pages, err := strconv.Atoi(m[1])
	return pages, err == nil
}
