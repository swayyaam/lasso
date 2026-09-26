package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/swayyaam/lasso/packages/ghrelease"
)

// ---- fixtures ---------------------------------------------------------

// makeBundle writes a directory shaped like Lasso.app, with helper programs
// pinned to the given versions.
func makeBundle(t *testing.T, path string, appVersion string, helpers map[string]string) {
	t.Helper()

	binDir := filepath.Join(path, "Contents", "Resources", "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(path, "Contents", "MacOS"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(path, "Contents", "MacOS", "Lasso"), "binary-"+appVersion)
	write(t, filepath.Join(path, "Contents", "Info.plist"), appVersion)
	write(t, manifestPath(path), manifestJSON(helpers))

	// Something big enough to tell apart from the app itself, standing in for
	// the 328 MB the real bundle carries.
	for name := range helpers {
		write(t, filepath.Join(binDir, name), "helper-"+name+"-"+helpers[name])
	}
}

func manifestJSON(helpers map[string]string) string {
	type spec struct {
		Version string `json:"version"`
	}
	doc := struct {
		Binaries map[string]spec `json:"binaries"`
	}{Binaries: map[string]spec{}}
	for name, version := range helpers {
		doc.Binaries[name] = spec{Version: version}
	}
	raw, _ := json.Marshal(doc)
	return string(raw)
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
}

func digestOf(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// releaseServer stands in for github.com, laid out the way GitHub serves
// releases: the latest release's manifest under /releases/latest/download/,
// each version's files under /releases/download/v<version>/. There is no API
// here, and any request outside that layout fails the test.
type releaseServer struct {
	*httptest.Server
	t       *testing.T
	version string
	archive []byte

	// omitApp publishes a manifest with no app archive in it.
	omitApp bool
	// omitManifest makes latest.json a 404, as a release without one would be.
	omitManifest bool
	// corruptDigest publishes a checksum that does not match the archive.
	corruptDigest bool
	// declaredSize overrides the archive size the manifest publishes.
	declaredSize int64
	// status, when set, is the answer to every request.
	status     int
	retryAfter string

	mu     sync.Mutex
	hits   map[string]int
	agents []string
}

func newReleaseServer(t *testing.T, tag string, archive []byte) *releaseServer {
	t.Helper()
	rs := &releaseServer{t: t, version: strings.TrimPrefix(tag, "v"), archive: archive, hits: map[string]int{}}
	rs.Server = httptest.NewServer(http.HandlerFunc(rs.serve))
	t.Cleanup(rs.Close)
	return rs
}

func (rs *releaseServer) serve(w http.ResponseWriter, r *http.Request) {
	rs.mu.Lock()
	rs.hits[r.URL.Path]++
	rs.agents = append(rs.agents, r.Header.Get("User-Agent"))
	version := rs.version
	rs.mu.Unlock()

	if !strings.HasPrefix(r.URL.Path, "/releases/") {
		rs.t.Errorf("a request left the releases layout: %s", r.URL.Path)
	}
	if rs.status != 0 {
		if rs.retryAfter != "" {
			w.Header().Set("Retry-After", rs.retryAfter)
		}
		w.WriteHeader(rs.status)
		return
	}

	switch r.URL.Path {
	case "/releases/latest/download/" + ManifestName:
		if rs.omitManifest {
			http.NotFound(w, r)
			return
		}
		m := Manifest{
			SchemaVersion: ManifestSchema,
			Version:       version,
			Published:     "2026-09-23T00:00:00Z",
			Notes:         "notes for " + version,
			Assets:        map[string]AssetInfo{},
		}
		if !rs.omitApp {
			digest := digestOf(rs.archive)
			if rs.corruptDigest {
				digest = strings.Repeat("0", 64)
			}
			size := int64(len(rs.archive))
			if rs.declaredSize != 0 {
				size = rs.declaredSize
			}
			m.Assets[AppAsset] = AssetInfo{Size: size, SHA256: digest}
		}
		json.NewEncoder(w).Encode(m)
	case "/releases/download/v" + version + "/" + AppAsset:
		w.Write(rs.archive)
	default:
		http.NotFound(w, r)
	}
}

// publish makes tag the latest release, as a new release on GitHub would.
func (rs *releaseServer) publish(tag string) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	rs.version = strings.TrimPrefix(tag, "v")
}

func (rs *releaseServer) count(path string) int {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	return rs.hits[path]
}

func (rs *releaseServer) total() int {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	n := 0
	for _, c := range rs.hits {
		n += c
	}
	return n
}

// fakeRunner stands in for ditto, codesign and xattr.
//
// ditto is the only one that has to do anything real — the swap depends on
// files actually being where it put them — so it copies, and the rest are
// recorded and succeed.
type fakeRunner struct {
	calls []string
	// stagedVersion is what the "downloaded" bundle claims to be.
	stagedVersion string
	// stagedHelpers is the manifest the release wants.
	stagedHelpers map[string]string
	// failVerify makes codesign --verify reject the staged bundle.
	failVerify bool
	t          *testing.T
}

func (f *fakeRunner) run(_ context.Context, name string, args ...string) (string, error) {
	f.calls = append(f.calls, name+" "+strings.Join(args, " "))
	base := filepath.Base(name)

	switch {
	case base == "ditto" && len(args) >= 4 && args[0] == "-x":
		// Unpacking: write the bundle the archive would have contained.
		dest := args[3]
		makeBundle(f.t, filepath.Join(dest, "Lasso.app"), f.stagedVersion, f.stagedHelpers)
		// The real thin archive ships only the manifest, not the helpers.
		binDir := filepath.Join(dest, "Lasso.app", "Contents", "Resources", "bin")
		entries, _ := os.ReadDir(binDir)
		for _, e := range entries {
			if e.Name() != "manifest.json" {
				os.Remove(filepath.Join(binDir, e.Name()))
			}
		}
		return "", nil

	case base == "ditto" && len(args) == 2:
		// Carrying the helpers across.
		return "", copyTree(args[0], args[1])

	case base == "codesign" && len(args) > 0 && args[0] == "--verify":
		if f.failVerify {
			return "a sealed resource is missing or invalid", errors.New("exit status 1")
		}
		return "", nil
	}
	return "", nil
}

func copyTree(from, to string) error {
	return filepath.Walk(from, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(from, path)
		if err != nil {
			return err
		}
		target := filepath.Join(to, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, raw, info.Mode())
	})
}

// harness wires an updater to a fake release and a fake toolchain.
type harness struct {
	u      *Updater
	bundle string
	runner *fakeRunner
	server *releaseServer
}

func newHarness(t *testing.T, currentVersion, releaseTag string, helpers map[string]string) *harness {
	t.Helper()

	// The bundle lives in its own directory, because the updater stages the
	// replacement as a sibling and then renames over it.
	root := t.TempDir()
	bundle := filepath.Join(root, "Lasso.app")
	makeBundle(t, bundle, currentVersion, helpers)

	server := newReleaseServer(t, releaseTag, []byte("pretend-zip-"+releaseTag))
	runner := &fakeRunner{
		t:             t,
		stagedVersion: strings.TrimPrefix(releaseTag, "v"),
		stagedHelpers: helpers,
	}

	u, err := New(Config{
		BundlePath:     bundle,
		CurrentVersion: currentVersion,
		ReleasesURL:    server.URL + "/releases",
		Run:            runner.run,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return &harness{u: u, bundle: bundle, runner: runner, server: server}
}

// ---- version comparison ----------------------------------------------

func TestIsNewer(t *testing.T) {
	cases := []struct {
		current, candidate string
		want               bool
	}{
		{"0.1.0", "0.1.1", true},
		{"0.1.1", "0.1.0", false},
		{"0.1.1", "0.1.1", false},
		{"0.1.0", "1.0.0", true},
		// The one a string comparison gets backwards, and the reason this is
		// not a string comparison.
		{"0.9.0", "0.10.0", true},
		{"0.10.0", "0.9.0", false},
		// Tags carry a v; versions from Info.plist do not.
		{"0.1.0", "v0.1.1", true},
		// Shorter is not automatically older.
		{"1.0", "1.0.0", false},
		{"1.0", "1.0.1", true},
		// Pre-release suffixes are ignored rather than guessed at.
		{"0.1.0", "0.2.0-beta.1", true},
		// Nonsense must not read as an upgrade.
		{"0.1.0", "", false},
		{"0.1.0", "not-a-version", false},
	}

	for _, c := range cases {
		t.Run(c.current+"->"+c.candidate, func(t *testing.T) {
			if got := IsNewer(c.current, c.candidate); got != c.want {
				t.Errorf("IsNewer(%q, %q) = %v, want %v", c.current, c.candidate, got, c.want)
			}
		})
	}
}

// ---- checking ---------------------------------------------------------

func TestCheckFindsANewerRelease(t *testing.T) {
	h := newHarness(t, "0.1.0", "v0.2.0", map[string]string{"yt-dlp": "1"})

	update, err := h.u.Check(context.Background(), 0)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if !update.Available {
		t.Fatal("a newer release was not reported as available")
	}
	if update.Version != "0.2.0" {
		t.Errorf("Version = %q, want the tag without its v", update.Version)
	}
	if update.Notes == "" || update.PageURL == "" {
		t.Error("the release notes and page should come back for the UI to show")
	}
}

func TestCheckSaysNothingWhenCurrent(t *testing.T) {
	h := newHarness(t, "0.2.0", "v0.2.0", map[string]string{"yt-dlp": "1"})

	update, err := h.u.Check(context.Background(), 0)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if update.Available {
		t.Error("the running version was offered as an update to itself")
	}
}

func TestCheckIgnoresAReleaseItCannotInstall(t *testing.T) {
	// Offering an update and then failing to install it is worse than staying
	// quiet, so a release with no app asset is not an available update.
	h := newHarness(t, "0.1.0", "v0.2.0", map[string]string{"yt-dlp": "1"})
	h.server.omitApp = true

	update, err := h.u.Check(context.Background(), 0)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if update.Available {
		t.Error("a release with no installable asset was offered")
	}
}

func TestCheckReportsAnUnreachableServer(t *testing.T) {
	u, err := New(Config{
		BundlePath:     filepath.Join(t.TempDir(), "Lasso.app"),
		CurrentVersion: "0.1.0",
		ReleasesURL:    "http://127.0.0.1:1/releases",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := u.Check(context.Background(), 0); err == nil {
		t.Error("an unreachable update server was not reported")
	}
}

// ---- installing -------------------------------------------------------

func TestInstallReplacesTheApp(t *testing.T) {
	helpers := map[string]string{"yt-dlp": "2026.08.19", "ffmpeg": "9.0.1"}
	h := newHarness(t, "0.1.0", "v0.2.0", helpers)

	result, err := h.u.Install(context.Background())
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if !result.Installed || !result.NeedsRestart {
		t.Fatalf("result = %+v, want an installed update awaiting restart", result)
	}

	// The app is the new one.
	binary, err := os.ReadFile(filepath.Join(h.bundle, "Contents", "MacOS", "Lasso"))
	if err != nil {
		t.Fatalf("reading the installed binary: %v", err)
	}
	if string(binary) != "binary-0.2.0" {
		t.Errorf("installed binary is %q, want the new version", binary)
	}

	// And the helpers came across, which is the whole point of the small
	// download: the archive never contained them.
	for name, version := range helpers {
		raw, err := os.ReadFile(filepath.Join(h.bundle, "Contents", "Resources", "bin", name))
		if err != nil {
			t.Fatalf("helper %s is missing after the update: %v", name, err)
		}
		if want := "helper-" + name + "-" + version; string(raw) != want {
			t.Errorf("helper %s = %q, want %q", name, raw, want)
		}
	}
}

func TestInstallResealsTheBundle(t *testing.T) {
	// The helpers are written into a bundle that was signed without them, so
	// the seal has to be replaced or the app opens as "damaged" — the exact
	// bug that broke v0.1.0.
	h := newHarness(t, "0.1.0", "v0.2.0", map[string]string{"yt-dlp": "1"})

	if _, err := h.u.Install(context.Background()); err != nil {
		t.Fatalf("Install: %v", err)
	}

	var signed, verified bool
	for _, call := range h.runner.calls {
		if strings.Contains(call, "codesign --force --sign -") {
			signed = true
		}
		if strings.Contains(call, "codesign --verify") {
			verified = true
		}
	}
	if !signed {
		t.Error("the updated bundle was never re-signed")
	}
	if !verified {
		t.Error("the updated bundle was never verified")
	}
}

func TestInstallRefusesAnUnverifiableBundle(t *testing.T) {
	// If the reseal did not take, installing it would hand the user an app
	// macOS calls damaged. Better to keep the working one.
	h := newHarness(t, "0.1.0", "v0.2.0", map[string]string{"yt-dlp": "1"})
	h.runner.failVerify = true

	if _, err := h.u.Install(context.Background()); err == nil {
		t.Fatal("an unverifiable update was installed")
	}

	binary, err := os.ReadFile(filepath.Join(h.bundle, "Contents", "MacOS", "Lasso"))
	if err != nil {
		t.Fatalf("the old app is gone: %v", err)
	}
	if string(binary) != "binary-0.1.0" {
		t.Errorf("installed binary is %q, want the original to be untouched", binary)
	}
}

func TestInstallRefusesAWrongChecksum(t *testing.T) {
	h := newHarness(t, "0.1.0", "v0.2.0", map[string]string{"yt-dlp": "1"})
	h.server.corruptDigest = true

	_, err := h.u.Install(context.Background())
	if err == nil {
		t.Fatal("a download that did not match its checksum was installed")
	}
	if !strings.Contains(err.Error(), "checksum") {
		t.Errorf("error = %v, want it to name the checksum", err)
	}
	assertUntouched(t, h.bundle, "0.1.0")
}

func TestInstallRefusesWithoutAManifest(t *testing.T) {
	// The manifest carries the digest. Without it there is no way to know
	// what was downloaded, and the update replaces the whole application.
	h := newHarness(t, "0.1.0", "v0.2.0", map[string]string{"yt-dlp": "1"})
	h.server.omitManifest = true

	if _, err := h.u.Install(context.Background()); err == nil {
		t.Fatal("an update with no manifest was installed")
	}
	assertUntouched(t, h.bundle, "0.1.0")
}

func TestInstallRefusesAnArchiveLargerThanPublished(t *testing.T) {
	// The declared size is the ceiling, so a swapped or padded archive is cut
	// off rather than written to disk in full and then rejected.
	h := newHarness(t, "0.1.0", "v0.2.0", map[string]string{"yt-dlp": "1"})
	h.server.declaredSize = int64(len(h.server.archive)) - 1

	if _, err := h.u.Install(context.Background()); err == nil {
		t.Fatal("an archive larger than the manifest said was installed")
	}
	assertUntouched(t, h.bundle, "0.1.0")
}

func TestInstallRefusesWhenHelpersChanged(t *testing.T) {
	// Carrying the old helpers across would leave the user on a build that
	// says it updated and did not.
	h := newHarness(t, "0.1.0", "v0.2.0", map[string]string{"yt-dlp": "2026.08.19"})
	h.runner.stagedHelpers = map[string]string{"yt-dlp": "2026.12.01"}

	_, err := h.u.Install(context.Background())
	if err == nil {
		t.Fatal("an update needing different helper programs was installed")
	}
	if !errors.Is(err, ErrHelpersChanged) {
		t.Errorf("error = %v, want it to be ErrHelpersChanged so the UI can explain", err)
	}
	assertUntouched(t, h.bundle, "0.1.0")
}

func TestInstallTakesTheNewestReleaseNotTheOneOffered(t *testing.T) {
	// Settings offered 0.2.2, and by the time Update was pressed 0.2.3 was
	// out. Installing the offer left a second update to go through at once:
	// that is how 0.2.1 stepped to 0.2.2 and only then reached 0.2.3.
	helpers := map[string]string{"yt-dlp": "1"}
	bundle := filepath.Join(t.TempDir(), "Lasso.app")
	makeBundle(t, bundle, "0.2.1", helpers)
	server := newReleaseServer(t, "v0.2.2", []byte("pretend-zip"))
	runner := &fakeRunner{t: t, stagedVersion: "0.2.2", stagedHelpers: helpers}

	now := time.Date(2026, 9, 26, 5, 0, 0, 0, time.UTC)
	u, err := New(Config{
		BundlePath:     bundle,
		CurrentVersion: "0.2.1",
		ReleasesURL:    server.URL + "/releases",
		Releases:       ghrelease.New(ghrelease.Config{UserAgent: "Lasso/test", Now: func() time.Time { return now }}),
		Run:            runner.run,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	offer, err := u.Check(context.Background(), CheckMaxAge)
	if err != nil || !offer.Available || offer.Version != "0.2.2" {
		t.Fatalf("Check = %+v, %v; want 0.2.2 offered", offer, err)
	}

	server.publish("v0.2.3")
	runner.stagedVersion = "0.2.3"
	// Long enough that asking again is allowed; well inside the time a check
	// is reused, so the offer on screen still names 0.2.2.
	now = now.Add(time.Minute)

	result, err := u.Install(context.Background())
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if result.Version != "0.2.3" {
		t.Errorf("installed %q, want 0.2.3: the newest release, not the one offered", result.Version)
	}
	binary, err := os.ReadFile(filepath.Join(bundle, "Contents", "MacOS", "Lasso"))
	if err != nil {
		t.Fatal(err)
	}
	if string(binary) != "binary-0.2.3" {
		t.Errorf("installed binary is %q, want 0.2.3's", binary)
	}
}

func TestAnOfferOlderThanCheckMaxAgeIsAskedAgain(t *testing.T) {
	h := newHarness(t, "0.2.1", "v0.2.2", map[string]string{"yt-dlp": "1"})
	now := time.Date(2026, 9, 26, 5, 0, 0, 0, time.UTC)
	h.u.cfg.Releases = ghrelease.New(ghrelease.Config{UserAgent: "Lasso/test", Now: func() time.Time { return now }})

	if offer, _ := h.u.Check(context.Background(), CheckMaxAge); offer.Version != "0.2.2" {
		t.Fatalf("first offer = %q, want 0.2.2", offer.Version)
	}
	h.server.publish("v0.2.3")
	now = now.Add(CheckMaxAge + time.Second)
	if offer, _ := h.u.Check(context.Background(), CheckMaxAge); offer.Version != "0.2.3" {
		t.Errorf("offer after CheckMaxAge = %q, want 0.2.3", offer.Version)
	}
}

func TestInstallDoesNothingWhenCurrent(t *testing.T) {
	h := newHarness(t, "0.2.0", "v0.2.0", map[string]string{"yt-dlp": "1"})

	result, err := h.u.Install(context.Background())
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if result.Installed || result.NeedsRestart {
		t.Errorf("result = %+v, want nothing to have happened", result)
	}
}

func TestInstallRefusesAReadOnlyLocation(t *testing.T) {
	// Running from the mounted disk image is the common case: people open the
	// DMG and launch it from there without ever dragging it anywhere.
	root := t.TempDir()
	bundle := filepath.Join(root, "Lasso.app")
	makeBundle(t, bundle, "0.1.0", map[string]string{"yt-dlp": "1"})

	if err := os.Chmod(root, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(root, 0o700) })

	server := newReleaseServer(t, "v0.2.0", []byte("zip"))
	u, _ := New(Config{
		BundlePath:     bundle,
		CurrentVersion: "0.1.0",
		ReleasesURL:    server.URL + "/releases",
		Run:            (&fakeRunner{t: t, stagedVersion: "0.2.0"}).run,
	})

	if _, err := u.Install(context.Background()); err == nil {
		t.Fatal("an update was attempted into a location that cannot be written")
	}
}

// ---- cleanup ----------------------------------------------------------

func TestCleanUpRemovesOnlyTheSetAsideBundle(t *testing.T) {
	root := t.TempDir()
	bundle := filepath.Join(root, "Lasso.app")
	makeBundle(t, bundle, "0.2.0", map[string]string{"yt-dlp": "1"})

	old := bundle + ".old"
	makeBundle(t, old, "0.1.0", map[string]string{"yt-dlp": "1"})

	// Something that must survive: the path is derived, never searched for.
	bystander := filepath.Join(root, "Something Else.app")
	makeBundle(t, bystander, "1.0.0", map[string]string{"yt-dlp": "1"})

	CleanUp(bundle)

	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Error("the previous version was not removed")
	}
	if _, err := os.Stat(bundle); err != nil {
		t.Error("the current version was removed")
	}
	if _, err := os.Stat(bystander); err != nil {
		t.Error("CleanUp removed something that was not its business")
	}
}

func TestCleanUpIgnoresAnythingThatIsNotABundle(t *testing.T) {
	// Called with whatever os.Executable resolved to, which in development is
	// a bare binary in a temporary directory.
	root := t.TempDir()
	stray := filepath.Join(root, "lasso")
	write(t, stray, "dev binary")
	write(t, stray+".old", "should survive")

	CleanUp(stray)

	if _, err := os.Stat(stray + ".old"); err != nil {
		t.Error("CleanUp removed a path it should not have touched")
	}
	CleanUp("")
}

func assertUntouched(t *testing.T, bundle, version string) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(bundle, "Contents", "MacOS", "Lasso"))
	if err != nil {
		t.Fatalf("the app is gone after a failed update: %v", err)
	}
	if want := "binary-" + version; string(raw) != want {
		t.Errorf("binary = %q, want the failed update to have changed nothing (%q)", raw, want)
	}
	if _, err := os.Stat(bundle + ".old"); err == nil {
		t.Error("a failed update left the old bundle moved aside")
	}
}

// ---- staying off GitHub's API ----------------------------------------
//
// The manners themselves — caching, refusals across relaunches, spacing,
// backoff, serial requests — are tested in packages/ghrelease. These check
// that the updater goes through them and never around them.

func TestAnUpdateCostsOneSmallRequestAndTheArchive(t *testing.T) {
	h := newHarness(t, "0.1.0", "v0.2.0", map[string]string{"yt-dlp": "1"})

	if _, err := h.u.Check(context.Background(), 0); err != nil {
		t.Fatalf("Check: %v", err)
	}
	if _, err := h.u.Install(context.Background()); err != nil {
		t.Fatalf("Install: %v", err)
	}
	// serve() has already failed the test if anything went outside /releases/.
	if n := h.server.count("/releases/latest/download/" + ManifestName); n != 1 {
		t.Errorf("latest.json was fetched %d times for a check and an install; want 1, the install reusing the check's copy", n)
	}
	if n := h.server.count("/releases/download/v0.2.0/" + AppAsset); n != 1 {
		t.Errorf("the archive was fetched %d times; want 1", n)
	}
}

func TestChecksWithinMaxAgeAskNothing(t *testing.T) {
	// Opening Settings checks. Opening it again, or five times, must not.
	h := newHarness(t, "0.1.0", "v0.2.0", map[string]string{"yt-dlp": "1"})

	for i := 0; i < 5; i++ {
		if _, err := h.u.Check(context.Background(), CheckMaxAge); err != nil {
			t.Fatalf("Check: %v", err)
		}
	}
	if n := h.server.total(); n != 1 {
		t.Errorf("%d requests for five checks; want 1", n)
	}
}

func TestARefusalIsExplainedAndHonouredByCheckAndInstall(t *testing.T) {
	h := newHarness(t, "0.1.0", "v0.2.0", map[string]string{"yt-dlp": "1"})
	h.server.status = http.StatusTooManyRequests
	h.server.retryAfter = "900"

	_, err := h.u.Check(context.Background(), 0)
	if err == nil || !strings.Contains(err.Error(), "slow down") || !strings.Contains(err.Error(), "15 minutes") {
		t.Fatalf("err = %v, want the slow-down explanation with the wait in words", err)
	}
	if _, err := h.u.Check(context.Background(), 0); err == nil {
		t.Error("a forced check went ahead inside the refusal window")
	}
	if _, err := h.u.Install(context.Background()); err == nil {
		t.Error("an install went ahead inside the refusal window")
	}
	if n := h.server.total(); n != 1 {
		t.Errorf("%d requests reached GitHub; want 1, the rest refused locally", n)
	}
}

func TestAMissingManifestIsExplained(t *testing.T) {
	h := newHarness(t, "0.1.0", "v0.2.0", map[string]string{"yt-dlp": "1"})
	h.server.omitManifest = true

	_, err := h.u.Check(context.Background(), 0)
	if err == nil || !strings.Contains(err.Error(), "releases page") {
		t.Errorf("err = %v, want it to point at the releases page", err)
	}
}

func TestAManifestThatCouldMisdirectTheDownloadIsRefused(t *testing.T) {
	// The version becomes part of the download address. Anything that is not
	// exactly a version could walk that address somewhere else.
	good := strings.Repeat("a", 64)
	for _, m := range []Manifest{
		{SchemaVersion: 1, Version: "../../evil"},
		{SchemaVersion: 1, Version: "1.0.0/../2.0.0"},
		{SchemaVersion: 1, Version: "v1.0.0"},
		{SchemaVersion: 2, Version: "1.0.0"},
		{SchemaVersion: 1, Version: "1.0.0", Assets: map[string]AssetInfo{AppAsset: {Size: 10, SHA256: "not-a-digest"}}},
		{SchemaVersion: 1, Version: "1.0.0", Assets: map[string]AssetInfo{AppAsset: {Size: 0, SHA256: good}}},
	} {
		if err := m.Validate(); err == nil {
			t.Errorf("Validate accepted %+v", m)
		}
	}
	ok := Manifest{SchemaVersion: 1, Version: "1.0.0", Assets: map[string]AssetInfo{AppAsset: {Size: 10, SHA256: good}}}
	if err := ok.Validate(); err != nil {
		t.Errorf("Validate refused a good manifest: %v", err)
	}
}

func TestCheckIdentifiesItself(t *testing.T) {
	h := newHarness(t, "0.1.0", "v0.2.0", map[string]string{"yt-dlp": "1"})
	if _, err := h.u.Check(context.Background(), 0); err != nil {
		t.Fatal(err)
	}
	h.server.mu.Lock()
	defer h.server.mu.Unlock()
	for _, agent := range h.server.agents {
		if agent != "Lasso/0.1.0" {
			t.Errorf("User-Agent = %q, want Lasso/0.1.0", agent)
		}
	}
}
