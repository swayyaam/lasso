// Package updater replaces Lasso with a newer release of itself.
//
// It exists so that updating does not mean downloading a 150 MB disk image and
// dragging it over the old copy. That matters more than convenience here:
// Lasso is not notarised, so a freshly downloaded copy is quarantined and has
// to be let past Gatekeeper by hand every single time. An update the app
// installs itself is never quarantined, so it simply works.
//
// The shape follows packages/binaries, which already updates yt-dlp: read the
// release, verify a published digest, stage the new copy beside the old one,
// prove it is sound, and only then swap. A failed or interrupted update leaves
// the working copy exactly where it was.
//
// What it downloads is the app *without* its helper programs. They are 328 MB
// of the 329 MB bundle and they rarely change, so they are carried across from
// the copy already installed — which turns a 150 MB download into about 15 MB.
// When a release does change them, the manifests disagree and the update is
// refused with an explanation rather than quietly installing helpers the user
// did not get.
package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultReleaseAPI is the only endpoint this talks to.
	DefaultReleaseAPI = "https://api.github.com/repos/swayyaam/lasso/releases/latest"

	// APIEnv overrides that endpoint. It exists so the whole update — download,
	// verify, carry the helpers across, reseal, swap — can be exercised against
	// a local server on a real bundle, rather than only ever being tried for
	// the first time by someone's actual installation.
	//
	// Same idea as LASSO_BIN_SOURCE and LASSO_SUPPORT_DIR in packages/binaries.
	APIEnv = "LASSO_UPDATE_API"

	// AppAsset is the app on its own, without Contents/Resources/bin.
	AppAsset = "Lasso-app.zip"
	// SumsAsset lists the SHA-256 of every asset in the release.
	SumsAsset = "SHA256SUMS"

	checkTimeout    = 30 * time.Second
	installTimeout  = 15 * time.Minute
	maxDownloadSize = 128 << 20
	maxSumsSize     = 1 << 20
)

// ErrHelpersChanged means the release ships different helper programs, so the
// small update cannot carry the installed ones across.
var ErrHelpersChanged = errors.New("this release updates the helper programs too")

// Update describes a release that is newer than the running app.
type Update struct {
	// Version is the release's version, without the leading v.
	Version string `json:"version"`
	// Notes is the release body, shown before installing.
	Notes string `json:"notes"`
	// PageURL is the release page, for anyone who would rather do it by hand.
	PageURL string `json:"pageUrl"`
	// Available is false when the running app is already current.
	Available bool `json:"available"`
	// Bytes is the download size, so the UI can say what it is about to fetch.
	Bytes int64 `json:"bytes"`
}

// Result describes a completed install.
type Result struct {
	// Version now on disk.
	Version string `json:"version"`
	// Installed is false when there was nothing to do.
	Installed bool `json:"installed"`
	// NeedsRestart is true once the swap is done: the running process is still
	// the old build until it is replaced.
	NeedsRestart bool `json:"needsRestart"`
	// Output is a log of what happened, for the details toggle.
	Output string `json:"output"`
}

// Runner executes a command and returns its combined output.
//
// Injected so the tests can exercise the whole install without ditto, codesign
// or xattr — and so every external command this package runs is visible in one
// place rather than scattered through it.
type Runner func(ctx context.Context, name string, args ...string) (string, error)

// Config is what the updater needs to do its job.
type Config struct {
	// BundlePath is the .app to replace.
	BundlePath string
	// CurrentVersion is what is running, without the leading v.
	CurrentVersion string
	// ReleaseAPI defaults to DefaultReleaseAPI. Tests point it at a local server.
	ReleaseAPI string
	// HTTP defaults to a client with a sensible timeout.
	HTTP *http.Client
	// Run defaults to executing the command for real.
	Run Runner
}

// Updater checks for and installs new releases.
type Updater struct {
	cfg Config
	log []string
}

// New builds an Updater.
func New(cfg Config) (*Updater, error) {
	if cfg.BundlePath == "" {
		return nil, fmt.Errorf("updater needs the path of the app to replace")
	}
	if cfg.CurrentVersion == "" {
		return nil, fmt.Errorf("updater needs to know which version is running")
	}
	if cfg.ReleaseAPI == "" {
		cfg.ReleaseAPI = DefaultReleaseAPI
	}
	if cfg.HTTP == nil {
		cfg.HTTP = &http.Client{Timeout: installTimeout}
	}
	if cfg.Run == nil {
		cfg.Run = execRun
	}
	return &Updater{cfg: cfg}, nil
}

type releaseInfo struct {
	TagName string `json:"tag_name"`
	Body    string `json:"body"`
	HTMLURL string `json:"html_url"`
	Draft   bool   `json:"draft"`
	Assets  []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
		Size int64  `json:"size"`
	} `json:"assets"`
}

func (r releaseInfo) asset(name string) (url string, size int64, ok bool) {
	for _, a := range r.Assets {
		if a.Name == name {
			return a.URL, a.Size, true
		}
	}
	return "", 0, false
}

// Check asks whether a newer release exists.
//
// It reads and reports; nothing is downloaded and nothing on disk is touched,
// so it is safe to call on a timer or whenever a window opens.
func (u *Updater) Check(ctx context.Context) (Update, error) {
	ctx, cancel := context.WithTimeout(ctx, checkTimeout)
	defer cancel()

	release, err := u.latest(ctx)
	if err != nil {
		return Update{}, err
	}

	version := strings.TrimPrefix(release.TagName, "v")
	update := Update{Version: version, Notes: release.Body, PageURL: release.HTMLURL}

	if !IsNewer(u.cfg.CurrentVersion, version) {
		return update, nil
	}
	// A release with no app asset cannot be installed from here, so it is not
	// offered as one — saying an update is available and then failing to
	// install it would be worse than staying quiet.
	_, size, ok := release.asset(AppAsset)
	if !ok {
		return update, nil
	}

	update.Available = true
	update.Bytes = size
	return update, nil
}

func (u *Updater) latest(ctx context.Context) (releaseInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.cfg.ReleaseAPI, nil)
	if err != nil {
		return releaseInfo{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	// GitHub asks every caller to identify itself, and it makes Lasso's
	// traffic legible in their logs rather than anonymous Go.
	req.Header.Set("User-Agent", "Lasso/"+u.cfg.CurrentVersion)

	resp, err := u.cfg.HTTP.Do(req)
	if err != nil {
		return releaseInfo{}, fmt.Errorf("could not reach the update server: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return releaseInfo{}, describeAPIFailure(resp)
	}

	var release releaseInfo
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxSumsSize)).Decode(&release); err != nil {
		return releaseInfo{}, fmt.Errorf("could not read the release: %w", err)
	}
	return release, nil
}

// describeAPIFailure turns a refusal into something worth reading.
//
// Rate limiting is the one that actually happens: GitHub allows 60
// unauthenticated requests an hour per address, and an address is a whole
// office behind one NAT as easily as it is one person. "403 Forbidden" tells
// that person nothing; the limit resetting on its own is the entire answer.
func describeAPIFailure(resp *http.Response) error {
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
		if resp.Header.Get("X-RateLimit-Remaining") == "0" {
			when := "shortly"
			if reset := resp.Header.Get("X-RateLimit-Reset"); reset != "" {
				if unix, err := strconv.ParseInt(reset, 10, 64); err == nil {
					if wait, ok := humanWait(time.Until(time.Unix(unix, 0))); ok {
						when = "in about " + wait
					}
				}
			}
			return fmt.Errorf("GitHub is rate-limiting update checks from your network. It will work again %s — or download the new version from the releases page.", when)
		}
	}
	return fmt.Errorf("the update server answered %s", resp.Status)
}

// humanWait renders a wait the way a sentence needs it.
//
// Duration.String() gives "24m0s", which belongs in a log rather than in
// something a person reads. GitHub's window is an hour, so minutes and the
// exact hour cover the whole real range; a wait far outside it means the
// local clock is wrong, and a time quoted against a wrong clock is worse than
// no time at all, so that case declines to give one.
func humanWait(d time.Duration) (string, bool) {
	minutes := int(d.Round(time.Minute).Minutes())
	switch {
	case minutes < 1 || minutes > 120:
		return "", false
	case minutes == 1:
		return "a minute", true
	case minutes < 60:
		return fmt.Sprintf("%d minutes", minutes), true
	default:
		return "an hour", true
	}
}

// Install downloads the newest release and puts it in place.
//
// The old bundle is moved aside rather than deleted, and moved back if
// anything after that point fails. The caller restarts the app; this cannot,
// because the process doing the replacing is the one being replaced.
func (u *Updater) Install(ctx context.Context) (Result, error) {
	ctx, cancel := context.WithTimeout(ctx, installTimeout)
	defer cancel()

	u.log = nil

	update, err := u.Check(ctx)
	if err != nil {
		return Result{}, err
	}
	if !update.Available {
		return Result{Version: u.cfg.CurrentVersion, Output: u.output()}, nil
	}

	if err := u.writable(); err != nil {
		return Result{}, err
	}

	release, err := u.latest(ctx)
	if err != nil {
		return Result{}, err
	}
	assetURL, _, ok := release.asset(AppAsset)
	if !ok {
		return Result{}, fmt.Errorf("release %s has no %s to install", update.Version, AppAsset)
	}

	work, err := os.MkdirTemp(filepath.Dir(u.cfg.BundlePath), ".lasso-update-*")
	if err != nil {
		// Falling back to the system temp directory would put the staged copy
		// on another volume, and the swap below has to be a rename on the same
		// one to be atomic.
		return Result{}, fmt.Errorf("could not stage the update beside the app: %w", err)
	}
	defer os.RemoveAll(work)

	archive := filepath.Join(work, AppAsset)
	if err := u.download(ctx, assetURL, archive, release); err != nil {
		return Result{}, err
	}
	u.note("downloaded and verified " + AppAsset)

	staged, err := u.unpack(ctx, archive, work)
	if err != nil {
		return Result{}, err
	}

	if err := u.carryHelpers(ctx, staged); err != nil {
		return Result{}, err
	}
	if err := u.prepare(ctx, staged); err != nil {
		return Result{}, err
	}

	if err := u.swap(staged); err != nil {
		return Result{}, err
	}

	u.note("replaced " + filepath.Base(u.cfg.BundlePath))
	return Result{
		Version:      update.Version,
		Installed:    true,
		NeedsRestart: true,
		Output:       u.output(),
	}, nil
}

// writable refuses an update the swap could not complete.
//
// Better to say so before downloading 15 MB than after. Running from the disk
// image is the common case — people open the DMG and launch it from there
// without ever dragging it to Applications.
func (u *Updater) writable() error {
	parent := filepath.Dir(u.cfg.BundlePath)

	probe, err := os.CreateTemp(parent, ".lasso-update-probe-*")
	if err != nil {
		if strings.Contains(parent, "/Volumes/") {
			return fmt.Errorf("Lasso is running from a disk image, which cannot be updated in place. Drag it to your Applications folder first")
		}
		return fmt.Errorf("Lasso cannot write to %s, so it cannot replace itself there: %w", parent, err)
	}
	name := probe.Name()
	probe.Close()
	return os.Remove(name)
}

// download fetches the asset, hashing as it goes, and refuses anything whose
// digest does not match the one published alongside it.
func (u *Updater) download(ctx context.Context, url, dest string, release releaseInfo) error {
	want, err := u.expectedDigest(ctx, release)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := u.cfg.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("could not download the update: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("downloading the update failed: %s", resp.Status)
	}

	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()

	digest := sha256.New()
	// Capped so a wrong or hostile URL cannot fill the disk.
	if _, err := io.Copy(io.MultiWriter(f, digest), io.LimitReader(resp.Body, maxDownloadSize)); err != nil {
		return fmt.Errorf("could not download the update: %w", err)
	}

	got := hex.EncodeToString(digest.Sum(nil))
	if got != want {
		return fmt.Errorf("the download does not match its published checksum; it may be damaged or tampered with")
	}
	return nil
}

// expectedDigest reads the SHA256SUMS asset and finds this asset's line.
func (u *Updater) expectedDigest(ctx context.Context, release releaseInfo) (string, error) {
	sumsURL, _, ok := release.asset(SumsAsset)
	if !ok {
		return "", fmt.Errorf("release %s publishes no %s, so the download cannot be verified", release.TagName, SumsAsset)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sumsURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := u.cfg.HTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("could not fetch %s: %w", SumsAsset, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("could not fetch %s: %s", SumsAsset, resp.Status)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxSumsSize))
	if err != nil {
		return "", err
	}

	// Lines are the shasum format: digest, whitespace, then the filename.
	for _, line := range strings.Split(string(body), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		if strings.TrimPrefix(fields[len(fields)-1], "*") == AppAsset {
			return strings.ToLower(fields[0]), nil
		}
	}
	return "", fmt.Errorf("%s does not list %s", SumsAsset, AppAsset)
}

// unpack extracts the archive and returns the app bundle inside it.
//
// ditto rather than Go's archive/zip: it is what created the archive, and it
// carries the extended attributes and symlinks that a bundle depends on. Go's
// zip reader would silently drop them.
func (u *Updater) unpack(ctx context.Context, archive, into string) (string, error) {
	dest := filepath.Join(into, "extracted")
	if _, err := u.cfg.Run(ctx, "/usr/bin/ditto", "-x", "-k", archive, dest); err != nil {
		return "", fmt.Errorf("could not unpack the update: %w", err)
	}

	name := filepath.Base(u.cfg.BundlePath)
	staged := filepath.Join(dest, name)
	if info, err := os.Stat(staged); err != nil || !info.IsDir() {
		return "", fmt.Errorf("the update does not contain %s", name)
	}
	return staged, nil
}

// manifestPath is where the helper programs record their versions.
func manifestPath(bundle string) string {
	return filepath.Join(bundle, "Contents", "Resources", "bin", "manifest.json")
}

type manifest struct {
	Binaries map[string]struct {
		Version string `json:"version"`
	} `json:"binaries"`
}

// readManifest reads a bundle's helper-program versions.
func readManifest(bundle string) (manifest, error) {
	raw, err := os.ReadFile(manifestPath(bundle))
	if err != nil {
		return manifest{}, err
	}
	var m manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return manifest{}, err
	}
	return m, nil
}

// sameHelpers reports whether two manifests pin the same versions.
func sameHelpers(a, b manifest) bool {
	if len(a.Binaries) != len(b.Binaries) {
		return false
	}
	for name, spec := range a.Binaries {
		other, ok := b.Binaries[name]
		if !ok || other.Version != spec.Version {
			return false
		}
	}
	return true
}

// carryHelpers moves the installed helper programs into the staged bundle.
//
// They are the whole reason the download is small. The staged bundle arrives
// with only a manifest where they should be; this puts the real ones back —
// but only after checking that the release wants the same versions. If it
// wants different ones, installing the old helpers would leave the user on a
// build that says it updated and did not.
func (u *Updater) carryHelpers(ctx context.Context, staged string) error {
	wanted, err := readManifest(staged)
	if err != nil {
		return fmt.Errorf("the update does not say which helper programs it needs: %w", err)
	}
	have, err := readManifest(u.cfg.BundlePath)
	if err != nil {
		return fmt.Errorf("cannot read the installed helper programs: %w", err)
	}

	if !sameHelpers(wanted, have) {
		return fmt.Errorf("%w, so it has to be installed from the disk image: %s",
			ErrHelpersChanged, "download it from the release page")
	}

	from := filepath.Join(u.cfg.BundlePath, "Contents", "Resources", "bin")
	to := filepath.Join(staged, "Contents", "Resources", "bin")
	if err := os.RemoveAll(to); err != nil {
		return err
	}
	if _, err := u.cfg.Run(ctx, "/usr/bin/ditto", from, to); err != nil {
		return fmt.Errorf("could not carry the helper programs across: %w", err)
	}

	u.note("carried the helper programs across, unchanged")
	return nil
}

// prepare makes the staged bundle launchable, and refuses to go further if it
// is not.
func (u *Updater) prepare(ctx context.Context, staged string) error {
	// Downloaded by this process rather than a browser, so it should carry no
	// quarantine — but an archive can, and a quarantined replacement would put
	// the user back in front of Gatekeeper, which is the thing this avoids.
	if _, err := u.cfg.Run(ctx, "/usr/bin/xattr", "-d", "-r", "com.apple.quarantine", staged); err != nil {
		// Almost always "no such xattr", which is the good case.
		u.note("no quarantine flag to clear")
	}

	// Re-sealed because the helpers were just written into a bundle that was
	// signed without them. An unsealed bundle opens as "damaged".
	if _, err := u.cfg.Run(ctx, "/usr/bin/codesign", "--force", "--sign", "-", staged); err != nil {
		return fmt.Errorf("could not sign the updated app: %w", err)
	}
	if out, err := u.cfg.Run(ctx, "/usr/bin/codesign", "--verify", "--deep", "--strict", staged); err != nil {
		return fmt.Errorf("the updated app is not correctly signed, so it was not installed: %s", strings.TrimSpace(out))
	}

	u.note("signed and verified the new copy")
	return nil
}

// swap puts the staged bundle where the old one was.
//
// Two renames on the same volume, with the old copy kept until the new one is
// in place. If the second fails the first is undone, so the worst case is the
// app exactly where it started.
func (u *Updater) swap(staged string) error {
	aside := u.cfg.BundlePath + ".old"
	_ = os.RemoveAll(aside)

	if err := os.Rename(u.cfg.BundlePath, aside); err != nil {
		return fmt.Errorf("could not move the old version aside: %w", err)
	}
	if err := os.Rename(staged, u.cfg.BundlePath); err != nil {
		// Put it back rather than leaving no app at all.
		if undo := os.Rename(aside, u.cfg.BundlePath); undo != nil {
			return fmt.Errorf("could not install the update, and could not restore the old version from %s: %w", aside, err)
		}
		return fmt.Errorf("could not install the update; the old version is untouched: %w", err)
	}

	// The old bundle stays until the next launch. This process is still
	// running out of it.
	return nil
}

// CleanUp removes the previous version left behind by an update.
//
// Called at startup, when the running process is no longer inside it. The path
// is derived rather than searched for, so this cannot remove anything but the
// bundle the updater itself set aside.
func CleanUp(bundlePath string) {
	if bundlePath == "" || !strings.HasSuffix(bundlePath, ".app") {
		return
	}
	_ = os.RemoveAll(bundlePath + ".old")
}

func (u *Updater) note(s string) { u.log = append(u.log, s) }

func (u *Updater) output() string { return strings.Join(u.log, "\n") }

// IsNewer compares two dotted version strings.
//
// Numeric per component, so 0.10.0 is newer than 0.9.0 — which a string
// comparison gets backwards, and which is the kind of thing that only bites
// once the version numbers grow.
func IsNewer(current, candidate string) bool {
	return compareVersions(candidate, current) > 0
}

func compareVersions(a, b string) int {
	as, bs := parseVersion(a), parseVersion(b)
	for i := 0; i < len(as) || i < len(bs); i++ {
		av, bv := 0, 0
		if i < len(as) {
			av = as[i]
		}
		if i < len(bs) {
			bv = bs[i]
		}
		if av != bv {
			if av > bv {
				return 1
			}
			return -1
		}
	}
	return 0
}

// parseVersion splits a version into its numeric components, ignoring any
// pre-release suffix. A component that is not a number stops the parse rather
// than being guessed at.
func parseVersion(v string) []int {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if cut := strings.IndexAny(v, "-+"); cut >= 0 {
		v = v[:cut]
	}

	var out []int
	for _, part := range strings.Split(v, ".") {
		n, err := strconv.Atoi(part)
		if err != nil {
			break
		}
		out = append(out, n)
	}
	return out
}
