package binaries

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
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
	// releaseAPI is the only endpoint this updater talks to.
	releaseAPI = "https://api.github.com/repos/yt-dlp/yt-dlp/releases/latest"

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

// client returns the HTTP client to use, defaulting when unset.
func (m *Manager) client() *http.Client {
	if m.http != nil {
		return m.http
	}
	return http.DefaultClient
}

type releaseInfo struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	} `json:"assets"`
}

func (r releaseInfo) asset(name string) (string, bool) {
	for _, a := range r.Assets {
		if a.Name == name {
			return a.URL, true
		}
	}
	return "", false
}

// UpdateYtDlp installs the latest yt-dlp release into Application Support.
//
// The new copy is staged beside the old one and only swapped in after it has
// been verified to run, so a failed or interrupted update leaves the working
// installation untouched.
func (m *Manager) UpdateYtDlp(ctx context.Context) (UpdateResult, error) {
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

	release, err := m.latestRelease(ctx)
	if err != nil {
		return result, err
	}
	fmt.Fprintf(log, "Latest release: %s\n", release.TagName)

	if release.TagName == before.Version {
		result.VersionAfter = before.Version
		result.Output = log.String() + "Already up to date.\n"
		return result, nil
	}

	zipURL, ok := release.asset(ytDlpAsset)
	if !ok {
		return result, fmt.Errorf("the latest yt-dlp release has no %s", ytDlpAsset)
	}
	sumsURL, ok := release.asset(sumsAsset)
	if !ok {
		return result, fmt.Errorf("the latest yt-dlp release has no checksum file")
	}

	staging, err := os.MkdirTemp(m.paths.Bin, ".yt-dlp-update-")
	if err != nil {
		return result, fmt.Errorf("could not prepare the update: %w", err)
	}
	defer os.RemoveAll(staging)

	archive := filepath.Join(staging, ytDlpAsset)
	fmt.Fprintf(log, "Downloading %s\n", ytDlpAsset)
	if err := m.download(ctx, zipURL, archive, maxDownloadBytes); err != nil {
		return result, err
	}

	want, err := m.expectedDigest(ctx, sumsURL, ytDlpAsset)
	if err != nil {
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

// latestRelease asks GitHub what the newest yt-dlp release is.
func (m *Manager) latestRelease(ctx context.Context) (releaseInfo, error) {
	var info releaseInfo

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.updateAPI, nil)
	if err != nil {
		return info, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := m.client().Do(req)
	if err != nil {
		return info, fmt.Errorf("could not reach the yt-dlp release server: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return info, fmt.Errorf("the yt-dlp release server returned %s", resp.Status)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxChecksumBytes)).Decode(&info); err != nil {
		return info, fmt.Errorf("could not read the release information: %w", err)
	}
	if info.TagName == "" {
		return info, fmt.Errorf("the release server did not name a version")
	}
	return info, nil
}

// expectedDigest pulls one file's published digest out of the release's
// checksum list.
func (m *Manager) expectedDigest(ctx context.Context, sumsURL, name string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sumsURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := m.client().Do(req)
	if err != nil {
		return "", fmt.Errorf("could not fetch the update's checksums: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("the checksum file returned %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxChecksumBytes))
	if err != nil {
		return "", err
	}

	for _, line := range strings.Split(string(body), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == name {
			return strings.ToLower(fields[0]), nil
		}
	}
	return "", fmt.Errorf("the release's checksum file does not list %s", name)
}

// download fetches a URL to a file, refusing anything over the size limit.
func (m *Manager) download(ctx context.Context, url, dest string, limit int64) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := m.client().Do(req)
	if err != nil {
		return fmt.Errorf("could not download the update: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("downloading the update returned %s", resp.Status)
	}

	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, io.LimitReader(resp.Body, limit)); err != nil {
		return fmt.Errorf("could not save the update: %w", err)
	}
	return out.Sync()
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
