package core

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

// gracePeriod is how long a cancelled yt-dlp gets to shut down cleanly before
// the process group is killed outright.
const gracePeriod = 3 * time.Second

// maxLine bounds a single line of output. yt-dlp's -J emits the whole metadata
// document on one line — comfortably past bufio's 64 KB default, and far more
// for a large playlist — so the limit is raised rather than left to truncate.
const maxLine = 16 << 20

// Runner executes yt-dlp and streams its output line by line.
//
// Implementations must kill the entire process group on cancellation: yt-dlp
// spawns ffmpeg for merging and post-processing, and killing only the parent
// leaves ffmpeg running against a file nobody is waiting for.
type Runner interface {
	Run(ctx context.Context, args []string, stdout, stderr func(line string)) error
}

// ExecRunner runs the real yt-dlp binary.
type ExecRunner struct {
	// Path is the yt-dlp executable.
	Path string
	// Env is the environment for the subprocess, including a PATH that reaches
	// the bundled deno.
	Env []string
	// Dir is the working directory, if any.
	Dir string
}

var _ Runner = (*ExecRunner)(nil)

// Run starts yt-dlp and calls stdout and stderr for each line produced. It
// returns when the process exits, the context is cancelled, or output cannot be
// read.
func (r *ExecRunner) Run(ctx context.Context, args []string, stdout, stderr func(string)) error {
	cmd := exec.CommandContext(ctx, r.Path, args...)
	cmd.Env = r.Env
	cmd.Dir = r.Dir

	// Put the child in its own process group so signals reach its descendants.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	// exec.CommandContext would otherwise kill only the direct child.
	cmd.Cancel = func() error {
		return killGroup(cmd.Process.Pid)
	}
	// A backstop if the group ignores SIGTERM and the SIGKILL below is missed.
	cmd.WaitDelay = gracePeriod + time.Second

	outPipe, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("capturing yt-dlp output: %w", err)
	}
	errPipe, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("capturing yt-dlp errors: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting yt-dlp: %w", err)
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); scanLines(outPipe, stdout) }()
	go func() { defer wg.Done(); scanLines(errPipe, stderr) }()
	wg.Wait()

	return cmd.Wait()
}

// killGroup signals the whole process group, escalating to SIGKILL if anything
// is still alive after the grace period.
func killGroup(pid int) error {
	if pid <= 0 {
		return nil
	}
	// The negative pid targets the group, which Setpgid made equal to the pid.
	if err := syscall.Kill(-pid, syscall.SIGTERM); err != nil {
		return err
	}
	time.AfterFunc(gracePeriod, func() {
		_ = syscall.Kill(-pid, syscall.SIGKILL)
	})
	return nil
}

func scanLines(r io.Reader, onLine func(string)) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), maxLine)
	for scanner.Scan() {
		if onLine != nil {
			onLine(scanner.Text())
		}
	}
}

// discard is a convenience for callers that ignore one stream.
func discard(string) {}
