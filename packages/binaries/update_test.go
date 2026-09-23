package binaries

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// buildReleaseZip produces an archive shaped like yt-dlp's onedir release: a
// launcher beside an _internal payload.
func buildReleaseZip(t *testing.T, version string) []byte {
	t.Helper()

	var buf bytes.Buffer
	w := zip.NewWriter(&buf)

	launcher, err := w.CreateHeader(&zip.FileHeader{Name: "yt-dlp_macos", Method: zip.Deflate})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := launcher.Write([]byte("#!/bin/sh\necho \"" + version + "\"\n")); err != nil {
		t.Fatal(err)
	}

	payload, err := w.Create("_internal/base_library.zip")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := payload.Write([]byte("payload-" + version)); err != nil {
		t.Fatal(err)
	}

	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func digestOf(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// releaseServer stands in for the GitHub release endpoints.
type releaseServer struct {
	*httptest.Server
	hits map[string]int
}

func newReleaseServer(t *testing.T, version string, archive []byte, digest string) *releaseServer {
	t.Helper()
	rs := &releaseServer{hits: map[string]int{}}

	// Laid out like github.com: the latest-release page redirects to the tag,
	// and the tag's files sit under /releases/download/<tag>/. No API.
	mux := http.NewServeMux()
	mux.HandleFunc("/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		rs.hits["latest"]++
		if strings.HasPrefix(r.URL.Path, "/api") {
			t.Error("the updater called an API endpoint")
		}
		http.Redirect(w, r, "/releases/tag/"+version, http.StatusFound)
	})
	mux.HandleFunc("/releases/download/"+version+"/"+ytDlpAsset, func(w http.ResponseWriter, _ *http.Request) {
		rs.hits["zip"]++
		w.Write(archive)
	})
	mux.HandleFunc("/releases/download/"+version+"/"+sumsAsset, func(w http.ResponseWriter, _ *http.Request) {
		rs.hits["sums"]++
		w.Write([]byte("0000  some-other-file\n" + digest + "  " + ytDlpAsset + "\n"))
	})

	rs.Server = httptest.NewServer(mux)
	t.Cleanup(rs.Close)
	return rs
}

// updatableManager returns an installed manager pointed at a local release.
func updatableManager(t *testing.T, server *releaseServer) *Manager {
	t.Helper()
	m, _ := newFakeManager(t)
	if _, err := m.Install(context.Background()); err != nil {
		t.Fatalf("Install: %v", err)
	}
	m.releasesURL = server.URL + "/releases"
	return m
}

func TestUpdateInstallsNewRelease(t *testing.T) {
	const newVersion = "2026.12.01"
	archive := buildReleaseZip(t, newVersion)
	server := newReleaseServer(t, newVersion, archive, digestOf(archive))
	m := updatableManager(t, server)

	result, err := m.UpdateYtDlp(context.Background(), nil)
	if err != nil {
		t.Fatalf("UpdateYtDlp: %v", err)
	}

	if result.VersionBefore != fakeVersions[YtDlp] {
		t.Errorf("VersionBefore = %q", result.VersionBefore)
	}
	if result.VersionAfter != newVersion {
		t.Errorf("VersionAfter = %q, want %q", result.VersionAfter, newVersion)
	}
	if !result.Updated {
		t.Error("Updated = false after a version change")
	}
	if !strings.Contains(result.Output, "Checksum verified") {
		t.Errorf("Output does not record the checksum check:\n%s", result.Output)
	}

	// The whole folder is replaced, not just the launcher.
	payload, err := os.ReadFile(filepath.Join(m.paths.Bin, "yt-dlp", "_internal", "base_library.zip"))
	if err != nil {
		t.Fatalf("payload missing after update: %v", err)
	}
	if string(payload) != "payload-"+newVersion {
		t.Errorf("payload = %q, want the new one", payload)
	}
	mustBeExecutable(t, m.Path(YtDlp))
}

func TestUpdateRecordsNewVersionInStamp(t *testing.T) {
	const newVersion = "2026.12.01"
	archive := buildReleaseZip(t, newVersion)
	server := newReleaseServer(t, newVersion, archive, digestOf(archive))
	m := updatableManager(t, server)

	if _, err := m.UpdateYtDlp(context.Background(), nil); err != nil {
		t.Fatalf("UpdateYtDlp: %v", err)
	}

	// Without this, the next launch would treat the update as stale and put
	// the older bundled copy back.
	if got := readStamp(m.paths.Bin).Binaries[YtDlp]; got != newVersion {
		t.Errorf("stamp = %q, want %q", got, newVersion)
	}

	report, err := m.Install(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range report.Installed {
		if name == YtDlp {
			t.Error("a relaunch overwrote the updated yt-dlp")
		}
	}
}

func TestUpdateSkipsWhenAlreadyCurrent(t *testing.T) {
	current := fakeVersions[YtDlp]
	archive := buildReleaseZip(t, current)
	server := newReleaseServer(t, current, archive, digestOf(archive))
	m := updatableManager(t, server)

	result, err := m.UpdateYtDlp(context.Background(), nil)
	if err != nil {
		t.Fatalf("UpdateYtDlp: %v", err)
	}
	if result.Updated {
		t.Error("Updated = true when the version had not changed")
	}
	if server.hits["zip"] != 0 {
		t.Error("downloaded an archive despite already being current")
	}
	if !strings.Contains(result.UserMessage(), "already up to date") {
		t.Errorf("UserMessage = %q", result.UserMessage())
	}
}

// TestUpdateRejectsWrongChecksum is the guarantee that matters most: bytes
// that are not the published ones never get installed.
func TestUpdateRejectsWrongChecksum(t *testing.T) {
	archive := buildReleaseZip(t, "2026.12.01")
	tampered := append([]byte{}, archive...)
	tampered[len(tampered)/2] ^= 0xff

	// The server publishes the digest of the original but serves the tampered
	// bytes, which is what a compromised mirror would look like.
	server := newReleaseServer(t, "2026.12.01", tampered, digestOf(archive))
	m := updatableManager(t, server)

	_, err := m.UpdateYtDlp(context.Background(), nil)
	if err == nil {
		t.Fatal("UpdateYtDlp installed an archive that failed its checksum")
	}
	if !strings.Contains(err.Error(), "checksum") {
		t.Errorf("error = %q, want it to name the checksum", err)
	}

	// The working installation must be untouched.
	out, _ := os.ReadFile(m.Path(YtDlp))
	if !strings.Contains(string(out), fakeVersions[YtDlp]) {
		t.Error("a rejected update damaged the existing installation")
	}
}

func TestUpdateKeepsWorkingCopyWhenNewOneWillNotRun(t *testing.T) {
	// An archive whose launcher exits non-zero: staged, verified, rejected.
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	f, _ := w.CreateHeader(&zip.FileHeader{Name: "yt-dlp_macos", Method: zip.Deflate})
	f.Write([]byte("#!/bin/sh\nexit 1\n"))
	w.Close()
	archive := buf.Bytes()

	server := newReleaseServer(t, "2026.12.01", archive, digestOf(archive))
	m := updatableManager(t, server)

	if _, err := m.UpdateYtDlp(context.Background(), nil); err == nil {
		t.Fatal("UpdateYtDlp installed a binary that would not run")
	}

	out, _ := os.ReadFile(m.Path(YtDlp))
	if !strings.Contains(string(out), fakeVersions[YtDlp]) {
		t.Error("the working copy was replaced by one that does not run")
	}
	if v := m.verifyOne(context.Background(), YtDlp); !v.OK {
		t.Errorf("yt-dlp no longer runs after a failed update: %v", v.Err)
	}
}

func TestUpdateReportsMissingAsset(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/releases/tag/2026.12.01", http.StatusFound)
	})
	mux.HandleFunc("/releases/download/2026.12.01/"+sumsAsset, func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(strings.Repeat("a", 64) + "  " + ytDlpAsset + "\n"))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	m, _ := newFakeManager(t)
	if _, err := m.Install(context.Background()); err != nil {
		t.Fatal(err)
	}
	m.releasesURL = server.URL + "/releases"

	_, err := m.UpdateYtDlp(context.Background(), nil)
	if err == nil || !strings.Contains(err.Error(), ytDlpAsset) {
		t.Errorf("error = %v, want it to name the missing asset", err)
	}
}

func TestUpdateReportsUnreachableServer(t *testing.T) {
	m, _ := newFakeManager(t)
	if _, err := m.Install(context.Background()); err != nil {
		t.Fatal(err)
	}
	m.releasesURL = "http://127.0.0.1:1/releases"

	if _, err := m.UpdateYtDlp(context.Background(), nil); err == nil {
		t.Fatal("UpdateYtDlp succeeded with no server")
	}
}

func TestDigestForParsing(t *testing.T) {
	body := []byte("aaa  yt-dlp\nbbb  " + ytDlpAsset + "\nccc  yt-dlp.exe\n")

	got, err := digestFor(body, ytDlpAsset)
	if err != nil {
		t.Fatal(err)
	}
	if got != "bbb" {
		t.Errorf("digest = %q, want the line for %s", got, ytDlpAsset)
	}
	if _, err := digestFor(body, "absent.zip"); err == nil {
		t.Error("digestFor accepted a name that is not listed")
	}
}

func TestUpdateNeverTouchesTheAPIAndReadsNothingWhenCurrent(t *testing.T) {
	// The check that finds nothing new is the common one, so it has to be the
	// cheapest: one redirect read, no checksum list, no archive.
	server := newReleaseServer(t, fakeVersions[YtDlp], nil, "")
	m := updatableManager(t, server)

	if _, err := m.UpdateYtDlp(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if server.hits["latest"] != 1 || server.hits["sums"] != 0 || server.hits["zip"] != 0 {
		t.Errorf("hits = %v, want only the one redirect read", server.hits)
	}
}

func TestSwapDirRollsBackOnFailure(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "installed")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "marker"), []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Staging a path that does not exist makes the rename fail.
	if err := swapDir(filepath.Join(root, "missing"), target); err == nil {
		t.Fatal("swapDir succeeded with no staged directory")
	}

	got, err := os.ReadFile(filepath.Join(target, "marker"))
	if err != nil || string(got) != "original" {
		t.Errorf("the original was not restored: %q, %v", got, err)
	}
}
