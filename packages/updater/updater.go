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
//
// It never uses GitHub's API. Everything it reads comes from the release
// download CDN — the latest release's latest.json, then the archive at an
// address derived from the version that manifest names — which does not count
// against the API's 60-an-hour allowance. packages/ghrelease explains, and
// enforces the manners that still apply.
package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/swayyaam/lasso/packages/ghrelease"
)

const (
	// DefaultReleasesURL is where Lasso's releases live. Every address the
	// updater uses is derived from it.
	DefaultReleasesURL = "https://github.com/swayyaam/lasso/releases"

	// ReleasesEnv overrides DefaultReleasesURL. It exists so the whole update —
	// download, verify, carry the helpers across, reseal, swap — can be run
	// against a local server laid out like GitHub, on a real bundle, rather
	// than only ever being tried for the first time by someone's installation.
	//
	// Same idea as LASSO_BIN_SOURCE and LASSO_SUPPORT_DIR in packages/binaries.
	ReleasesEnv = "LASSO_RELEASES_URL"

	// ManifestName is the file every release publishes about itself, read from
	// the latest one at <releases>/latest/download/latest.json.
	ManifestName = "latest.json"

	// ManifestSchema is the manifest format this updater reads. A release that
	// changes the format bumps it, and older copies of Lasso then say so and
	// point at the releases page rather than misreading it.
	ManifestSchema = 1

	// AppAsset is the app on its own, without Contents/Resources/bin.
	AppAsset = "Lasso-app.zip"

	// CheckMaxAge is how long a check is reused — across relaunches too, since
	// the cache lives on disk. Long enough that opening Settings costs
	// nothing, short enough that a release published this morning is offered
	// this afternoon.
	CheckMaxAge = 6 * time.Hour

	checkTimeout    = 30 * time.Second
	installTimeout  = 15 * time.Minute
	maxDownloadSize = 128 << 20
)

// ErrHelpersChanged means the release ships different helper programs, so the
// small update cannot carry the installed ones across.
var ErrHelpersChanged = errors.New("this release updates the helper programs too")

// Manifest is latest.json: what a release says about itself.
//
// It is everything the updater needs to decide, in one small request: the
// version, the notes shown before installing, and each file's size and
// digest. It never carries an address. Where files live is derived from the
// version, so a manifest cannot send the updater anywhere else.
//
// cmd/release-manifest writes it with this same type, so there is one
// definition of the shape.
type Manifest struct {
	SchemaVersion int                  `json:"schemaVersion"`
	Version       string               `json:"version"`
	Published     string               `json:"published"`
	Notes         string               `json:"notes"`
	Assets        map[string]AssetInfo `json:"assets"`
}

// AssetInfo describes one published file.
type AssetInfo struct {
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

var (
	versionPattern = regexp.MustCompile(`^\d+\.\d+\.\d+$`)
	digestPattern  = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// Validate refuses a manifest the updater should not act on.
//
// The version becomes part of a download address, so it has to be exactly a
// version; and a file listed without a usable size and digest could not be
// verified, so it is not listed at all as far as the updater is concerned.
func (m Manifest) Validate() error {
	if m.SchemaVersion != ManifestSchema {
		return fmt.Errorf("the newest release describes itself in a format this version of Lasso does not read (format %d). Download it from the releases page", m.SchemaVersion)
	}
	if !versionPattern.MatchString(m.Version) {
		return fmt.Errorf("the release manifest names an unusable version %q", m.Version)
	}
	for name, a := range m.Assets {
		if a.Size <= 0 || !digestPattern.MatchString(a.SHA256) {
			return fmt.Errorf("the release manifest describes %s without a usable size and checksum", name)
		}
	}
	return nil
}

// Update describes a release that is newer than the running app.
type Update struct {
	// Version is the release's version, without the leading v.
	Version string `json:"version"`
	// Notes is the release's notes, shown before installing.
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
	// ReleasesURL defaults to DefaultReleasesURL. Tests point it at a local
	// server laid out the same way.
	ReleasesURL string
	// Releases fetches release files. Pass the process's shared client, so a
	// refusal from GitHub reaches every updater and the cache outlives the
	// process; nil builds a private one that keeps nothing between launches.
	Releases *ghrelease.Client
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
	if cfg.ReleasesURL == "" {
		cfg.ReleasesURL = DefaultReleasesURL
	}
	cfg.ReleasesURL = strings.TrimSuffix(cfg.ReleasesURL, "/")
	if cfg.Releases == nil {
		cfg.Releases = ghrelease.New(ghrelease.Config{UserAgent: "Lasso/" + cfg.CurrentVersion})
	}
	if cfg.Run == nil {
		cfg.Run = execRun
	}
	return &Updater{cfg: cfg}, nil
}

// Check asks whether a newer release exists.
//
// It reads one small file and touches nothing on disk. A check younger than
// maxAge is answered from the cache without asking at all; zero is a
// deliberate "check now", which is still spaced and still honours a refusal.
func (u *Updater) Check(ctx context.Context, maxAge time.Duration) (Update, error) {
	ctx, cancel := context.WithTimeout(ctx, checkTimeout)
	defer cancel()

	m, err := u.manifest(ctx, maxAge)
	if err != nil {
		return Update{}, err
	}

	update := Update{Version: m.Version, Notes: m.Notes, PageURL: u.pageURL(m.Version)}
	if !IsNewer(u.cfg.CurrentVersion, m.Version) {
		return update, nil
	}
	// A release with no app archive cannot be installed from here, so it is
	// not offered as one: saying an update is available and then failing to
	// install it would be worse than staying quiet.
	app, ok := m.Assets[AppAsset]
	if !ok {
		return update, nil
	}
	update.Available = true
	update.Bytes = app.Size
	return update, nil
}

func (u *Updater) manifest(ctx context.Context, maxAge time.Duration) (Manifest, error) {
	body, err := u.cfg.Releases.Get(ctx, u.cfg.ReleasesURL+"/latest/download/"+ManifestName, maxAge)
	if errors.Is(err, ghrelease.ErrNotFound) {
		return Manifest{}, fmt.Errorf("the newest Lasso release does not describe itself for the updater, so it can only be installed from the releases page")
	}
	if err != nil {
		return Manifest{}, err
	}
	var m Manifest
	if err := json.Unmarshal(body, &m); err != nil {
		return Manifest{}, fmt.Errorf("could not read the release manifest: %w", err)
	}
	if err := m.Validate(); err != nil {
		return Manifest{}, err
	}
	return m, nil
}

func (u *Updater) pageURL(version string) string {
	return u.cfg.ReleasesURL + "/tag/v" + version
}

func (u *Updater) assetURL(version, name string) string {
	return u.cfg.ReleasesURL + "/download/v" + version + "/" + name
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

	// The check that offered this update is usually seconds old, so the
	// manifest comes from the cache: installing exactly what was offered, and
	// costing no extra request to do it.
	m, err := u.manifest(ctx, CheckMaxAge)
	if err != nil {
		return Result{}, err
	}
	app, ok := m.Assets[AppAsset]
	if !ok || !IsNewer(u.cfg.CurrentVersion, m.Version) {
		return Result{Version: u.cfg.CurrentVersion, Output: u.output()}, nil
	}

	if err := u.writable(); err != nil {
		return Result{}, err
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
	if err := u.download(ctx, u.assetURL(m.Version, AppAsset), archive, app); err != nil {
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
		Version:      m.Version,
		Installed:    true,
		NeedsRestart: true,
		Output:       u.output(),
	}, nil
}

// writable refuses an update the swap could not complete.
//
// Better to say so before downloading 5 MB than after. Running from the disk
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

// download fetches the app archive and refuses it unless its bytes are
// exactly the ones the manifest describes.
func (u *Updater) download(ctx context.Context, url, dest string, want AssetInfo) error {
	// The declared size is the ceiling: anything larger is not what was
	// published, and is cut off rather than written to disk in full.
	limit := min(want.Size, maxDownloadSize)
	if err := u.cfg.Releases.Download(ctx, url, dest, limit); err != nil {
		return fmt.Errorf("could not download the update: %w", err)
	}
	got, err := fileDigest(dest)
	if err != nil {
		return err
	}
	if got != want.SHA256 {
		return fmt.Errorf("the download does not match its published checksum; it may be damaged or tampered with")
	}
	return nil
}

func fileDigest(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
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
