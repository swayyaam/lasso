package updater

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestRealUpdateReplacesARealBundle runs the whole update against the app
// `make build` actually produced — real ditto, real codesign, real xattr, a
// real 280 MB bundle with its helper programs in it.
//
// The unit tests fake the toolchain, which is what makes them fast and lets
// them cover the failure paths. What they cannot prove is the part that would
// hurt: that a thin release, signed as it ships and installed untouched,
// verifies — and, signed with the Lasso certificate, is the same app to
// macOS as the copy it replaced, so the permissions someone gave it survive.
// Up to 0.2.8 every update re-signed ad hoc and lost them.
//
// Both copies are signed by scripts/sign-app.sh, as releases are: with the
// "Lasso Signing" certificate when the keychain (or LASSO_SIGN_KEYCHAIN) has
// one, ad hoc otherwise, in which case the identity check is skipped and
// says so.
//
// Skipped unless LASSO_INTEGRATION is set, because it copies the bundle twice.
//
//	LASSO_INTEGRATION=1 go test ./... -run RealUpdate -v
func TestRealUpdateReplacesARealBundle(t *testing.T) {
	installed, current, certified := installedCopy(t)
	next := bumpMinor(t, current)
	t.Logf("installed %s, releasing %s", current, next)
	before := designated(t, installed)

	archive, digest := buildThinRelease(t, builtBundle(t), next, true)
	server := serveRelease(t, "v"+next, archive, digest)

	u, err := New(Config{
		BundlePath:     installed,
		CurrentVersion: current,
		ReleasesURL:    server.URL + "/releases",
		// The real toolchain. That is the point of this test.
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	result, err := u.Install(context.Background())
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if !result.Installed {
		t.Fatal("nothing was installed")
	}
	t.Logf("updater said:\n%s", result.Output)

	// The app is the new version.
	if got := plistVersion(t, installed); got != next {
		t.Errorf("installed version = %q, want %q", got, next)
	}

	// Installed as it shipped: the manifest and no helpers. Lasso runs the
	// copies in Application Support, which the DMG's app installed.
	entries, err := os.ReadDir(filepath.Join(installed, "Contents", "Resources", "bin"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "manifest.json" {
		t.Errorf("bin holds %d entries, want only the manifest", len(entries))
	}

	// Sealed, or the app opens as "damaged" — the failure this project has
	// already shipped once.
	if out, err := exec.Command("/usr/bin/codesign", "--verify", "--deep", "--strict", installed).CombinedOutput(); err != nil {
		t.Errorf("the updated app does not verify, so macOS would call it damaged: %v\n%s", err, out)
	}

	// The same app to macOS: what a person allowed the old copy, macOS keeps
	// against exactly this requirement, and checks it exactly this way.
	if certified {
		if out, err := exec.Command("/usr/bin/codesign", "--verify", "-R="+before, installed).CombinedOutput(); err != nil {
			t.Errorf("the updated app is a different app to macOS, so it loses its permissions: %v\n%s", err, out)
		}
		if after := designated(t, installed); after != before {
			t.Errorf("identity changed from %s to %s", before, after)
		}
	} else {
		t.Log("no Lasso Signing certificate: signed ad hoc, so identity was not checked")
	}

	// The old version is kept until the app restarts, then cleaned up.
	if _, err := os.Stat(installed + ".old"); err != nil {
		t.Error("the previous version was not kept beside the new one")
	}
	CleanUp(installed)
	if _, err := os.Stat(installed + ".old"); !os.IsNotExist(err) {
		t.Error("CleanUp did not remove the previous version")
	}
}

// TestRealUpdateRefusesAnotherSigner hands a certificate-signed copy an update
// signed ad hoc, with the real codesign deciding. Installed, it would be a new
// app to macOS; it is refused and the working copy stays.
func TestRealUpdateRefusesAnotherSigner(t *testing.T) {
	installed, current, certified := installedCopy(t)
	if !certified {
		t.Skip("needs the Lasso Signing certificate")
	}
	next := bumpMinor(t, current)

	archive, digest := buildThinRelease(t, builtBundle(t), next, false)
	server := serveRelease(t, "v"+next, archive, digest)
	u, err := New(Config{BundlePath: installed, CurrentVersion: current, ReleasesURL: server.URL + "/releases"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if _, err := u.Install(context.Background()); !errors.Is(err, ErrNotSameSigner) {
		t.Fatalf("Install = %v, want ErrNotSameSigner", err)
	}
	if got := plistVersion(t, installed); got != current {
		t.Errorf("installed version = %q, want the working copy left as it was", got)
	}
}

// installedCopy is the built app copied aside and signed the way a release is,
// standing in for a copy installed from the DMG. certified is whether it
// carries a certificate rather than an ad-hoc signature.
func installedCopy(t *testing.T) (path, version string, certified bool) {
	t.Helper()
	if os.Getenv("LASSO_INTEGRATION") == "" {
		t.Skip("set LASSO_INTEGRATION=1 to run against the built app")
	}
	if runtime.GOOS != "darwin" {
		t.Skip("macOS only")
	}
	path = filepath.Join(t.TempDir(), "Lasso.app")
	ditto(t, builtBundle(t), path)
	signLikeARelease(t, path, true)
	return path, plistVersion(t, path), strings.Contains(designated(t, path), "certificate")
}

// signLikeARelease signs a bundle with scripts/sign-app.sh, or ad hoc when
// withIdentity is false.
func signLikeARelease(t *testing.T, bundle string, withIdentity bool) {
	t.Helper()
	if !withIdentity {
		if out, err := exec.Command("/usr/bin/codesign", "--force", "--sign", "-", bundle).CombinedOutput(); err != nil {
			t.Fatalf("signing ad hoc: %v\n%s", err, out)
		}
		return
	}
	cmd := exec.Command(filepath.Join(repoRoot(t), "scripts", "sign-app.sh"), bundle)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("sign-app.sh: %v\n%s", err, out)
	}
}

// designated is a bundle's designated requirement, as macOS records it.
func designated(t *testing.T, bundle string) string {
	t.Helper()
	out, err := exec.Command("/usr/bin/codesign", "-d", "-r-", bundle).CombinedOutput()
	if err != nil {
		t.Fatalf("codesign -d -r- %s: %v\n%s", bundle, err, out)
	}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimPrefix(strings.TrimSpace(line), "# ")
		if requirement, ok := strings.CutPrefix(line, "designated => "); ok {
			return requirement
		}
	}
	t.Fatalf("no designated requirement for %s:\n%s", bundle, out)
	return ""
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Skip("cannot locate the repository")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
}

// builtBundle finds the app `make build` produced, or skips.
func builtBundle(t *testing.T) string {
	t.Helper()

	bundle := filepath.Join(repoRoot(t), "apps", "desktop", "build", "bin", "Lasso.app")

	if _, err := os.Stat(filepath.Join(bundle, "Contents", "MacOS", "Lasso")); err != nil {
		t.Skipf("no built app at %s — run `make build`", bundle)
	}
	return bundle
}

func ditto(t *testing.T, from, to string) {
	t.Helper()
	if out, err := exec.Command("/usr/bin/ditto", from, to).CombinedOutput(); err != nil {
		t.Fatalf("ditto %s -> %s: %v\n%s", from, to, err, out)
	}
}

func plistVersion(t *testing.T, bundle string) string {
	t.Helper()
	out, err := exec.Command("/usr/libexec/PlistBuddy",
		"-c", "Print :CFBundleShortVersionString",
		filepath.Join(bundle, "Contents", "Info.plist"),
	).Output()
	if err != nil {
		t.Fatalf("reading the version of %s: %v", bundle, err)
	}
	return strings.TrimSpace(string(out))
}

func setPlistVersion(t *testing.T, bundle, version string) {
	t.Helper()
	plist := filepath.Join(bundle, "Contents", "Info.plist")
	for _, key := range []string{"CFBundleShortVersionString", "CFBundleVersion"} {
		cmd := exec.Command("/usr/libexec/PlistBuddy", "-c", "Set :"+key+" "+version, plist)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("setting %s: %v\n%s", key, err, out)
		}
	}
}

// bumpMinor returns a version one minor step above the given one.
func bumpMinor(t *testing.T, version string) string {
	t.Helper()
	parts := parseVersion(version)
	if len(parts) < 2 {
		t.Fatalf("cannot bump %q", version)
	}
	return fmt.Sprintf("%d.%d.0", parts[0], parts[1]+1)
}

// buildThinRelease makes the asset a release would publish: the app at a new
// version, with Contents/Resources/bin emptied down to its manifest, signed
// as it is — with the certificate when withIdentity, ad hoc otherwise.
//
// This mirrors scripts/make-release-assets.sh. If the two drift, this test
// stops representing what is actually shipped — which is why it asserts the
// archive really is thin.
func buildThinRelease(t *testing.T, source, version string, withIdentity bool) (archive []byte, digest string) {
	t.Helper()

	work := t.TempDir()
	staged := filepath.Join(work, "Lasso.app")
	ditto(t, source, staged)
	setPlistVersion(t, staged, version)

	binDir := filepath.Join(staged, "Contents", "Resources", "bin")
	manifest, err := os.ReadFile(filepath.Join(binDir, "manifest.json"))
	if err != nil {
		t.Fatalf("reading the manifest: %v", err)
	}
	if err := os.RemoveAll(binDir); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "manifest.json"), manifest, 0o644); err != nil {
		t.Fatal(err)
	}
	signLikeARelease(t, staged, withIdentity)

	zipPath := filepath.Join(work, AppAsset)
	cmd := exec.Command("/usr/bin/ditto", "-c", "-k", "--sequesterRsrc", "--keepParent", staged, zipPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("building the release archive: %v\n%s", err, out)
	}

	archive, err = os.ReadFile(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	// The whole design rests on this being small. A regression that shipped
	// the helpers would still pass every other assertion here.
	if len(archive) > 64<<20 {
		t.Fatalf("the update archive is %d bytes; it is supposed to exclude the helper programs", len(archive))
	}
	t.Logf("update archive is %.1f MB", float64(len(archive))/(1<<20))

	return archive, digestOf(archive)
}

// serveRelease stands in for GitHub, laid out the way it serves releases:
// the manifest at /releases/latest/download/latest.json and the archive at
// /releases/download/v<version>/Lasso-app.zip. No API.
func serveRelease(t *testing.T, tag string, archive []byte, digest string) *httptest.Server {
	t.Helper()
	version := strings.TrimPrefix(tag, "v")

	mux := http.NewServeMux()
	mux.HandleFunc("/releases/latest/download/"+ManifestName, func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(Manifest{
			SchemaVersion: ManifestSchema,
			Version:       version,
			Published:     "2026-09-23T00:00:00Z",
			Notes:         "integration test release",
			Assets:        map[string]AssetInfo{AppAsset: {Size: int64(len(archive)), SHA256: digest}},
		})
	})
	mux.HandleFunc("/releases/download/"+tag+"/"+AppAsset, func(w http.ResponseWriter, _ *http.Request) { w.Write(archive) })

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}
