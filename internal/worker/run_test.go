package worker

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunToolReportsExitCodeAndOutput(t *testing.T) {
	res, err := runTool(context.Background(), t.TempDir(), []string{"PATH=/usr/bin:/bin"},
		"sh", []string{"-c", "echo out; echo err >&2; exit 3"})
	if err == nil || res.ExitCode != 3 {
		t.Fatalf("err = %v, exit = %d", err, res.ExitCode)
	}
	if !strings.Contains(res.Output, "out") || !strings.Contains(res.Output, "err") {
		t.Errorf("output = %q", res.Output)
	}
}

func TestRunToolKillsTheWholeProcessGroupOnTimeout(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "grandchild-survived")
	// The grandchild would create the marker after one second if it lived.
	script := "(sleep 1; touch " + marker + ") & sleep 30"
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	start := time.Now()
	res, err := runTool(ctx, dir, []string{"PATH=/usr/bin:/bin"}, "sh", []string{"-c", script})

	if err == nil || !res.TimedOut {
		t.Fatalf("err = %v, timedOut = %v; want a timeout", err, res.TimedOut)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("took %s to stop the tool", elapsed)
	}
	time.Sleep(1500 * time.Millisecond)
	if _, err := os.Stat(marker); err == nil {
		t.Error("a child of the tool survived the kill")
	}
}

func TestCappedBufferDropsOutputBeyondTheLimit(t *testing.T) {
	var b cappedBuffer
	chunk := []byte(strings.Repeat("x", 1<<20))
	for i := 0; i < maxCapturedOutput>>20+4; i++ {
		if n, err := b.Write(chunk); n != len(chunk) || err != nil {
			t.Fatalf("Write = %d, %v", n, err)
		}
	}
	if b.buf.Len() != maxCapturedOutput {
		t.Errorf("kept %d bytes, want %d", b.buf.Len(), maxCapturedOutput)
	}
}

func TestRunToolReportsAToolKilledBySignal(t *testing.T) {
	res, _ := runTool(context.Background(), t.TempDir(), []string{"PATH=/usr/bin:/bin"},
		"sh", []string{"-c", "kill -9 $$"})
	if !res.Signaled || res.TimedOut {
		t.Errorf("Signaled = %v, TimedOut = %v", res.Signaled, res.TimedOut)
	}
	res, _ = runTool(context.Background(), t.TempDir(), []string{"PATH=/usr/bin:/bin"}, "sh", []string{"-c", "exit 1"})
	if res.Signaled {
		t.Error("a plain failure is not a signal")
	}
}
