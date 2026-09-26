package main

import (
	"encoding/json"
	"errors"

	"github.com/swayyaam/lasso/packages/core"
)

// formatError is how an error from a bound method reaches the page.
//
// A yt-dlp failure crosses whole — {kind, message, raw} — so the page can act
// on what went wrong rather than read it back out of a sentence: Choose offers
// the doctor for a bot check, and not for a typo.
//
// It crosses as JSON text, not as an object. Wails' runtime rebuilds every
// rejection as new Error(value), which keeps a string as the message and
// flattens an object to "[object Object]" (seen in the running app). The page
// reads it back with failureOf. Every other error crosses as its text, exactly
// as Wails sends it without a formatter.
func formatError(err error) any {
	var failure *core.DownloadError
	if errors.As(err, &failure) {
		if raw, jerr := json.Marshal(failure); jerr == nil {
			return string(raw)
		}
	}
	return err.Error()
}
