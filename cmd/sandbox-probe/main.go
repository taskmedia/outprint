//go:build linux

// Command sandbox-probe is the test-only adversary of the sandbox security
// suite (docs/internals/security.md, layers 2 and 3). It runs hostile actions as the
// Worker user, inside the container flags the shipped Compose file sets, and
// reports what was contained. It is built into the "sandbox" image stage
// only; the release image must not contain it.
//
//	sandbox-probe checks [flags]   run the containment checks, print a JSON report
//	sandbox-probe plant DIR TEXT   write DIR/secret, then hold
//	sandbox-probe read PATH        print the size and SHA-256 of a file, never its content
//	sandbox-probe listen ADDR TEXT answer every connection with TEXT
//	sandbox-probe dial ADDR        connect and print the first line the peer sends
//	sandbox-probe resolve NAME     print the addresses of a name
//	sandbox-probe memory-bomb      allocate memory until the kernel kills the process
//	sandbox-probe post URL         send a bundle (stdin) to a Worker, print the outcome
//	sandbox-probe procs | ls DIR | hold
//
// Called as "lualatex" it is the intentionally unsafe pipeline: see shim.go.
package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	if filepath.Base(os.Args[0]) == "lualatex" {
		os.Exit(runShim(os.Args[1:]))
	}
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: sandbox-probe checks|plant|read|listen|memory-bomb|post|procs|ls|hold")
		os.Exit(2)
	}
	os.Exit(dispatch(os.Args[1], os.Args[2:]))
}

func dispatch(command string, args []string) int {
	switch command {
	case "checks":
		return runChecks(args)
	case "plant":
		return runPlant(args)
	case "read":
		return runRead(args)
	case "listen":
		return runListen(args)
	case "memory-bomb":
		memoryBomb()
		return 0
	case "dial":
		return runDial(args)
	case "resolve":
		return runResolve(args)
	case "post":
		return runPost(args)
	case "procs":
		return runProcs()
	case "ls":
		return runLs(args)
	case "hold":
		holdForever()
		return 0
	case "ptrace-child":
		return runPtraceChild(args)
	case "forkbomb-child":
		return runForkBombChild()
	}
	fmt.Fprintf(os.Stderr, "sandbox-probe: unknown command %q\n", command)
	return 2
}
