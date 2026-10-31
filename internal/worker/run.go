package worker

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"syscall"
	"time"
)

const (
	maxCapturedOutput = 8 << 20
	killWaitDelay     = 2 * time.Second
	exitCodeUnknown   = -1
)

// toolResult is what a finished tool left behind. Output is only for the
// worker's own inspection (a filter marker, a TeX rerun hint) and is never
// returned to callers.
type toolResult struct {
	Output   string
	ExitCode int
	Duration time.Duration
	// TimedOut is true when the job context ended while the tool ran.
	TimedOut bool
	// Signaled is true when a signal ended the tool that the Worker did not
	// send: the kernel killed it, typically for lack of memory.
	Signaled bool
}

// cappedBuffer keeps the first maxCapturedOutput bytes and drops the rest,
// so a runaway tool cannot exhaust memory.
type cappedBuffer struct{ buf bytes.Buffer }

func (c *cappedBuffer) Write(p []byte) (int, error) {
	if room := maxCapturedOutput - c.buf.Len(); room > 0 {
		c.buf.Write(p[:min(room, len(p))])
	}
	return len(p), nil
}

// runTool executes one tool in its own process group with the given
// environment. When the context ends the whole group is killed; the group is
// also killed after a normal exit so no stray child outlives the tool.
func runTool(ctx context.Context, dir string, env []string, name string, args []string) (toolResult, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Env = env
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return killGroup(cmd) }
	cmd.WaitDelay = killWaitDelay
	var out cappedBuffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	start := time.Now()
	err := cmd.Run()
	killGroup(cmd)

	res := toolResult{
		Output:   out.buf.String(),
		ExitCode: exitCodeUnknown,
		Duration: time.Since(start),
		TimedOut: ctx.Err() != nil,
	}
	if cmd.ProcessState != nil {
		res.ExitCode = cmd.ProcessState.ExitCode()
		res.Signaled = wasSignaled(cmd.ProcessState)
	}
	return res, err
}

func wasSignaled(state *os.ProcessState) bool {
	status, ok := state.Sys().(syscall.WaitStatus)
	return ok && status.Signaled()
}

func killGroup(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
}
