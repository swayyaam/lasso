package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/swayyaam/lasso/packages/updater"
)

func TestWritesAManifestTheUpdaterAccepts(t *testing.T) {
	dir := t.TempDir()
	app := filepath.Join(dir, updater.AppAsset)
	os.WriteFile(app, []byte("zip bytes"), 0o644)
	notes := filepath.Join(dir, "notes.md")
	os.WriteFile(notes, []byte("\n## What changed\nThings.\n"), 0o644)
	out := filepath.Join(dir, updater.ManifestName)

	if err := run("v0.1.6", notes, out, []string{app}); err != nil {
		t.Fatalf("run: %v", err)
	}

	raw, _ := os.ReadFile(out)
	var m updater.Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("the manifest is not JSON the updater can read: %v", err)
	}
	if err := m.Validate(); err != nil {
		t.Errorf("the updater would refuse this manifest: %v", err)
	}
	if m.Version != "0.1.6" {
		t.Errorf("Version = %q, want the leading v dropped", m.Version)
	}
	if got := m.Assets[updater.AppAsset]; got.Size != 9 || len(got.SHA256) != 64 {
		t.Errorf("app asset = %+v, want its real size and digest", got)
	}
	if !strings.HasPrefix(m.Notes, "## What changed") {
		t.Errorf("Notes = %q, want them trimmed", m.Notes)
	}
}

func TestRefusesAReleaseTheAppCouldNotInstall(t *testing.T) {
	dir := t.TempDir()
	notes := filepath.Join(dir, "notes.md")
	os.WriteFile(notes, []byte("notes"), 0o644)
	dmg := filepath.Join(dir, "Lasso.dmg")
	os.WriteFile(dmg, []byte("dmg"), 0o644)

	if err := run("0.1.6", notes, filepath.Join(dir, "latest.json"), []string{dmg}); err == nil {
		t.Error("a manifest without the app archive was written")
	}
	empty := filepath.Join(dir, "empty.md")
	os.WriteFile(empty, []byte("  \n"), 0o644)
	app := filepath.Join(dir, updater.AppAsset)
	os.WriteFile(app, []byte("zip"), 0o644)
	if err := run("0.1.6", empty, filepath.Join(dir, "latest.json"), []string{app}); err == nil {
		t.Error("a manifest with empty notes was written")
	}
	if err := run("0.1.6/../x", notes, filepath.Join(dir, "latest.json"), []string{app}); err == nil {
		t.Error("a manifest with an unusable version was written")
	}
}
