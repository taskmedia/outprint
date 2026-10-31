//go:build linux

package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"syscall"
)

const forkBombAttempts = 1000

// forkBombCheck runs a fork bomb in a child, in its own process group, and
// counts how many processes it got. The container's pids limit must stop it.
// The count is read from the child's output, so it survives the child
// crashing when the limit starves its runtime.
func forkBombCheck(limit int) check {
	c := check{Name: "fork_bomb"}
	cmd := exec.Command("/proc/self/exe", "forkbomb-child")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		c.Detail = "cannot start: " + err.Error()
		return c
	}
	if err := cmd.Start(); err != nil {
		c.Detail = "cannot start: " + errnoName(err)
		return c
	}
	spawned := lastCount(bufio.NewScanner(stdout))
	_ = cmd.Wait()
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	c.Detail = fmt.Sprintf("spawned %d", spawned)
	c.Breach = spawned >= limit || spawned >= forkBombAttempts
	return c
}

func lastCount(s *bufio.Scanner) int {
	last := 0
	for s.Scan() {
		if n, err := strconv.Atoi(s.Text()); err == nil {
			last = n
		}
	}
	return last
}

// runForkBombChild starts idle copies of itself until the system refuses,
// printing the running count.
func runForkBombChild() int {
	spawned := 0
	for i := 0; i < forkBombAttempts; i++ {
		cmd := exec.Command("/proc/self/exe", "hold")
		if err := cmd.Start(); err != nil {
			fmt.Fprintln(os.Stderr, "fork refused:", errnoName(err))
			return 0
		}
		spawned++
		fmt.Println(spawned)
	}
	return 0
}
