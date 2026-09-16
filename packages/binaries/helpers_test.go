package binaries

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// fakeVersions is the version set the test source directory reports.
var fakeVersions = map[Name]string{
	YtDlp:   "2026.08.19",
	FFmpeg:  "9.0.1",
	FFprobe: "9.0.1",
	Deno:    "2.9.6",
}

// writeScript creates an executable shell script that prints line and exits 0.
func writeScript(t *testing.T, path, line string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "#!/bin/sh\necho \"" + line + "\"\n"
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
}

// newFakeSource builds a directory that looks like a fetched sidecar set:
// three single-file binaries plus yt-dlp's onedir layout, and a manifest.
func newFakeSource(t *testing.T, versions map[Name]string) string {
	t.Helper()
	dir := t.TempDir()

	specs := map[Name]Spec{
		YtDlp:   {Version: versions[YtDlp], Layout: LayoutDir, Entrypoint: "yt-dlp_macos"},
		FFmpeg:  {Version: versions[FFmpeg], Layout: LayoutFile, Entrypoint: "ffmpeg"},
		FFprobe: {Version: versions[FFprobe], Layout: LayoutFile, Entrypoint: "ffprobe"},
		Deno:    {Version: versions[Deno], Layout: LayoutFile, Entrypoint: "deno"},
	}

	writeScript(t, filepath.Join(dir, "ffmpeg"), "ffmpeg version "+versions[FFmpeg])
	writeScript(t, filepath.Join(dir, "ffprobe"), "ffprobe version "+versions[FFprobe])
	writeScript(t, filepath.Join(dir, "deno"), "deno "+versions[Deno])

	// yt-dlp's onedir build: a launcher next to an _internal tree that must be
	// copied along with it.
	writeScript(t, filepath.Join(dir, "yt-dlp", "yt-dlp_macos"), versions[YtDlp])
	if err := os.MkdirAll(filepath.Join(dir, "yt-dlp", "_internal"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "yt-dlp", "_internal", "base_library.zip"), []byte("payload"), 0o644); err != nil {
		t.Fatal(err)
	}

	writeManifest(t, dir, specs)
	return dir
}

func writeManifest(t *testing.T, dir string, specs map[Name]Spec) {
	t.Helper()
	raw, err := json.MarshalIndent(Manifest{
		SchemaVersion: 1,
		Platform:      "darwin-arm64",
		Binaries:      specs,
	}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, manifestName), raw, 0o644); err != nil {
		t.Fatal(err)
	}
}

// newFakeManager wires a fake source to a throwaway install directory.
func newFakeManager(t *testing.T) (*Manager, Paths) {
	t.Helper()
	support := t.TempDir()
	paths := Paths{
		Support: support,
		Bin:     filepath.Join(support, "bin"),
		Source:  newFakeSource(t, fakeVersions),
	}
	m, err := New(paths)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return m, paths
}

func mustBeExecutable(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Errorf("%s is not executable (mode %v)", path, info.Mode().Perm())
	}
}
