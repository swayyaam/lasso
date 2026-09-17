package binaries

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestRealBinariesInstallAndRun exercises the whole startup path against the
// binaries `make setup` actually fetched — real Mach-O images, real quarantine
// and signature handling, real Gatekeeper warm-up.
//
// It is skipped unless LASSO_INTEGRATION is set, because it needs those
// binaries present and copies ~330 MB.
//
//	LASSO_INTEGRATION=1 go test ./... -run Real -v
func TestRealBinariesInstallAndRun(t *testing.T) {
	if os.Getenv("LASSO_INTEGRATION") == "" {
		t.Skip("set LASSO_INTEGRATION=1 to run against the fetched binaries")
	}

	source := repoBinDir(t)
	if _, err := os.Stat(filepath.Join(source, manifestName)); err != nil {
		t.Skipf("no fetched binaries at %s — run `make setup`", source)
	}

	t.Setenv(SourceEnv, source)
	t.Setenv(SupportEnv, t.TempDir())

	m, status, err := Startup(context.Background())
	if err != nil {
		t.Fatalf("Startup: %v", err)
	}
	if !status.Ready {
		for _, p := range status.Problems {
			t.Errorf("%s: %s\n%s", p.Name, p.Message, p.Detail)
		}
		t.Fatal("sidecar binaries are not ready")
	}

	for _, name := range requiredBinaries {
		t.Logf("%-8s %s", name, status.Versions[name])
	}

	// Second launch must skip the copy entirely and still verify clean.
	_, again, err := Startup(context.Background())
	if err != nil {
		t.Fatalf("second Startup: %v", err)
	}
	if again.FirstRun {
		t.Error("FirstRun = true on a second launch")
	}
	if !again.Ready {
		t.Errorf("not ready on second launch: %+v", again.Problems)
	}

	// The warm path is what every download pays.
	for _, r := range m.Verify(context.Background()) {
		t.Logf("warm %-8s %v", r.Name, r.Duration.Round(1e6))
	}
}

func repoBinDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate the test file")
	}
	repo := filepath.Join(filepath.Dir(file), "..", "..")
	return filepath.Clean(filepath.Join(repo, "apps", "desktop", "build", "bin", "darwin-"+runtime.GOARCH))
}

// TestRealYtDlpUpdate performs an actual update against the real yt-dlp
// release server: it installs an older release, runs the updater, and checks
// that the newer one is in place and runs.
//
// This is the test that matters for the updater, because yt-dlp's own -U
// refuses the onedir build ("Auto-update is not supported for unpackaged
// executables"), which is why Lasso does the update itself.
//
//	LASSO_INTEGRATION=1 go test ./... -run RealYtDlpUpdate -v
func TestRealYtDlpUpdate(t *testing.T) {
	if os.Getenv("LASSO_INTEGRATION") == "" {
		t.Skip("set LASSO_INTEGRATION=1 to update against the real release server")
	}

	const oldVersion = "2026.07.04"
	const oldURL = "https://github.com/yt-dlp/yt-dlp/releases/download/" + oldVersion + "/yt-dlp_macos.zip"

	source := repoBinDir(t)
	support := t.TempDir()
	t.Setenv(SourceEnv, source)
	t.Setenv(SupportEnv, support)

	m, err := Discover()
	if err != nil {
		t.Skipf("no fetched binaries: %v", err)
	}

	// Put the older release in place by hand, standing in for an install that
	// has fallen behind.
	binDir := filepath.Join(support, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(t.TempDir(), "old.zip")
	if err := m.download(context.Background(), oldURL, archive, maxDownloadBytes); err != nil {
		t.Fatalf("fetching the old release: %v", err)
	}
	if err := unzip(context.Background(), archive, filepath.Join(binDir, "yt-dlp")); err != nil {
		t.Fatal(err)
	}
	if _, err := m.fixupTree(context.Background(), filepath.Join(binDir, "yt-dlp"), "yt-dlp_macos"); err != nil {
		t.Fatal(err)
	}

	before := m.verifyOne(context.Background(), YtDlp)
	if before.Err != nil {
		t.Fatalf("the old release does not run: %v", before.Err)
	}
	if before.Version != oldVersion {
		t.Fatalf("staged version = %q, want %q", before.Version, oldVersion)
	}
	t.Logf("installed the old release: %s", before.Version)

	result, err := m.UpdateYtDlp(context.Background())
	if err != nil {
		t.Fatalf("UpdateYtDlp: %v\n%s", err, result.Output)
	}
	t.Logf("update log:\n%s", result.Output)

	if !result.Updated {
		t.Errorf("Updated = false; before %q after %q", result.VersionBefore, result.VersionAfter)
	}
	if result.VersionAfter == oldVersion {
		t.Errorf("version did not move off %s", oldVersion)
	}

	// The updated copy must actually run, and its payload must be complete.
	after := m.verifyOne(context.Background(), YtDlp)
	if after.Err != nil {
		t.Fatalf("the updated yt-dlp does not run: %v", after.Err)
	}
	t.Logf("updated to %s, runs in %v", after.Version, after.Duration.Round(1e6))

	if _, err := os.Stat(filepath.Join(binDir, "yt-dlp", "_internal")); err != nil {
		t.Errorf("the update left no _internal payload: %v", err)
	}
	if got := readStamp(binDir).Binaries[YtDlp]; got != after.Version {
		t.Errorf("stamp = %q, want %q", got, after.Version)
	}

	// Nothing may be left with a quarantine flag or a signature macOS rejects.
	out, _ := exec.Command("xattr", "-r", filepath.Join(binDir, "yt-dlp")).CombinedOutput()
	if strings.Contains(string(out), "com.apple.quarantine") {
		t.Error("the updated folder still carries a quarantine flag")
	}
	if err := exec.Command("codesign", "--verify", "--no-strict", m.Path(YtDlp)).Run(); err != nil {
		t.Errorf("the updated launcher has a signature macOS rejects: %v", err)
	}
}
