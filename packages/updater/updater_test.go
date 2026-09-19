package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
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

// releaseServer stands in for the GitHub release endpoints.
type releaseServer struct {
	*httptest.Server
	archive []byte
	// omitSums drops the checksum asset, which must stop an install.
	omitSums bool
	// omitApp drops the app asset.
	omitApp bool
	// corruptDigest publishes a checksum that does not match the archive.
	corruptDigest bool
}

func newReleaseServer(t *testing.T, tag string, archive []byte) *releaseServer {
	t.Helper()
	rs := &releaseServer{archive: archive}

	mux := http.NewServeMux()
	mux.HandleFunc("/release", func(w http.ResponseWriter, _ *http.Request) {
		type asset struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
			Size int64  `json:"size"`
		}
		var assets []asset
		if !rs.omitApp {
			assets = append(assets, asset{AppAsset, rs.URL + "/app.zip", int64(len(rs.archive))})
		}
		if !rs.omitSums {
			assets = append(assets, asset{SumsAsset, rs.URL + "/sums", 128})
		}
		json.NewEncoder(w).Encode(map[string]any{
			"tag_name": tag,
			"body":     "notes for " + tag,
			"html_url": "https://example.com/releases/" + tag,
			"assets":   assets,
		})
	})
	mux.HandleFunc("/app.zip", func(w http.ResponseWriter, _ *http.Request) {
		w.Write(rs.archive)
	})
	mux.HandleFunc("/sums", func(w http.ResponseWriter, _ *http.Request) {
		digest := digestOf(rs.archive)
		if rs.corruptDigest {
			digest = strings.Repeat("0", 64)
		}
		fmt.Fprintf(w, "%s  %s\n%s  Lasso.dmg\n", digest, AppAsset, strings.Repeat("a", 64))
	})

	rs.Server = httptest.NewServer(mux)
	t.Cleanup(rs.Close)
	return rs
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
		ReleaseAPI:     server.URL + "/release",
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

	update, err := h.u.Check(context.Background())
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

	update, err := h.u.Check(context.Background())
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

	update, err := h.u.Check(context.Background())
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
		ReleaseAPI:     "http://127.0.0.1:1/release",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := u.Check(context.Background()); err == nil {
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

func TestInstallRefusesWithoutPublishedChecksums(t *testing.T) {
	// No checksums means no way to know what was downloaded, and the update is
	// a replacement for the whole application.
	h := newHarness(t, "0.1.0", "v0.2.0", map[string]string{"yt-dlp": "1"})
	h.server.omitSums = true

	if _, err := h.u.Install(context.Background()); err == nil {
		t.Fatal("an unverifiable download was installed")
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
		ReleaseAPI:     server.URL + "/release",
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

func TestRateLimitIsExplainedRatherThanShown(t *testing.T) {
	// GitHub allows 60 unauthenticated calls an hour per address, and an
	// address can be a whole office. "403 Forbidden" tells that person
	// nothing, and the limit resetting on its own is the whole answer.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("X-RateLimit-Reset", fmt.Sprint(time.Now().Add(23*time.Minute).Unix()))
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"message":"API rate limit exceeded"}`)
	}))
	t.Cleanup(server.Close)

	u, _ := New(Config{
		BundlePath:     filepath.Join(t.TempDir(), "Lasso.app"),
		CurrentVersion: "0.1.0",
		ReleaseAPI:     server.URL,
	})

	_, err := u.Check(context.Background())
	if err == nil {
		t.Fatal("a rate-limited check reported success")
	}
	if !strings.Contains(err.Error(), "rate-limiting") {
		t.Errorf("error = %v, want it to name rate limiting", err)
	}
	if strings.Contains(err.Error(), "403") {
		t.Errorf("error = %v, want the status code kept out of it", err)
	}
	// The reset time is the actionable part, in words rather than in Go's
	// duration syntax — "23m0s" is a log line, not a sentence.
	if !strings.Contains(err.Error(), "23 minutes") {
		t.Errorf("error = %v, want it to say when it will work again", err)
	}
	if strings.Contains(err.Error(), "m0s") {
		t.Errorf("error = %v, want no raw Duration in it", err)
	}
}

func TestHumanWaitReadsAsASentence(t *testing.T) {
	// The whole point of this helper is that its output is dropped into
	// prose, so each case is checked as the words it produces.
	cases := []struct {
		in   time.Duration
		want string
		ok   bool
	}{
		{20 * time.Second, "", false}, // rounds to nothing to say
		{90 * time.Second, "2 minutes", true},
		{time.Minute, "a minute", true},
		{23 * time.Minute, "23 minutes", true},
		{59 * time.Minute, "59 minutes", true},
		{time.Hour, "an hour", true},
		// A wrong local clock, which would otherwise quote a confident and
		// completely wrong time.
		{9 * time.Hour, "", false},
		{-5 * time.Minute, "", false},
	}
	for _, c := range cases {
		got, ok := humanWait(c.in)
		if got != c.want || ok != c.ok {
			t.Errorf("humanWait(%v) = %q, %v; want %q, %v", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestOtherFailuresKeepTheirStatus(t *testing.T) {
	// Only rate limiting gets the special explanation; anything else is more
	// useful reported as it came back.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)

	u, _ := New(Config{
		BundlePath:     filepath.Join(t.TempDir(), "Lasso.app"),
		CurrentVersion: "0.1.0",
		ReleaseAPI:     server.URL,
	})

	_, err := u.Check(context.Background())
	if err == nil || !strings.Contains(err.Error(), "500") {
		t.Errorf("error = %v, want the status reported", err)
	}
}

func TestCheckIdentifiesItself(t *testing.T) {
	// GitHub asks every caller to say who it is.
	var agent string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		agent = r.Header.Get("User-Agent")
		json.NewEncoder(w).Encode(map[string]any{"tag_name": "v0.1.0"})
	}))
	t.Cleanup(server.Close)

	u, _ := New(Config{
		BundlePath:     filepath.Join(t.TempDir(), "Lasso.app"),
		CurrentVersion: "0.1.0",
		ReleaseAPI:     server.URL,
	})
	u.Check(context.Background())

	if !strings.HasPrefix(agent, "Lasso/") {
		t.Errorf("User-Agent = %q, want Lasso to identify itself", agent)
	}
}

// --- Staying inside GitHub's allowance -------------------------------------
//
// Exceeding the hourly allowance only earns a 403. What gets a caller blocked
// is what it does next, so these cover the "next": asking again immediately,
// retrying a failure in a loop, and ignoring a Retry-After.

// countingAPI serves a release and counts how many requests actually arrive.
type countingAPI struct {
	mu       sync.Mutex
	requests int
	etags    []string
	handler  func(w http.ResponseWriter, r *http.Request, n int)
}

func (c *countingAPI) start(t *testing.T) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.mu.Lock()
		c.requests++
		n := c.requests
		c.etags = append(c.etags, r.Header.Get("If-None-Match"))
		c.mu.Unlock()
		c.handler(w, r, n)
	}))
	t.Cleanup(server.Close)
	return server.URL
}

func (c *countingAPI) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.requests
}

func releaseJSON(w http.ResponseWriter) {
	fmt.Fprint(w, `{"tag_name":"v9.9.9","body":"notes","html_url":"https://example.com",
		"assets":[{"name":"Lasso-app.zip","browser_download_url":"https://example.com/a.zip","size":5}]}`)
}

func TestARefusalStopsTheNextRequestLeaving(t *testing.T) {
	// The one that matters. Being told "0 remaining" and asking again anyway
	// is the behaviour that turns a throttle into a block, so the second
	// check must not reach the network at all.
	api := &countingAPI{handler: func(w http.ResponseWriter, _ *http.Request, _ int) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("X-RateLimit-Reset", fmt.Sprint(time.Now().Add(30*time.Minute).Unix()))
		w.WriteHeader(http.StatusForbidden)
	}}
	u, _ := New(Config{
		BundlePath:     filepath.Join(t.TempDir(), "Lasso.app"),
		CurrentVersion: "0.1.0",
		ReleaseAPI:     api.start(t),
	})

	if _, err := u.Check(context.Background()); err == nil {
		t.Fatal("a rate-limited check reported success")
	}
	// Someone leaning on "Check again".
	for i := 0; i < 5; i++ {
		_, err := u.Check(context.Background())
		if err == nil {
			t.Fatal("a check during the rate-limit window reported success")
		}
		if !strings.Contains(err.Error(), "rate-limiting") {
			t.Errorf("error = %v, want the rate-limit explanation repeated from memory", err)
		}
	}
	if got := api.count(); got != 1 {
		t.Errorf("%d requests reached GitHub; want 1, the rest refused locally", got)
	}
}

func TestRetryAfterIsObeyed(t *testing.T) {
	// A secondary rate limit. GitHub says how long to wait and this is the
	// signal it escalates on, so the wait is taken literally.
	api := &countingAPI{handler: func(w http.ResponseWriter, _ *http.Request, _ int) {
		w.Header().Set("Retry-After", "120")
		w.WriteHeader(http.StatusForbidden)
	}}
	u, _ := New(Config{
		BundlePath:     filepath.Join(t.TempDir(), "Lasso.app"),
		CurrentVersion: "0.1.0",
		ReleaseAPI:     api.start(t),
	})

	_, err := u.Check(context.Background())
	if err == nil || !strings.Contains(err.Error(), "slow down") {
		t.Fatalf("error = %v, want it to name the slow-down", err)
	}
	if _, err := u.Check(context.Background()); err == nil {
		t.Fatal("a check inside the Retry-After window reported success")
	}
	if got := api.count(); got != 1 {
		t.Errorf("%d requests reached GitHub; want 1 — Retry-After was ignored", got)
	}
}

func TestRepeatedChecksAskConditionallyAndReuseTheAnswer(t *testing.T) {
	// A 304 costs nothing against the allowance, so the usual case — nothing
	// new since last time — should cost nothing.
	api := &countingAPI{handler: func(w http.ResponseWriter, r *http.Request, n int) {
		w.Header().Set("ETag", `W/"abc"`)
		if n > 1 {
			if r.Header.Get("If-None-Match") != `W/"abc"` {
				t.Errorf("request %d did not ask conditionally", n)
			}
			w.WriteHeader(http.StatusNotModified)
			return
		}
		releaseJSON(w)
	}}
	u, _ := New(Config{
		BundlePath:     filepath.Join(t.TempDir(), "Lasso.app"),
		CurrentVersion: "0.1.0",
		ReleaseAPI:     api.start(t),
	})

	first, err := u.Check(context.Background())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if !first.Available || first.Version != "9.9.9" {
		t.Fatalf("first check = %+v, want 9.9.9 available", first)
	}

	// Inside the spacing window: served from memory, nothing sent.
	again, err := u.Check(context.Background())
	if err != nil {
		t.Fatalf("second Check: %v", err)
	}
	if again.Version != first.Version || !again.Available {
		t.Errorf("second check = %+v, want the same answer as the first", again)
	}
	if got := api.count(); got != 1 {
		t.Errorf("%d requests for two checks; want 1", got)
	}

	// Past the spacing window it asks again — conditionally, and the 304
	// still yields the full answer.
	u.lim.next = time.Now().Add(-time.Second)
	third, err := u.Check(context.Background())
	if err != nil {
		t.Fatalf("third Check: %v", err)
	}
	if third.Version != "9.9.9" || !third.Available {
		t.Errorf("third check = %+v, want the cached release returned for the 304", third)
	}
	if got := api.count(); got != 2 {
		t.Errorf("%d requests; want 2", got)
	}
}

func TestAnUnreachableServerBacksOff(t *testing.T) {
	// A wrong URL or a dead network must not mean a fresh request every time
	// the settings screen opens.
	u, _ := New(Config{
		BundlePath:     filepath.Join(t.TempDir(), "Lasso.app"),
		CurrentVersion: "0.1.0",
		// Reserved by RFC 6761 to never resolve.
		ReleaseAPI: "http://update.invalid/releases/latest",
		HTTP:       &http.Client{Timeout: 2 * time.Second},
	})

	if _, err := u.Check(context.Background()); err == nil {
		t.Fatal("a check against an unreachable server reported success")
	}
	wait := time.Until(u.lim.next)
	if wait < backoffBase-time.Second {
		t.Errorf("next attempt allowed in %v; want at least %v", wait, backoffBase)
	}
}

func TestChecksDoNotRunConcurrently(t *testing.T) {
	// GitHub asks for serial requests, and two racing checks would each
	// spend allowance the other had not accounted for.
	var inFlight, overlapped int32
	api := &countingAPI{handler: func(w http.ResponseWriter, _ *http.Request, _ int) {
		if atomic.AddInt32(&inFlight, 1) > 1 {
			atomic.StoreInt32(&overlapped, 1)
		}
		time.Sleep(20 * time.Millisecond)
		atomic.AddInt32(&inFlight, -1)
		releaseJSON(w)
	}}
	u, _ := New(Config{
		BundlePath:     filepath.Join(t.TempDir(), "Lasso.app"),
		CurrentVersion: "0.1.0",
		ReleaseAPI:     api.start(t),
	})

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); u.Check(context.Background()) }()
	}
	wg.Wait()

	if atomic.LoadInt32(&overlapped) == 1 {
		t.Error("two checks were in flight at once")
	}
	if got := api.count(); got != 1 {
		t.Errorf("%d requests for 8 concurrent checks; want 1", got)
	}
}
