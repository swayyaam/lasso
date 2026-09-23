package binaries

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/swayyaam/lasso/packages/ghrelease"
)

// Lasso updates yt-dlp itself rather than calling `yt-dlp -U`.
//
// The bundled build is the PyInstaller onedir release, which yt-dlp's own
// updater refuses:
//
//	ERROR: Auto-update is not supported for unpackaged executables
//
// The onefile build does support -U, but it re-extracts a 37MB archive on
// every invocation — about 5.8 seconds per call against 0.16 once warm — which
// is why Lasso ships onedir. So the updater lives here: it fetches the same
// release asset the lock file pins, verifies its digest, and swaps the folder.

const (
	// ytDlpReleases is where yt-dlp's releases live. The updater never uses
	// GitHub's API: the latest-release page's redirect names the newest tag,
	// and that tag's files come from the download CDN — neither counts
	// against the API's 60-an-hour allowance. packages/ghrelease has the rest.
	ytDlpReleases = "https://github.com/yt-dlp/yt-dlp/releases"

	// sumsMaxAge is how long a release's checksum list is reused. A tag's
	// files never change once published, so this is generous.
	sumsMaxAge = 24 * time.Hour

	// ytDlpAsset is the release asset Lasso installs. It must stay the same
	// shape as binaries.lock.json pins, or an update would silently change the
	// layout out from under Manager.Path.
	ytDlpAsset = "yt-dlp_macos.zip"
	sumsAsset  = "SHA2-256SUMS"

	updateTimeout    = 15 * time.Minute
	maxDownloadBytes = 256 << 20
	maxChecksumBytes = 1 << 20
)

// UpdateResult describes the outcome of an "Update yt-dlp" request.
type UpdateResult struct {
	VersionBefore string `json:"versionBefore"`
	VersionAfter  string `json:"versionAfter"`
	Updated       bool   `json:"updated"`
	// Output is a human-readable log of what the updater did, for the details
	// toggle.
	Output string `json:"output"`
	// Fixups notes the repairs applied to the updated copy.
	Fixups []string `json:"fixups"`
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

// UpdateYtDlp installs the latest yt-dlp release into Application Support.
//
// The new copy is staged beside the old one and only swapped in after it has
// been verified to run, so a failed or interrupted update leaves the working
// installation untouched.
//
// releases is the process's shared GitHub client, so a refusal from GitHub
// reaches this updater and Lasso's own alike. nil builds a private one.
func (m *Manager) UpdateYtDlp(ctx context.Context, releases *ghrelease.Client) (UpdateResult, error) {
	if releases == nil {
		releases = ghrelease.New(ghrelease.Config{UserAgent: "Lasso"})
	}
	ctx, cancel := context.WithTimeout(ctx, updateTimeout)
	defer cancel()

	result := UpdateResult{}
	log := &strings.Builder{}

	before := m.verifyOne(ctx, YtDlp)
	if before.Err != nil {
		return result, before.Err
	}
	result.VersionBefore = before.Version
	fmt.Fprintf(log, "Installed version: %s\n", before.Version)

	// A deliberate press asks now; the shared client still spaces presses
	// and honours any refusal, so leaning on the button costs nothing extra.
	tag, err := releases.LatestTag(ctx, m.releasesURL, 0)
	if err != nil {
		return result, err
	}
	fmt.Fprintf(log, "Latest release: %s\n", tag)

	if tag == before.Version {
		result.VersionAfter = before.Version
		result.Output = log.String() + "Already up to date.\n"
		return result, nil
	}

	// Addresses are built from the tag rather than read from anywhere, so
	// nothing GitHub sends back can point the download somewhere else.
	base := strings.TrimSuffix(m.releasesURL, "/") + "/download/" + tag + "/"

	staging, err := os.MkdirTemp(m.paths.Bin, ".yt-dlp-update-")
	if err != nil {
		return result, fmt.Errorf("could not prepare the update: %w", err)
	}
	defer os.RemoveAll(staging)

	archive := filepath.Join(staging, ytDlpAsset)
	fmt.Fprintf(log, "Downloading %s\n", ytDlpAsset)
	sums, err := releases.Get(ctx, base+sumsAsset, sumsMaxAge)
	if errors.Is(err, ghrelease.ErrNotFound) {
		return result, fmt.Errorf("yt-dlp %s publishes no checksum file, so its download cannot be verified", tag)
	}
	if err != nil {
		return result, err
	}
	signature, err := releases.Get(ctx, base+sumsSigAsset, sumsMaxAge)
	if errors.Is(err, ghrelease.ErrNotFound) {
		return result, fmt.Errorf("yt-dlp %s publishes no signature for its checksums, so it cannot be trusted as yt-dlp's", tag)
	}
	if err != nil {
		return result, err
	}
	if err := verifySums(m.signingKey, m.signingFingerprint, sums, signature); err != nil {
		// Not "try again": this is either tampering or a new signing key, and
		// either way the answer is a Lasso that knows about it.
		return result, fmt.Errorf("yt-dlp %s is not signed by yt-dlp's release key, so it was not installed. If yt-dlp has changed its key, updating Lasso will bring the new one: %w", tag, err)
	}
	fmt.Fprintf(log, "Signature verified: yt-dlp's release key %s\n", m.signingFingerprint[len(m.signingFingerprint)-16:])

	want, err := digestFor(sums, ytDlpAsset)
	if err != nil {
		return result, err
	}

	if err := releases.Download(ctx, base+ytDlpAsset, archive, maxDownloadBytes); err != nil {
		if errors.Is(err, ghrelease.ErrNotFound) {
			return result, fmt.Errorf("yt-dlp %s has no %s", tag, ytDlpAsset)
		}
		return result, err
	}
	got, err := fileDigest(archive)
	if err != nil {
		return result, err
	}
	if got != want {
		// Never install something whose bytes were not the published ones.
		return result, fmt.Errorf("the downloaded update did not match its published checksum, so it was discarded")
	}
	fmt.Fprintf(log, "Checksum verified: %s\n", got[:16]+"…")

	unpacked := filepath.Join(staging, "unpacked")
	if err := unzip(ctx, archive, unpacked); err != nil {
		return result, err
	}

	spec := m.manifest.Binaries[YtDlp]
	if _, err := os.Stat(filepath.Join(unpacked, spec.Entrypoint)); err != nil {
		return result, fmt.Errorf("the update did not contain %s", spec.Entrypoint)
	}

	fixes, err := m.fixupTree(ctx, unpacked, spec.Entrypoint)
	if err != nil {
		return result, err
	}
	result.Fixups = fixes
	for _, fix := range fixes {
		fmt.Fprintf(log, "Repaired: %s\n", fix)
	}

	// Prove the staged copy runs before anything is swapped.
	staged := filepath.Join(unpacked, spec.Entrypoint)
	version, err := runVersion(ctx, staged, m.Environ())
	if err != nil {
		return result, fmt.Errorf("the updated yt-dlp would not run, so the existing one was kept: %w", err)
	}
	fmt.Fprintf(log, "Staged copy runs: %s\n", version)

	target := filepath.Join(m.paths.Bin, string(YtDlp))
	if err := swapDir(unpacked, target); err != nil {
		return result, err
	}
	fmt.Fprintf(log, "Installed into %s\n", target)

	after := m.verifyOne(ctx, YtDlp)
	if after.Err != nil {
		return result, after.Err
	}
	result.VersionAfter = after.Version
	result.Updated = after.Version != before.Version

	// Record what is actually installed, so the next launch does not treat the
	// newer copy as stale and overwrite it with the bundled one.
	stamp := readStamp(m.paths.Bin)
	stamp.Binaries[YtDlp] = after.Version
	if err := writeStamp(m.paths.Bin, stamp); err != nil {
		return result, err
	}

	result.Output = log.String()
	return result, nil
}

// digestFor pulls one file's published digest out of a checksum list, in
// shasum's format: digest, whitespace, filename.
func digestFor(sums []byte, name string) (string, error) {
	for _, line := range strings.Split(string(sums), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == name {
			return strings.ToLower(fields[0]), nil
		}
	}
	return "", fmt.Errorf("the release's checksum file does not list %s", name)
}

func fileDigest(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	sum := sha256.New()
	if _, err := io.Copy(sum, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(sum.Sum(nil)), nil
}

// unzip unpacks an archive using the system tool, which handles the symlinks
// inside yt-dlp's bundled Python framework correctly.
func unzip(ctx context.Context, archive, dest string) error {
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	out, err := exec.CommandContext(ctx, "/usr/bin/unzip", "-qq", "-o", archive, "-d", dest).CombinedOutput()
	if err != nil {
		return fmt.Errorf("could not unpack the update: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// swapDir replaces target with staged, keeping the old copy until the new one
// is in place so a failure mid-swap can be undone.
func swapDir(staged, target string) error {
	backup := target + ".previous"
	_ = os.RemoveAll(backup)

	if _, err := os.Stat(target); err == nil {
		if err := os.Rename(target, backup); err != nil {
			return fmt.Errorf("could not move the old yt-dlp aside: %w", err)
		}
	}

	if err := os.Rename(staged, target); err != nil {
		// Put the working copy back rather than leaving nothing installed.
		_ = os.Rename(backup, target)
		return fmt.Errorf("could not install the update: %w", err)
	}

	_ = os.RemoveAll(backup)
	return nil
}

// runVersion executes a binary's version command and returns its first line.
func runVersion(ctx context.Context, path string, env []string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, verifyTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, path, "--version")
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return firstLine(string(out)), nil
}
