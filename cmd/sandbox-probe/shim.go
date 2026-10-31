//go:build linux

package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"syscall"
)

// The intentionally unsafe pipeline. In the sandbox image this binary stands
// in for lualatex, ahead of the real one on PATH. A Document that contains
// the word probeaction-<name> picks a hostile action; any other Document gets
// the real lualatex, so the rest of the pipeline (job directory, Pandoc,
// filters, limits, process groups, timeouts) is the shipped one.
const realLualatex = "/usr/bin/lualatex"

var (
	actionRE = regexp.MustCompile(`probeaction-([a-z]+)`)
	paramsRE = regexp.MustCompile(`probeparams-([0-9a-f]+)`)
)

func runShim(args []string) int {
	data, err := os.ReadFile("doc.tex")
	if err != nil {
		return execRealLualatex(args)
	}
	match := actionRE.FindSubmatch(data)
	if match == nil {
		return execRealLualatex(args)
	}
	switch string(match[1]) {
	case "loop":
		return texLoop(args)
	case "memory":
		memoryBomb()
	case "forkbomb":
		return shimForkBomb()
	case "fill":
		return fillDisk()
	case "checks":
		return shimChecks(data)
	}
	fmt.Fprintf(os.Stderr, "unknown probe action %q\n", match[1])
	return 2
}

func execRealLualatex(args []string) int {
	err := syscall.Exec(realLualatex, append([]string{"lualatex"}, args...), os.Environ())
	fmt.Fprintln(os.Stderr, "exec lualatex:", err)
	return 127
}

// texLoop replaces the document with a TeX infinite loop and runs the real
// engine on it, so the loop runs under the real lockdown and limits.
func texLoop(args []string) int {
	const loop = "\\documentclass{article}\n\\begin{document}\n\\def\\spin{\\spin}\\spin\n\\end{document}\n"
	if err := os.WriteFile("doc.tex", []byte(loop), 0o600); err != nil {
		return 1
	}
	return execRealLualatex(args)
}

// shimForkBomb starts idle processes, each in its own session so the
// Worker's process-group kill does not reach them, until the system refuses;
// then it idles.
func shimForkBomb() int {
	for i := 0; i < forkBombAttempts; i++ {
		cmd := exec.Command("/proc/self/exe", "hold")
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		if err := cmd.Start(); err != nil {
			break
		}
	}
	holdForever()
	return 0
}

// fillDisk writes into the job directory until the tmpfs is full. It leaves
// the file in place and fails, as a tool that died of a full disk would.
func fillDisk() int {
	f, err := os.Create("fill.bin")
	if err != nil {
		return 1
	}
	chunk := make([]byte, 1<<20)
	for {
		if _, err := f.Write(chunk); err != nil {
			fmt.Fprintln(os.Stderr, "write:", errnoName(err))
			return 1
		}
	}
}

// shimChecks runs the containment checks inside a real job and returns the
// report as the job's result, so the Worker hands it back like a PDF.
func shimChecks(doc []byte) int {
	var p params
	if m := paramsRE.FindSubmatch(doc); m != nil {
		raw, err := hex.DecodeString(string(m[1]))
		if err != nil || json.Unmarshal(raw, &p) != nil {
			fmt.Fprintln(os.Stderr, "bad probe parameters")
			return 2
		}
	}
	body, err := json.Marshal(buildReport(p))
	if err != nil || os.WriteFile("doc.pdf", body, 0o600) != nil {
		return 1
	}
	log := fmt.Sprintf("Output written on doc.pdf (1 page, %d bytes).\n", len(body))
	if os.WriteFile("doc.log", []byte(log), 0o600) != nil {
		return 1
	}
	return 0
}
