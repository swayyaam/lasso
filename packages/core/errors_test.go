package core

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestClassifyCookieAccessFailure(t *testing.T) {
	// Verbatim from a real run: Safari keeps its cookie jar inside a container
	// macOS protects, so Lasso is refused however the file permissions read.
	output := `ERROR: [Errno 1] Operation not permitted: ` +
		`'/Users/someone/Library/Containers/com.apple.Safari/Data/Library/Cookies/Cookies.binarycookies'`

	got := ClassifyError(output, errors.New("exit status 1"))
	if got.Kind != ErrCookieAccess {
		t.Fatalf("Kind = %q, want %q", got.Kind, ErrCookieAccess)
	}
	if !strings.Contains(got.Message, "Full Disk Access") {
		t.Errorf("message %q does not name the remedy", got.Message)
	}
	if got.Raw != strings.TrimSpace(output) {
		t.Error("the raw output must survive for the details toggle")
	}
}

func TestClassifyMissingCookieJar(t *testing.T) {
	// Verbatim from a real run, with the browser set to one that is not
	// installed. This is a different problem from a jar that will not open,
	// and Full Disk Access would do nothing for it.
	output := `ERROR: could not find chrome cookies database in "/Users/someone/Library/Application Support/Google/Chrome"`

	got := ClassifyError(output, errors.New("exit status 1"))
	if got.Kind != ErrCookieAccess {
		t.Fatalf("Kind = %q, want %q", got.Kind, ErrCookieAccess)
	}
	// Both causes have to be named. A protected location reports "no such
	// file" to a process without permission, so an installed browser Lasso may
	// not read looks exactly like one that was never installed — and claiming
	// only the first sends someone hunting for a browser already on their Mac.
	if !strings.Contains(got.Message, "not") || !strings.Contains(got.Message, "installed") {
		t.Errorf("message %q does not offer the missing-browser cause", got.Message)
	}
	if !strings.Contains(got.Message, "hiding") {
		t.Errorf("message %q does not offer the permission cause", got.Message)
	}
}

func TestRefusedJarBeatsMissingJar(t *testing.T) {
	// A locked jar often reports both shapes. The permission problem is the
	// one the user can act on.
	output := `ERROR: Operation not permitted: could not find cookies database`

	if got := ClassifyError(output, errors.New("exit status 1")); !strings.Contains(got.Message, "Full Disk Access") {
		t.Errorf("message = %q, want the permission remedy to win", got.Message)
	}
}

func TestCookieAccessDoesNotClaimAProtectedDownloadFolder(t *testing.T) {
	// Same refusal from the operating system, entirely different cause. Sending
	// this user to Full Disk Access for their cookies would be a dead end.
	output := `ERROR: unable to open for writing: [Errno 1] Operation not permitted: '/protected/clip.mp4'`

	if got := ClassifyError(output, errors.New("exit status 1")); got.Kind == ErrCookieAccess {
		t.Errorf("Kind = %q, want a write failure not to be read as a cookie problem", got.Kind)
	}
}

func TestCookieAccessDoesNotClaimTheSignInPrompt(t *testing.T) {
	// This one mentions cookies too, but nothing was refused — the user simply
	// has none set up, and the remedy is the opposite: turn them on.
	output := `ERROR: [youtube] abc: Sign in to confirm you're not a bot. ` +
		`Use --cookies-from-browser or --cookies for the authentication.`

	got := ClassifyError(output, errors.New("exit status 1"))
	if got.Kind != ErrBotCheck {
		t.Errorf("Kind = %q, want %q", got.Kind, ErrBotCheck)
	}
}

// These use yt-dlp's verbatim output from real runs, as CLAUDE.md asks: the
// wordings that matter are the ones real runs produce.
func TestRealWordingsFromThisSession(t *testing.T) {
	cases := []struct {
		output string
		kind   ErrorKind
		says   string
	}{
		{
			"ERROR: [youtube] jNQXAC9IVRw: Sign in to confirm you’re not a bot. Use --cookies-from-browser or --cookies for the authentication. See  https://github.com/yt-dlp/yt-dlp/wiki/FAQ#how-do-i-pass-cookies-to-yt-dlp  for how to manually pass cookies. Also see  https://github.com/yt-dlp/yt-dlp/wiki/Extractors#exporting-youtube-cookies  for tips on effectively exporting YouTube cookies",
			ErrBotCheck, "isn't a bot",
		},
		{
			"ERROR: [generic] not-a-video: Unable to download webpage: HTTP Error 404: Not Found (caused by <HTTPError 404: Not Found>)",
			ErrNotFound, "doesn't exist",
		},
		{
			"ERROR: [youtube] jfKfPfyJRdk: This live stream recording is not available.",
			ErrUnavailable, "recording",
		},
		{
			"ERROR: [vimeo] 76979871: The web client only works when logged-in. Use --cookies, --cookies-from-browser, --username and --password, --netrc-cmd, or --netrc (vimeo) to provide account credentials. See  https://github.com/yt-dlp/yt-dlp/wiki/FAQ#how-do-i-pass-cookies-to-yt-dlp  for how to manually pass cookies",
			ErrLoginRequired, "signed in",
		},
	}
	for _, tc := range cases {
		got := ClassifyError(tc.output, errors.New("exit status 1"))
		if got.Kind != tc.kind || !strings.Contains(got.Message, tc.says) {
			t.Errorf("Kind %q Message %q\nwant kind %q saying %q, for:\n  %s", got.Kind, got.Message, tc.kind, tc.says, tc.output)
		}
	}
}

func TestA404IsNeverBlamedOnTheConnection(t *testing.T) {
	got := ClassifyError("ERROR: [generic] x: Unable to download webpage: HTTP Error 404: Not Found", errors.New("exit status 1"))
	if strings.Contains(strings.ToLower(got.Message), "internet") {
		t.Errorf("Message = %q; the site answered, so the connection is fine", got.Message)
	}
}

func TestClassifyCancellationBeforeOutput(t *testing.T) {
	// A killed yt-dlp leaves whatever it had written behind, which reads like a
	// failure. The context is the authority on what happened.
	got := ClassifyError("ERROR: Video unavailable", context.Canceled)
	if got.Kind != ErrCancelled {
		t.Errorf("Kind = %q, want %q", got.Kind, ErrCancelled)
	}
}

func TestClassifyKnownFailures(t *testing.T) {
	cases := []struct {
		name   string
		output string
		want   ErrorKind
	}{
		{"private", "ERROR: Private video. Sign in if you've been granted access", ErrPrivate},
		{"unavailable", "ERROR: Video unavailable", ErrUnavailable},
		{"geo-blocked", "ERROR: The uploader has not made this video available in your country", ErrGeoBlocked},
		{"rate-limited", "ERROR: HTTP Error 429: Too Many Requests", ErrRateLimited},
		{"network", "ERROR: Unable to download webpage: <urlopen error [Errno 8]>", ErrNetwork},
		{"disk-full", "ERROR: [Errno 28] No space left on device", ErrDiskFull},
		{"unsupported", "ERROR: Unsupported URL: https://example.com/page", ErrUnsupportedURL},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ClassifyError(c.output, errors.New("exit status 1")); got.Kind != c.want {
				t.Errorf("Kind = %q, want %q", got.Kind, c.want)
			}
		})
	}
}

func TestUnrecognisedFailureKeepsTheRawOutput(t *testing.T) {
	// The fallback message sends the user to the details toggle, so the detail
	// had better be there.
	got := ClassifyError("ERROR: something nobody has seen before", errors.New("exit status 1"))
	if got.Kind != ErrUnknown {
		t.Errorf("Kind = %q, want %q", got.Kind, ErrUnknown)
	}
	if got.Raw == "" {
		t.Error("an unrecognised failure with no raw output leaves the user nothing to read")
	}
}
