package binaries

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadManifest(t *testing.T) {
	dir := newFakeSource(t, fakeVersions)
	m, err := ReadManifest(dir)
	if err != nil {
		t.Fatalf("ReadManifest: %v", err)
	}
	if m.Platform != "darwin-arm64" {
		t.Errorf("platform = %q", m.Platform)
	}
	if got := m.Binaries[YtDlp]; got.Layout != LayoutDir || got.Entrypoint != "yt-dlp_macos" {
		t.Errorf("yt-dlp spec = %+v", got)
	}
}

func TestReadManifestRejectsBadInput(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(t *testing.T, dir string)
		wantSub string
	}{
		{
			name:    "no manifest",
			mutate:  func(t *testing.T, dir string) { os.Remove(filepath.Join(dir, manifestName)) },
			wantSub: "make setup",
		},
		{
			name: "not json",
			mutate: func(t *testing.T, dir string) {
				os.WriteFile(filepath.Join(dir, manifestName), []byte("{nope"), 0o644)
			},
			wantSub: "parsing",
		},
		{
			name: "future schema",
			mutate: func(t *testing.T, dir string) {
				os.WriteFile(filepath.Join(dir, manifestName), []byte(`{"schemaVersion":99,"binaries":{}}`), 0o644)
			},
			wantSub: "schemaVersion 99",
		},
		{
			name: "missing binary",
			mutate: func(t *testing.T, dir string) {
				writeManifest(t, dir, map[Name]Spec{
					YtDlp:  {Version: "1", Layout: LayoutDir, Entrypoint: "yt-dlp_macos"},
					FFmpeg: {Version: "1", Layout: LayoutFile, Entrypoint: "ffmpeg"},
				})
			},
			wantSub: "ffprobe",
		},
		{
			name: "unknown layout",
			mutate: func(t *testing.T, dir string) {
				specs := map[Name]Spec{}
				for _, n := range requiredBinaries {
					specs[n] = Spec{Version: "1", Layout: LayoutFile, Entrypoint: string(n)}
				}
				specs[Deno] = Spec{Version: "1", Layout: "symlink", Entrypoint: "deno"}
				writeManifest(t, dir, specs)
			},
			wantSub: `unknown layout "symlink"`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := newFakeSource(t, fakeVersions)
			tc.mutate(t, dir)

			_, err := ReadManifest(dir)
			if err == nil {
				t.Fatal("ReadManifest succeeded, want an error")
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Errorf("error = %q, want it to mention %q", err, tc.wantSub)
			}
		})
	}
}

func TestSpecPaths(t *testing.T) {
	dirSpec := Spec{Layout: LayoutDir, Entrypoint: "yt-dlp_macos"}
	if got := dirSpec.relPath(YtDlp); got != filepath.Join("yt-dlp", "yt-dlp_macos") {
		t.Errorf("relPath = %q", got)
	}
	if got := dirSpec.rootRel(YtDlp); got != "yt-dlp" {
		t.Errorf("rootRel = %q", got)
	}

	fileSpec := Spec{Layout: LayoutFile, Entrypoint: "ffmpeg"}
	if got := fileSpec.relPath(FFmpeg); got != "ffmpeg" {
		t.Errorf("relPath = %q", got)
	}
	if got := fileSpec.rootRel(FFmpeg); got != "ffmpeg" {
		t.Errorf("rootRel = %q", got)
	}
}

func TestStampRoundTrip(t *testing.T) {
	dir := t.TempDir()

	if got := readStamp(dir); len(got.Binaries) != 0 {
		t.Errorf("readStamp on an empty dir = %+v, want empty", got)
	}

	want := stamp{Binaries: map[Name]string{YtDlp: "2026.08.19"}}
	if err := writeStamp(dir, want); err != nil {
		t.Fatal(err)
	}
	if got := readStamp(dir); got.Binaries[YtDlp] != "2026.08.19" {
		t.Errorf("round trip = %+v", got)
	}

	// A corrupt stamp must read as "nothing installed" rather than exploding,
	// so the next launch simply reinstalls.
	if err := os.WriteFile(filepath.Join(dir, stampName), []byte("garbage"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := readStamp(dir); len(got.Binaries) != 0 {
		t.Errorf("corrupt stamp = %+v, want empty", got)
	}
}
