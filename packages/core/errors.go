package core

import (
	"context"
	"errors"
	"strings"
)

// ErrorKind is a coarse category of download failure, chosen so the UI can say
// something useful without the user reading a stack of yt-dlp output.
type ErrorKind string

const (
	ErrUnknown        ErrorKind = "unknown"
	ErrUnavailable    ErrorKind = "unavailable"
	ErrPrivate        ErrorKind = "private"
	ErrLoginRequired  ErrorKind = "login-required"
	ErrAgeRestricted  ErrorKind = "age-restricted"
	ErrGeoBlocked     ErrorKind = "geo-blocked"
	ErrUnsupportedURL ErrorKind = "unsupported-url"
	ErrNetwork        ErrorKind = "network"
	ErrRateLimited    ErrorKind = "rate-limited"
	ErrDiskFull       ErrorKind = "disk-full"
	ErrCancelled      ErrorKind = "cancelled"
	ErrPostProcess    ErrorKind = "post-processing"
	ErrCookieAccess   ErrorKind = "cookie-access"
	// ErrBotCheck is YouTube doubting the network, not the video: it asks
	// visitors from an address it distrusts to prove they are not a bot.
	ErrBotCheck ErrorKind = "bot-check"
	// ErrNotFound is a page that is not there. The site answered, which is
	// exactly what makes "check your internet connection" the wrong advice.
	ErrNotFound ErrorKind = "not-found"
	// ErrRefused is a site answering 403.
	ErrRefused ErrorKind = "refused"
	// ErrSiteChanged is yt-dlp failing in a way it did not expect, which is
	// almost always a site having changed under it. A newer yt-dlp is the fix.
	ErrSiteChanged ErrorKind = "site-changed"
)

// DownloadError is a yt-dlp failure translated into something a person can act
// on, with the original output kept for the details toggle.
type DownloadError struct {
	Kind    ErrorKind
	Message string
	// Raw is yt-dlp's own output, shown behind "details".
	Raw string
	Err error
}

func (e *DownloadError) Error() string {
	if e.Raw != "" {
		return e.Message + ": " + e.Raw
	}
	return e.Message
}

func (e *DownloadError) Unwrap() error { return e.Err }

// UserMessage is the plain-language sentence for the UI.
func (e *DownloadError) UserMessage() string { return e.Message }

// classifier maps a signature in yt-dlp's output to a kind and a message.
//
// Order matters: the first match wins, so narrower signatures come first.
// "Private video" contains "video" and would be caught by broader rules below.
var classifiers = []struct {
	kind     ErrorKind
	message  string
	patterns []string
}{
	{
		kind:     ErrPrivate,
		message:  "This video is private.",
		patterns: []string{"private video", "this video is private"},
	},
	{
		kind:    ErrAgeRestricted,
		message: "This video is age-restricted. Turn on cookies from your browser in Settings, and make sure you are signed in there.",
		patterns: []string{
			"age-restricted", "age restricted", "confirm your age",
			"inappropriate for some users",
		},
	},
	{
		// Before sign-in, whose "sign in to confirm" and "--cookies" it also
		// contains. It was reported as "this video requires you to be signed
		// in", which sent people to look for a private video that was public.
		kind:     ErrBotCheck,
		message:  "YouTube wants to confirm this network isn't a bot. Cookies from a browser where you're signed in to YouTube usually get past it; if they're on already, wait a while and try again.",
		patterns: []string{"not a bot"},
	},
	{
		kind:    ErrLoginRequired,
		message: "This video requires you to be signed in. Turn on cookies from your browser in Settings.",
		patterns: []string{
			"sign in to confirm", "login required", "requires authentication",
			"use --cookies", "cookies-from-browser", "members-only", "join this channel",
		},
	},
	{
		kind:    ErrGeoBlocked,
		message: "This video is not available in your country.",
		patterns: []string{
			"not available in your country", "geo restricted", "geo-restricted",
			"blocked it in your country", "unavailable in your location",
			// YouTube's own wording, which the phrase above does not cover:
			// "has not made this video available in your country".
			"made this video available in your country",
			"not available from your location",
		},
	},
	{
		kind:     ErrUnavailable,
		message:  "This live stream hasn't started yet.",
		patterns: []string{"live event will begin", "premieres in", "waiting for scheduled stream"},
	},
	{
		kind:     ErrUnavailable,
		message:  "This live stream's recording isn't available.",
		patterns: []string{"live stream recording is not available", "recording is not available"},
	},
	{
		kind:    ErrUnavailable,
		message: "This video is no longer available.",
		patterns: []string{
			"video unavailable", "this video is unavailable", "has been removed",
			"account associated with this video has been terminated",
			"video has been removed", "no longer available", "removed by the uploader",
			"this video does not exist",
		},
	},
	{
		// yt-dlp appends this to every extractor error it did not expect
		// (bug_reports_message in yt_dlp/utils/_utils.py), and never to the
		// expected ones: a private video, a network failure, a 404. So it
		// marks the failures a newer yt-dlp fixes. Ahead of the unsupported
		// link rule: yt-dlp's usual "a site changed" error begins "Unable to
		// extract", and was being reported as a link Lasso cannot handle.
		kind:     ErrSiteChanged,
		message:  "The site has changed in a way this version of yt-dlp does not understand yet. Updating yt-dlp usually fixes it.",
		patterns: []string{"please report this issue on"},
	},
	{
		kind:    ErrUnsupportedURL,
		message: "Lasso does not know how to download from that link.",
		patterns: []string{
			"unsupported url", "is not a valid url", "no suitable extractor",
			"unable to extract", "does not pass filter",
		},
	},
	{
		// Before network, whose "unable to download webpage" is the start of
		// this very message. A 404 was reported as "check your internet
		// connection" while another download ran at 4.5 MB/s beside it.
		kind:    ErrNotFound,
		message: "That page doesn't exist. The site answered, but there's nothing at that address, so check the link.",
		patterns: []string{
			"http error 404", "http error 410", "404: not found", "410: gone",
			// YouTube answering for a playlist ID that is not there.
			"the playlist does not exist",
		},
	},
	{
		kind:     ErrRefused,
		message:  "The site refused the request. On YouTube that's usually temporary, so try again in a few minutes; elsewhere, cookies from a browser where you're signed in often help.",
		patterns: []string{"http error 403", "403: forbidden"},
	},
	{
		kind:     ErrRateLimited,
		message:  "The site is asking us to slow down. Wait a few minutes and try again.",
		patterns: []string{"http error 429", "too many requests", "rate-limit", "rate limit"},
	},
	{
		kind:    ErrNetwork,
		message: "Could not reach the site. Check your internet connection and try again.",
		patterns: []string{
			"unable to download webpage", "temporary failure in name resolution",
			"connection refused", "connection reset", "network is unreachable",
			"failed to resolve", "timed out", "timeout", "urlopen error",
			"getaddrinfo", "ssl:", "certificate verify failed",
		},
	},
	{
		kind:     ErrDiskFull,
		message:  "There is not enough space left on the disk.",
		patterns: []string{"no space left on device", "disk quota exceeded"},
	},
	{
		kind:     ErrPostProcess,
		message:  "The download finished but converting the file failed.",
		patterns: []string{"postprocessing:", "ffmpeg exited with", "error opening output file"},
	},
}

// ClassifyError turns yt-dlp's output into a DownloadError.
//
// Cancellation is checked first: a cancelled download produces whatever output
// yt-dlp had managed to write, which would otherwise be misread as a failure.
func ClassifyError(output string, err error) *DownloadError {
	raw := strings.TrimSpace(output)

	if err != nil && (errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)) {
		kind, message := ErrCancelled, "Download cancelled."
		if errors.Is(err, context.DeadlineExceeded) {
			kind, message = ErrNetwork, "The site took too long to respond. Try again."
		}
		return &DownloadError{Kind: kind, Message: message, Raw: raw, Err: err}
	}

	haystack := strings.ToLower(raw)

	// Checked ahead of the table because it needs two signatures at once, and
	// because either one alone belongs to a different rule: "operation not
	// permitted" on its own is a protected download folder, and the mention of
	// --cookies-from-browser would otherwise be read as "you need to sign in" —
	// advice that goes nowhere when cookies were asked for and could not be read.
	if message, ok := cookieAccessFailure(haystack); ok {
		return &DownloadError{Kind: ErrCookieAccess, Message: message, Raw: raw, Err: err}
	}

	for _, c := range classifiers {
		for _, pattern := range c.patterns {
			if strings.Contains(haystack, pattern) {
				return &DownloadError{Kind: c.kind, Message: c.message, Raw: raw, Err: err}
			}
		}
	}

	return &DownloadError{
		Kind: ErrUnknown,
		// Not "the download failed": this is also what a link that would not
		// resolve says, before any download was asked for.
		Message: "yt-dlp could not do that. Open details to see what it reported.",
		Raw:     raw,
		Err:     err,
	}
}

// A cookie jar can fail in two ways that want opposite advice, so they get
// separate messages under the same kind.
const (
	// cookieRefusedMessage is for a jar that exists and cannot be opened.
	// Safari's always needs Full Disk Access: macOS keeps it inside Safari's
	// container, which is protected whatever the file permissions say.
	cookieRefusedMessage = "macOS would not let Lasso read your browser's cookies. " +
		"Give Lasso Full Disk Access in System Settings › Privacy & Security, then try again — " +
		"or choose a different browser in Settings."

	// cookieMissingMessage is for a jar that is not there as far as Lasso can
	// tell, which has two causes that look identical from here.
	//
	// The obvious one is that the browser is not installed. The other is that
	// macOS is hiding it: a protected location reports "no such file" to a
	// process without permission rather than "permission denied", so an
	// installed browser whose data Lasso may not read is indistinguishable
	// from one that was never installed. Naming only the first sends someone
	// with the second to look for a browser that is already there.
	//
	// The doctor can tell them apart, because it can look for the application
	// itself. This message cannot, so it says both.
	cookieMissingMessage = "Lasso could not find that browser's cookies. Either it is not " +
		"installed, or macOS is hiding its data from Lasso — Full Disk Access in System " +
		"Settings covers the second. Run the doctor to find out which."
)

// cookieRefusals are the ways the operating system, or yt-dlp, says a cookie
// jar exists but will not open.
var cookieRefusals = []string{
	"operation not permitted", "permission denied", "could not copy",
	"failed to decrypt", "unable to decrypt", "unable to read",
}

// cookieMissing are the ways it says there is no jar to open.
var cookieMissing = []string{
	"could not find", "does not exist", "no such file or directory", "not found",
}

// cookieAccessFailure reports whether output describes a cookie jar that could
// not be read, and which of the two remedies applies.
//
// Both halves are required: the output must be about cookies *and* describe a
// failure to get at them. A download folder Lasso cannot write to produces the
// same "operation not permitted", and sending that user to Full Disk Access for
// their cookies would be worse than saying nothing.
//
// Refusal is checked before absence because a jar that is present but locked
// often reports both, and the permission problem is the actionable one.
func cookieAccessFailure(haystack string) (string, bool) {
	if !strings.Contains(haystack, "cookie") {
		return "", false
	}
	for _, refusal := range cookieRefusals {
		if strings.Contains(haystack, refusal) {
			return cookieRefusedMessage, true
		}
	}
	for _, missing := range cookieMissing {
		if strings.Contains(haystack, missing) {
			return cookieMissingMessage, true
		}
	}
	return "", false
}

// UserMessage returns a plain-language message for any error.
func UserMessage(err error) string {
	if err == nil {
		return ""
	}
	var d *DownloadError
	if errors.As(err, &d) {
		return d.UserMessage()
	}
	return err.Error()
}

// subtitleFailurePatterns are yt-dlp's wordings for "the video is fine, the
// subtitles are not".
var subtitleFailurePatterns = []string{
	"unable to download video subtitles",
	"unable to download subtitles",
	"error downloading subtitles",
	"unable to extract subtitles",
}

// IsSubtitleFailure reports whether output describes a subtitle download that
// failed, as opposed to a failure of the video itself.
//
// This is deliberately separate from ErrorKind. The classifier answers "what
// should the user be told", and for a 429 on the subtitle endpoint that is
// still the rate-limit message. This answers a different question — "can the
// download be salvaged by dropping subtitles" — and the two want different
// precedence, so they stay apart.
func IsSubtitleFailure(output string) bool {
	haystack := strings.ToLower(output)
	for _, pattern := range subtitleFailurePatterns {
		if strings.Contains(haystack, pattern) {
			return true
		}
	}
	return false
}
