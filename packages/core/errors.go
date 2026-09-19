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
		kind:    ErrLoginRequired,
		message: "This video requires you to be signed in. Turn on cookies from your browser in Settings.",
		patterns: []string{
			"sign in to confirm", "login required", "requires authentication",
			"use --cookies", "cookies-from-browser", "members-only", "join this channel",
			"not a bot",
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
		kind:    ErrUnsupportedURL,
		message: "Lasso does not know how to download from that link.",
		patterns: []string{
			"unsupported url", "is not a valid url", "no suitable extractor",
			"unable to extract", "does not pass filter",
		},
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
	if isCookieAccessFailure(haystack) {
		return &DownloadError{Kind: ErrCookieAccess, Message: cookieAccessMessage, Raw: raw, Err: err}
	}

	for _, c := range classifiers {
		for _, pattern := range c.patterns {
			if strings.Contains(haystack, pattern) {
				return &DownloadError{Kind: c.kind, Message: c.message, Raw: raw, Err: err}
			}
		}
	}

	return &DownloadError{
		Kind:    ErrUnknown,
		Message: "The download failed. Open details to see what yt-dlp reported.",
		Raw:     raw,
		Err:     err,
	}
}

// cookieAccessMessage is the remedy for a cookie jar Lasso is not allowed to
// read. Safari's always needs Full Disk Access — macOS keeps it inside Safari's
// container, which is protected whatever the file permissions say.
const cookieAccessMessage = "macOS would not let Lasso read your browser's cookies. " +
	"Give Lasso Full Disk Access in System Settings › Privacy & Security, then try again — " +
	"or choose a different browser in Settings."

// cookieRefusals are the ways the operating system, or yt-dlp, says a cookie
// jar could not be opened or decrypted.
var cookieRefusals = []string{
	"operation not permitted", "permission denied", "could not copy",
	"failed to decrypt", "unable to decrypt", "could not find cookie",
	"unable to read", "no such file or directory",
}

// isCookieAccessFailure reports whether output describes a cookie jar that
// could not be read, as opposed to any other permission problem.
//
// Both halves are required. A download folder Lasso cannot write to produces
// the same "operation not permitted", and sending that user to Full Disk Access
// for their cookies would be worse than saying nothing.
func isCookieAccessFailure(haystack string) bool {
	mentionsCookies := strings.Contains(haystack, "cookie")
	if !mentionsCookies {
		return false
	}
	for _, refusal := range cookieRefusals {
		if strings.Contains(haystack, refusal) {
			return true
		}
	}
	return false
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
