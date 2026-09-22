package binaries

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// verifyTimeout is generous on purpose. The first run of a freshly installed
// binary pays a one-time Gatekeeper scan — for yt-dlp's onedir build, which has
// hundreds of files under _internal, that measured around six seconds.
const verifyTimeout = 90 * time.Second

// VerifyResult is the outcome of running one binary's version command.
type VerifyResult struct {
	Name     Name
	Path     string
	OK       bool
	Version  string
	Duration time.Duration
	Err      error
}

// Verify runs every sidecar binary's version command and reports what happened.
//
// It doubles as the warm-up for the first launch after an install: the initial
// exec of each binary triggers macOS's signature scan, and paying that here
// keeps it out of the user's first download.
//
// The binaries are verified concurrently, which is what makes that warm-up
// bearable. The scan is the slow part and it is not this process doing the
// work, so four serial scans spend four times as long waiting as one round of
// four at once — around six seconds each on a first launch. Warm, it is the
// difference between yt-dlp's quarter-second and the sum of all four.
//
// Results stay in requiredBinaries order: the startup screen lists them, and a
// list that reorders itself by whichever binary answered first would be a
// strange thing to read.
func (m *Manager) Verify(ctx context.Context) []VerifyResult {
	results := make([]VerifyResult, len(requiredBinaries))

	var wg sync.WaitGroup
	for i, name := range requiredBinaries {
		wg.Add(1)
		go func(i int, name Name) {
			defer wg.Done()
			results[i] = m.verifyOne(ctx, name)
		}(i, name)
	}
	wg.Wait()

	return results
}

func (m *Manager) verifyOne(ctx context.Context, name Name) VerifyResult {
	path := m.Path(name)
	result := VerifyResult{Name: name, Path: path}

	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		result.Err = &NotInstalledError{Name: name, Path: path}
		return result
	}

	ctx, cancel := context.WithTimeout(ctx, verifyTimeout)
	defer cancel()

	start := time.Now()
	cmd := exec.CommandContext(ctx, path, versionArgs(name)...)
	cmd.Env = m.Environ()
	out, err := cmd.CombinedOutput()
	result.Duration = time.Since(start)

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			err = &VerifyError{Name: name, Path: path, Output: "timed out", Err: err}
		} else {
			err = &VerifyError{Name: name, Path: path, Output: string(out), Err: err}
		}
		result.Err = err
		return result
	}

	result.OK = true
	result.Version = firstLine(string(out))
	return result
}

// versionArgs is the cheapest invocation that proves a binary runs.
func versionArgs(name Name) []string {
	switch name {
	case FFmpeg, FFprobe:
		// ffmpeg prints its banner to stderr and exits non-zero without input
		// unless -version is given; -hide_banner keeps the output to one line.
		return []string{"-hide_banner", "-version"}
	default:
		return []string{"--version"}
	}
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(line)
}

// Failures returns only the results that did not run.
func Failures(results []VerifyResult) []VerifyResult {
	var bad []VerifyResult
	for _, r := range results {
		if !r.OK {
			bad = append(bad, r)
		}
	}
	return bad
}
