package core

import (
	"context"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// processAlive reports whether a pid still exists. Signal 0 performs the
// permission and existence checks without delivering anything.
func processAlive(pid int) bool {
	return syscall.Kill(pid, 0) == nil
}

func waitForExit(pid int, within time.Duration) bool {
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if !processAlive(pid) {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return !processAlive(pid)
}

// TestRunnerKillsWholeProcessGroup is the orphan-process guarantee: yt-dlp
// spawns ffmpeg, and cancelling a download must not leave it running.
//
// A shell stands in for yt-dlp and a background sleep for ffmpeg. Killing only
// the direct child would leave the sleep alive, reparented to launchd.
func TestRunnerKillsWholeProcessGroup(t *testing.T) {
	const script = `sleep 300 &
echo "GRANDCHILD $!"
wait`

	runner := &ExecRunner{Path: "/bin/sh"}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pids := make(chan int, 1)
	done := make(chan error, 1)

	go func() {
		done <- runner.Run(ctx, []string{"-c", script}, func(line string) {
			if rest, ok := strings.CutPrefix(line, "GRANDCHILD "); ok {
				if pid, err := strconv.Atoi(strings.TrimSpace(rest)); err == nil {
					pids <- pid
				}
			}
		}, discard)
	}()

	var grandchild int
	select {
	case grandchild = <-pids:
	case <-time.After(10 * time.Second):
		t.Fatal("the stand-in never reported its child pid")
	}

	if !processAlive(grandchild) {
		t.Fatalf("grandchild %d was never running", grandchild)
	}

	cancel()

	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("Run did not return after cancellation")
	}

	if !waitForExit(grandchild, 10*time.Second) {
		// Clean up so the test does not leak the process it just caught.
		_ = syscall.Kill(grandchild, syscall.SIGKILL)
		t.Fatalf("grandchild %d survived cancellation: ffmpeg would be orphaned", grandchild)
	}
}

func TestRunnerStreamsBothStreams(t *testing.T) {
	runner := &ExecRunner{Path: "/bin/sh"}

	var out, errs []string
	err := runner.Run(context.Background(),
		[]string{"-c", `echo one; echo two; echo problem >&2`},
		func(l string) { out = append(out, l) },
		func(l string) { errs = append(errs, l) },
	)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(out) != 2 || out[0] != "one" || out[1] != "two" {
		t.Errorf("stdout = %v, want [one two]", out)
	}
	if len(errs) != 1 || errs[0] != "problem" {
		t.Errorf("stderr = %v, want [problem]", errs)
	}
}

func TestRunnerReturnsExitError(t *testing.T) {
	runner := &ExecRunner{Path: "/bin/sh"}
	err := runner.Run(context.Background(), []string{"-c", "exit 3"}, discard, discard)
	if err == nil {
		t.Fatal("Run returned nil for a failing command")
	}
}

// TestRunnerHandlesLinesLargerThanBufioDefault covers yt-dlp -J, which emits an
// entire metadata document on a single line — far past bufio's 64 KB default.
func TestRunnerHandlesLinesLargerThanBufioDefault(t *testing.T) {
	const size = 300_000
	runner := &ExecRunner{Path: "/bin/sh"}

	var got string
	err := runner.Run(context.Background(),
		[]string{"-c", "printf 'x%.0s' $(seq 1 " + strconv.Itoa(size) + ")"},
		func(l string) { got = l },
		discard,
	)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(got) != size {
		t.Errorf("got %d bytes, want %d: a long line was truncated", len(got), size)
	}
}

func TestRunnerReportsMissingBinary(t *testing.T) {
	runner := &ExecRunner{Path: "/nonexistent/yt-dlp"}
	if err := runner.Run(context.Background(), nil, discard, discard); err == nil {
		t.Fatal("Run returned nil for a missing binary")
	}
}
