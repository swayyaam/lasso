package binaries

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnvironPutsBinDirFirstOnPath(t *testing.T) {
	m, paths := newFakeManager(t)

	var path string
	for _, kv := range m.Environ() {
		if name, value, ok := strings.Cut(kv, "="); ok && name == "PATH" {
			path = value
		}
	}
	if path == "" {
		t.Fatal("Environ produced no PATH")
	}
	// deno must resolve to the bundled copy, never a system install.
	first, _, _ := strings.Cut(path, string(os.PathListSeparator))
	if first != paths.Bin {
		t.Errorf("PATH starts with %q, want %q", first, paths.Bin)
	}
	if !strings.Contains(path, os.Getenv("PATH")) {
		t.Error("Environ dropped the inherited PATH")
	}
}

func TestEnvironAddsPathWhenUnset(t *testing.T) {
	t.Setenv("PATH", "")
	m, paths := newFakeManager(t)

	var found bool
	for _, kv := range m.Environ() {
		if kv == "PATH="+paths.Bin || strings.HasPrefix(kv, "PATH="+paths.Bin+string(os.PathListSeparator)) {
			found = true
		}
	}
	if !found {
		t.Error("Environ did not set PATH to the install dir")
	}
}

func TestPathAndVersion(t *testing.T) {
	m, paths := newFakeManager(t)

	if got, want := m.Path(YtDlp), filepath.Join(paths.Bin, "yt-dlp", "yt-dlp_macos"); got != want {
		t.Errorf("Path(yt-dlp) = %q, want %q", got, want)
	}
	if got, want := m.Path(FFmpeg), filepath.Join(paths.Bin, "ffmpeg"); got != want {
		t.Errorf("Path(ffmpeg) = %q, want %q", got, want)
	}
	if got := m.Path("nonexistent"); got != "" {
		t.Errorf("Path(unknown) = %q, want empty", got)
	}
	if got := m.Version(YtDlp); got != fakeVersions[YtDlp] {
		t.Errorf("Version(yt-dlp) = %q", got)
	}
}

func TestFFmpegLocationIsTheInstallDir(t *testing.T) {
	m, paths := newFakeManager(t)
	// yt-dlp takes a directory here and finds both ffmpeg and ffprobe in it.
	if got := m.FFmpegLocation(); got != paths.Bin {
		t.Errorf("FFmpegLocation = %q, want %q", got, paths.Bin)
	}
	for _, name := range []Name{FFmpeg, FFprobe} {
		if filepath.Dir(m.Path(name)) != m.FFmpegLocation() {
			t.Errorf("%s does not live in FFmpegLocation", name)
		}
	}
}

func TestNewRejectsSourceWithoutManifest(t *testing.T) {
	support := t.TempDir()
	_, err := New(Paths{Support: support, Bin: filepath.Join(support, "bin"), Source: t.TempDir()})
	if err == nil {
		t.Fatal("New succeeded without a manifest")
	}
	if !strings.Contains(err.Error(), "make setup") {
		t.Errorf("error = %q, want actionable guidance", err)
	}
}
