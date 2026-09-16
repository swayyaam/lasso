package binaries

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

// updateTimeout bounds `yt-dlp -U`, which downloads a new release over the
// network before swapping itself out.
const updateTimeout = 10 * time.Minute

// UpdateResult describes the outcome of an "Update yt-dlp" request.
type UpdateResult struct {
	// VersionBefore and VersionAfter are what `--version` reported either side
	// of the update. They are equal when yt-dlp was already current.
	VersionBefore string
	VersionAfter  string
	// Updated is true when the version actually changed.
	Updated bool
	// Output is yt-dlp's own raw output, for the details toggle.
	Output string
	// Fixups notes any repairs applied to the updated copy.
	Fixups []string
}

// UpdateYtDlp runs `yt-dlp -U` against the copy in Application Support.
//
// This works because Lasso executes yt-dlp from Application Support rather than
// from inside the .app bundle: the bundle is read-only, so a self-update there
// would fail. Afterwards the same fixups as a fresh install are re-applied —
// yt-dlp replaces its own files, and the replacements need the executable bit,
// no quarantine flag, and a signature macOS accepts.
func (m *Manager) UpdateYtDlp(ctx context.Context) (UpdateResult, error) {
	result := UpdateResult{}
	path := m.Path(YtDlp)

	before := m.verifyOne(ctx, YtDlp)
	if before.Err != nil {
		return result, before.Err
	}
	result.VersionBefore = before.Version

	runCtx, cancel := context.WithTimeout(ctx, updateTimeout)
	defer cancel()

	cmd := exec.CommandContext(runCtx, path, "-U")
	cmd.Env = m.Environ()
	out, err := cmd.CombinedOutput()
	result.Output = strings.TrimSpace(string(out))
	if err != nil {
		return result, &VerifyError{Name: YtDlp, Path: path, Output: result.Output, Err: err}
	}

	fixes, err := m.fixup(ctx, path, m.manifest.Binaries[YtDlp])
	if err != nil {
		return result, err
	}
	result.Fixups = fixes

	after := m.verifyOne(ctx, YtDlp)
	if after.Err != nil {
		return result, after.Err
	}
	result.VersionAfter = after.Version
	result.Updated = result.VersionAfter != result.VersionBefore

	return result, nil
}

// UserMessage summarises an update for the UI.
func (r UpdateResult) UserMessage() string {
	switch {
	case r.Updated:
		return "Updated yt-dlp to " + r.VersionAfter + "."
	case r.VersionAfter != "":
		return "yt-dlp is already up to date (" + r.VersionAfter + ")."
	default:
		return "yt-dlp is up to date."
	}
}
