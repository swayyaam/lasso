package binaries

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

func TestVerifyReportsEveryBinary(t *testing.T) {
	m, _ := newFakeManager(t)
	ctx := context.Background()
	if _, err := m.Install(ctx); err != nil {
		t.Fatalf("Install: %v", err)
	}

	results := m.Verify(ctx)
	if len(results) != len(requiredBinaries) {
		t.Fatalf("got %d results, want %d", len(results), len(requiredBinaries))
	}
	for _, r := range results {
		if !r.OK {
			t.Errorf("%s did not run: %v", r.Name, r.Err)
		}
	}
	if len(Failures(results)) != 0 {
		t.Errorf("Failures = %v, want none", Failures(results))
	}
}

func TestVerifyCapturesVersionStrings(t *testing.T) {
	m, _ := newFakeManager(t)
	ctx := context.Background()
	if _, err := m.Install(ctx); err != nil {
		t.Fatalf("Install: %v", err)
	}

	want := map[Name]string{
		YtDlp:   "2026.08.19",
		FFmpeg:  "ffmpeg version 9.0.1",
		FFprobe: "ffprobe version 9.0.1",
		Deno:    "deno 2.9.6",
	}
	for _, r := range m.Verify(ctx) {
		if r.Version != want[r.Name] {
			t.Errorf("%s version = %q, want %q", r.Name, r.Version, want[r.Name])
		}
	}
}

func TestVerifyDetectsMissingBinary(t *testing.T) {
	m, _ := newFakeManager(t)
	ctx := context.Background()
	if _, err := m.Install(ctx); err != nil {
		t.Fatalf("Install: %v", err)
	}
	if err := os.Remove(m.Path(Deno)); err != nil {
		t.Fatal(err)
	}

	failures := Failures(m.Verify(ctx))
	if len(failures) != 1 || failures[0].Name != Deno {
		t.Fatalf("failures = %v, want just deno", failures)
	}

	var notInstalled *NotInstalledError
	if !errors.As(failures[0].Err, &notInstalled) {
		t.Fatalf("error is %T, want *NotInstalledError", failures[0].Err)
	}
	if msg := UserMessage(failures[0].Err); !strings.Contains(msg, "deno") {
		t.Errorf("user message %q does not name the binary", msg)
	}
}

func TestVerifyDetectsBinaryThatWillNotRun(t *testing.T) {
	m, _ := newFakeManager(t)
	ctx := context.Background()
	if _, err := m.Install(ctx); err != nil {
		t.Fatalf("Install: %v", err)
	}

	broken := "#!/bin/sh\necho 'dyld: bad CPU type in executable' >&2\nexit 1\n"
	if err := os.WriteFile(m.Path(FFmpeg), []byte(broken), 0o755); err != nil {
		t.Fatal(err)
	}

	failures := Failures(m.Verify(ctx))
	if len(failures) != 1 || failures[0].Name != FFmpeg {
		t.Fatalf("failures = %v, want just ffmpeg", failures)
	}

	var verifyErr *VerifyError
	if !errors.As(failures[0].Err, &verifyErr) {
		t.Fatalf("error is %T, want *VerifyError", failures[0].Err)
	}
	// The raw output stays available for the details toggle.
	if !strings.Contains(verifyErr.Output, "bad CPU type") {
		t.Errorf("raw output %q lost the underlying message", verifyErr.Output)
	}
	if msg := UserMessage(failures[0].Err); !strings.Contains(msg, "different kind of Mac") {
		t.Errorf("user message = %q, want the wrong-architecture wording", msg)
	}
}

func TestVerifyDetectsNonExecutableBinary(t *testing.T) {
	m, _ := newFakeManager(t)
	ctx := context.Background()
	if _, err := m.Install(ctx); err != nil {
		t.Fatalf("Install: %v", err)
	}
	if err := os.Chmod(m.Path(FFprobe), 0o644); err != nil {
		t.Fatal(err)
	}

	failures := Failures(m.Verify(ctx))
	if len(failures) != 1 || failures[0].Name != FFprobe {
		t.Fatalf("failures = %v, want just ffprobe", failures)
	}
	if msg := UserMessage(failures[0].Err); !strings.Contains(msg, "not allowed to run") {
		t.Errorf("user message = %q, want the permission wording", msg)
	}
}

func TestVerifyErrorUserMessages(t *testing.T) {
	cases := []struct {
		name   string
		output string
		want   string
	}{
		{"wrong arch", "dyld: bad CPU type in executable", "different kind of Mac"},
		{"gatekeeper", "signal: killed", "macOS blocked"},
		{"bad signature", "code signature invalid", "macOS blocked"},
		{"permissions", "fork/exec: permission denied", "not allowed to run"},
		{"timeout", "timed out", "did not respond"},
		{"unknown", "something unexpected happened", "could not be started"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := &VerifyError{Name: YtDlp, Path: "/tmp/yt-dlp", Output: tc.output, Err: errors.New("exit status 1")}
			if got := err.UserMessage(); !strings.Contains(got, tc.want) {
				t.Errorf("UserMessage() = %q, want it to contain %q", got, tc.want)
			}
		})
	}
}

func TestVerifyRespectsCancellation(t *testing.T) {
	m, _ := newFakeManager(t)
	if _, err := m.Install(context.Background()); err != nil {
		t.Fatalf("Install: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if failures := Failures(m.Verify(ctx)); len(failures) != len(requiredBinaries) {
		t.Errorf("got %d failures on a cancelled context, want all %d", len(failures), len(requiredBinaries))
	}
}

func TestVerifyRunsTheBinariesConcurrently(t *testing.T) {
	// Verify doubles as the first-launch warm-up, where each binary pays
	// macOS's signature scan — around six seconds for yt-dlp's onedir build.
	// Serially that is the sum of four scans; together it is the longest one.
	// The waiting is not this process's work to do, so it should overlap.
	m, _ := newFakeManager(t)
	ctx := context.Background()
	if _, err := m.Install(ctx); err != nil {
		t.Fatalf("Install: %v", err)
	}

	// Replace each fake with one that sleeps, so overlap is measurable
	// rather than inferred.
	const nap = 200 * time.Millisecond
	for _, name := range requiredBinaries {
		path := m.Path(name)
		script := "#!/bin/sh\nsleep 0.2\necho " + string(name) + " 1.0\n"
		if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	start := time.Now()
	results := m.Verify(ctx)
	elapsed := time.Since(start)

	serial := nap * time.Duration(len(requiredBinaries))
	if elapsed >= serial {
		t.Errorf("Verify took %v for %d binaries sleeping %v each; serial would be %v, so nothing overlapped",
			elapsed, len(requiredBinaries), nap, serial)
	}

	// Order is part of the contract: the startup screen lists these, and a
	// list ordered by whichever binary answered first would read oddly.
	if len(results) != len(requiredBinaries) {
		t.Fatalf("got %d results, want %d", len(results), len(requiredBinaries))
	}
	for i, want := range requiredBinaries {
		if results[i].Name != want {
			t.Errorf("results[%d] = %s, want %s — order was not preserved", i, results[i].Name, want)
		}
	}
}
