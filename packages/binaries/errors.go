package binaries

import (
	"errors"
	"fmt"
	"strings"
)

// SourceMissingError means the bundled sidecar directory could not be found at
// all — the app was built without binaries, or `make setup` was never run.
type SourceMissingError struct {
	Tried []string
}

func (e *SourceMissingError) Error() string {
	var b strings.Builder
	b.WriteString("Lasso could not find its bundled helper programs (yt-dlp, ffmpeg, ffprobe, deno).")
	if len(e.Tried) > 0 {
		b.WriteString("\n\nLooked in:")
		for _, p := range e.Tried {
			b.WriteString("\n  " + p)
		}
	}
	b.WriteString("\n\nIf you are running from source, run `make setup`.")
	return b.String()
}

// UserMessage is the plain-language sentence to show in the UI.
func (e *SourceMissingError) UserMessage() string {
	return "Lasso could not find its bundled helper programs. If you are running from source, run `make setup`."
}

// NotInstalledError means a binary is missing from the install directory.
type NotInstalledError struct {
	Name Name
	Path string
}

func (e *NotInstalledError) Error() string {
	return fmt.Sprintf("%s is not installed at %s", e.Name, e.Path)
}

func (e *NotInstalledError) UserMessage() string {
	return fmt.Sprintf("%s is missing. Quit and reopen Lasso to reinstall it.", e.Name)
}

// VerifyError means a binary is present but would not run.
type VerifyError struct {
	Name   Name
	Path   string
	Output string
	Err    error
}

func (e *VerifyError) Error() string {
	msg := fmt.Sprintf("%s at %s did not run: %v", e.Name, e.Path, e.Err)
	if e.Output != "" {
		msg += "\n" + strings.TrimSpace(e.Output)
	}
	return msg
}

func (e *VerifyError) Unwrap() error { return e.Err }

// UserMessage turns the common failure modes into something a non-technical
// user can act on.
func (e *VerifyError) UserMessage() string {
	out := strings.ToLower(e.Output + " " + errText(e.Err))
	switch {
	case strings.Contains(out, "bad cpu type"):
		return fmt.Sprintf("%s was built for a different kind of Mac. Reinstall Lasso for this machine.", e.Name)
	case strings.Contains(out, "killed") || strings.Contains(out, "code signature") || strings.Contains(out, "signal: killed"):
		return fmt.Sprintf("macOS blocked %s from running. Reinstalling Lasso usually fixes this.", e.Name)
	case strings.Contains(out, "permission denied"):
		return fmt.Sprintf("%s is not allowed to run. Reinstalling Lasso usually fixes this.", e.Name)
	case strings.Contains(out, "timed out"):
		return fmt.Sprintf("%s did not respond. Try reopening Lasso.", e.Name)
	default:
		return fmt.Sprintf("%s could not be started. Reinstalling Lasso usually fixes this.", e.Name)
	}
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// UserFacing is implemented by errors that carry a plain-language message.
type UserFacing interface {
	error
	UserMessage() string
}

// UserMessage returns a plain-language message for any error, falling back to
// the error text when it carries none.
func UserMessage(err error) string {
	if err == nil {
		return ""
	}
	var uf UserFacing
	if errors.As(err, &uf) {
		return uf.UserMessage()
	}
	return err.Error()
}
