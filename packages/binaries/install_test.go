package binaries

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestInstallPlacesEveryBinary(t *testing.T) {
	m, paths := newFakeManager(t)

	report, err := m.Install(context.Background())
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if len(report.Installed) != len(requiredBinaries) {
		t.Fatalf("installed %v, want all of %v", report.Installed, requiredBinaries)
	}
	if !report.Changed() {
		t.Error("Changed() = false after a first install")
	}

	for _, name := range requiredBinaries {
		path := m.Path(name)
		if !strings.HasPrefix(path, paths.Bin) {
			t.Errorf("%s resolves to %s, outside the install dir", name, path)
		}
		mustBeExecutable(t, path)
	}
}

func TestInstallCopiesOnedirPayload(t *testing.T) {
	m, paths := newFakeManager(t)
	if _, err := m.Install(context.Background()); err != nil {
		t.Fatalf("Install: %v", err)
	}

	// The launcher is useless without _internal; the whole tree must come over.
	payload := filepath.Join(paths.Bin, "yt-dlp", "_internal", "base_library.zip")
	got, err := os.ReadFile(payload)
	if err != nil {
		t.Fatalf("onedir payload missing: %v", err)
	}
	if string(got) != "payload" {
		t.Errorf("payload = %q, want %q", got, "payload")
	}
}

func TestInstallIsIdempotent(t *testing.T) {
	m, _ := newFakeManager(t)
	ctx := context.Background()

	if _, err := m.Install(ctx); err != nil {
		t.Fatalf("first Install: %v", err)
	}
	report, err := m.Install(ctx)
	if err != nil {
		t.Fatalf("second Install: %v", err)
	}
	if len(report.Installed) != 0 {
		t.Errorf("second install copied %v, want nothing", report.Installed)
	}
	if len(report.Skipped) != len(requiredBinaries) {
		t.Errorf("skipped %v, want all of %v", report.Skipped, requiredBinaries)
	}
	if report.Changed() {
		t.Error("Changed() = true when nothing was copied")
	}
}

func TestInstallRefreshesOnVersionBump(t *testing.T) {
	m, paths := newFakeManager(t)
	ctx := context.Background()
	if _, err := m.Install(ctx); err != nil {
		t.Fatalf("Install: %v", err)
	}

	// Rebuild the source at a newer yt-dlp, as an app update would ship.
	bumped := map[Name]string{}
	for k, v := range fakeVersions {
		bumped[k] = v
	}
	bumped[YtDlp] = "2026.09.01"
	paths.Source = newFakeSource(t, bumped)

	m2, err := New(paths)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	report, err := m2.Install(ctx)
	if err != nil {
		t.Fatalf("Install after bump: %v", err)
	}

	if !slices.Contains(report.Installed, YtDlp) {
		t.Errorf("installed %v, want yt-dlp refreshed", report.Installed)
	}
	if slices.Contains(report.Installed, FFmpeg) {
		t.Errorf("ffmpeg was reinstalled but its version did not change")
	}

	out, err := os.ReadFile(m2.Path(YtDlp))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "2026.09.01") {
		t.Error("yt-dlp on disk is still the old version")
	}
}

func TestInstallRepairsMissingBinary(t *testing.T) {
	m, _ := newFakeManager(t)
	ctx := context.Background()
	if _, err := m.Install(ctx); err != nil {
		t.Fatalf("Install: %v", err)
	}

	// Something deleted ffmpeg out from under us; the stamp still claims it.
	if err := os.Remove(m.Path(FFmpeg)); err != nil {
		t.Fatal(err)
	}
	report, err := m.Install(ctx)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if !slices.Contains(report.Installed, FFmpeg) {
		t.Errorf("installed %v, want ffmpeg restored", report.Installed)
	}
	mustBeExecutable(t, m.Path(FFmpeg))
}

func TestInstallFailsClearlyWhenSourceIncomplete(t *testing.T) {
	support := t.TempDir()
	source := newFakeSource(t, fakeVersions)
	if err := os.Remove(filepath.Join(source, "deno")); err != nil {
		t.Fatal(err)
	}

	m, err := New(Paths{Support: support, Bin: filepath.Join(support, "bin"), Source: source})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_, err = m.Install(context.Background())
	if err == nil {
		t.Fatal("Install succeeded with a missing binary")
	}
	if !strings.Contains(err.Error(), "deno") {
		t.Errorf("error %q does not name the missing binary", err)
	}
}

func TestInstallMakesNonExecutableSourceRunnable(t *testing.T) {
	support := t.TempDir()
	source := newFakeSource(t, fakeVersions)
	// A source file that lost its executable bit, e.g. via a zip round-trip.
	if err := os.Chmod(filepath.Join(source, "ffprobe"), 0o644); err != nil {
		t.Fatal(err)
	}

	m, err := New(Paths{Support: support, Bin: filepath.Join(support, "bin"), Source: source})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Install(context.Background()); err != nil {
		t.Fatalf("Install: %v", err)
	}
	mustBeExecutable(t, m.Path(FFprobe))
}

func TestCopyTreeLeavesNoStagingDirectory(t *testing.T) {
	m, paths := newFakeManager(t)
	if _, err := m.Install(context.Background()); err != nil {
		t.Fatalf("Install: %v", err)
	}

	entries, err := os.ReadDir(paths.Bin)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".incoming") {
			t.Errorf("staging directory %s was left behind", e.Name())
		}
	}
}

func TestIsMachO(t *testing.T) {
	dir := t.TempDir()

	script := filepath.Join(dir, "script.sh")
	writeScript(t, script, "hello")
	if got, err := isMachO(script); err != nil || got {
		t.Errorf("isMachO(shell script) = %v, %v; want false, nil", got, err)
	}

	// 64-bit little-endian Mach-O magic.
	macho := filepath.Join(dir, "fake-macho")
	if err := os.WriteFile(macho, []byte{0xcf, 0xfa, 0xed, 0xfe, 0x00}, 0o755); err != nil {
		t.Fatal(err)
	}
	if got, err := isMachO(macho); err != nil || !got {
		t.Errorf("isMachO(mach-o) = %v, %v; want true, nil", got, err)
	}

	tiny := filepath.Join(dir, "tiny")
	if err := os.WriteFile(tiny, []byte{0x01}, 0o755); err != nil {
		t.Fatal(err)
	}
	if got, err := isMachO(tiny); err != nil || got {
		t.Errorf("isMachO(1 byte) = %v, %v; want false, nil", got, err)
	}
}
