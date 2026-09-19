package updater

import (
	"context"
	"encoding/json"
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
// real 329 MB bundle with its helper programs in it.
//
// The unit tests fake the toolchain, which is what makes them fast and lets
// them cover the failure paths. What they cannot prove is the part that would
// hurt: that carrying 328 MB of helpers into a bundle signed without them and
// resealing it produces something macOS will actually open. That is exactly
// the mistake that shipped in v0.1.0, so it is worth a test that does it for
// real.
//
// Skipped unless LASSO_INTEGRATION is set, because it copies the bundle twice.
//
//	LASSO_INTEGRATION=1 go test ./... -run RealUpdate -v
func TestRealUpdateReplacesARealBundle(t *testing.T) {
	if os.Getenv("LASSO_INTEGRATION") == "" {
		t.Skip("set LASSO_INTEGRATION=1 to run against the built app")
	}
	if runtime.GOOS != "darwin" {
		t.Skip("macOS only")
	}

	source := builtBundle(t)
	installed := filepath.Join(t.TempDir(), "Lasso.app")
	ditto(t, source, installed)

	current := plistVersion(t, installed)
	next := bumpMinor(t, current)
	t.Logf("installed %s, releasing %s", current, next)

	archive, digest := buildThinRelease(t, source, next)
	server := serveRelease(t, "v"+next, archive, digest)

	u, err := New(Config{
		BundlePath:     installed,
		CurrentVersion: current,
		ReleaseAPI:     server.URL + "/release",
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

	// The helpers came across. They were never in the archive — it is 5 MB.
	for _, name := range []string{"ffmpeg", "ffprobe", "deno"} {
		path := filepath.Join(installed, "Contents", "Resources", "bin", name)
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("helper %s missing after the update: %v", name, err)
		}
		if info.Size() < 1<<20 {
			t.Errorf("helper %s is %d bytes, which is not the real binary", name, info.Size())
		}
	}
	if _, err := os.Stat(filepath.Join(installed, "Contents", "Resources", "bin", "yt-dlp", "yt-dlp_macos")); err != nil {
		t.Errorf("yt-dlp's onedir payload did not survive the update: %v", err)
	}

	// And the whole thing is sealed. Without this the app opens as "damaged",
	// which is the failure this project has already shipped once.
	if out, err := exec.Command("/usr/bin/codesign", "--verify", "--deep", "--strict", installed).CombinedOutput(); err != nil {
		t.Errorf("the updated app does not verify, so macOS would call it damaged: %v\n%s", err, out)
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

// builtBundle finds the app `make build` produced, or skips.
func builtBundle(t *testing.T) string {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Skip("cannot locate the repository")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
	bundle := filepath.Join(root, "apps", "desktop", "build", "bin", "Lasso.app")

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
// version, with Contents/Resources/bin emptied down to its manifest.
//
// This mirrors scripts/make-release-assets.sh. If the two drift, this test
// stops representing what is actually shipped — which is why it asserts the
// archive really is thin.
func buildThinRelease(t *testing.T, source, version string) (archive []byte, digest string) {
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

// serveRelease stands in for GitHub.
func serveRelease(t *testing.T, tag string, archive []byte, digest string) *httptest.Server {
	t.Helper()

	var server *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("/release", func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"tag_name": tag,
			"body":     "integration test release",
			"html_url": "https://example.com/" + tag,
			"assets": []map[string]any{
				{"name": AppAsset, "browser_download_url": server.URL + "/app.zip", "size": len(archive)},
				{"name": SumsAsset, "browser_download_url": server.URL + "/sums", "size": 128},
			},
		})
	})
	mux.HandleFunc("/app.zip", func(w http.ResponseWriter, _ *http.Request) { w.Write(archive) })
	mux.HandleFunc("/sums", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, "%s  %s\n", digest, AppAsset)
	})

	server = httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}
