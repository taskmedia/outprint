//go:build linux

package main

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"syscall"
)

const prSetDumpable = 4

// ptraceCheck makes this process non-dumpable and lets a child try to attach
// to it. Without CAP_SYS_PTRACE a non-dumpable process cannot be attached to,
// whatever the kernel's ptrace_scope says.
func ptraceCheck() check {
	if err := rawSyscall(syscall.SYS_PRCTL, prSetDumpable, 0); err != nil {
		return check{Name: "ptrace", Detail: "cannot set up: " + errnoName(err)}
	}
	cmd := exec.Command("/proc/self/exe", "ptrace-child", strconv.Itoa(os.Getpid()))
	out, err := cmd.CombinedOutput()
	if err != nil {
		return check{Name: "ptrace", Breach: true, Detail: "child could attach: " + string(out)}
	}
	return check{Name: "ptrace", Errno: string(out), Detail: "denied"}
}

func runPtraceChild(args []string) int {
	pid, err := strconv.Atoi(args[0])
	if err != nil {
		return 2
	}
	runtime.LockOSThread()
	if err := syscall.PtraceAttach(pid); err != nil {
		fmt.Print(errnoName(err))
		return 0
	}
	_ = syscall.PtraceDetach(pid)
	return 1
}
