package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/swayyaam/lasso/packages/core"
)

func TestDefaultSettings(t *testing.T) {
	s := defaultSettings()

	if s.Concurrency != core.DefaultConcurrency {
		t.Errorf("Concurrency = %d, want %d", s.Concurrency, core.DefaultConcurrency)
	}
	if s.Cookies != core.BrowserNone {
		t.Errorf("Cookies = %q, want none by default", s.Cookies)
	}
	if !strings.HasSuffix(s.DownloadFolder, "Downloads") {
		t.Errorf("DownloadFolder = %q, want the user's Downloads folder", s.DownloadFolder)
	}
}

func TestSettingsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	store, err := NewSettingsStore(dir)
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}

	want := Settings{DownloadFolder: dir, Concurrency: 4, Cookies: core.BrowserFirefox, Appearance: AppearanceDark}
	if _, err := store.Save(want); err != nil {
		t.Fatalf("Save: %v", err)
	}

	reopened, err := NewSettingsStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	got := reopened.Get()
	if got != want {
		t.Errorf("reloaded settings = %+v, want %+v", got, want)
	}
}

func TestSettingsNormalisation(t *testing.T) {
	cases := []struct {
		name  string
		in    Settings
		check func(*testing.T, Settings)
	}{
		{
			name: "zero concurrency",
			in:   Settings{Concurrency: 0},
			check: func(t *testing.T, s Settings) {
				if s.Concurrency != core.DefaultConcurrency {
					t.Errorf("Concurrency = %d", s.Concurrency)
				}
			},
		},
		{
			name: "excessive concurrency",
			in:   Settings{Concurrency: 500},
			check: func(t *testing.T, s Settings) {
				if s.Concurrency != core.MaxConcurrency {
					t.Errorf("Concurrency = %d, want it clamped", s.Concurrency)
				}
			},
		},
		{
			name: "unknown browser",
			in:   Settings{Cookies: core.Browser("netscape")},
			check: func(t *testing.T, s Settings) {
				if s.Cookies != core.BrowserNone {
					t.Errorf("Cookies = %q, want it reset", s.Cookies)
				}
			},
		},
		{
			name: "unknown appearance",
			in:   Settings{Appearance: Appearance("sepia")},
			check: func(t *testing.T, s Settings) {
				if s.Appearance != AppearanceAuto {
					t.Errorf("Appearance = %q, want Auto", s.Appearance)
				}
			},
		},
		{
			name: "empty folder",
			in:   Settings{DownloadFolder: ""},
			check: func(t *testing.T, s Settings) {
				if s.DownloadFolder == "" {
					t.Error("DownloadFolder was left empty")
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.check(t, normalise(tc.in))
		})
	}
}

func TestSaveRejectsMissingFolder(t *testing.T) {
	store, err := NewSettingsStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	_, err = store.Save(Settings{DownloadFolder: "/nope/does/not/exist", Concurrency: 2})
	if err == nil {
		t.Fatal("Save accepted a folder that does not exist")
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("error = %q", err)
	}
}

func TestSaveRejectsFileAsFolder(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "a-file")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	store, _ := NewSettingsStore(dir)
	if _, err := store.Save(Settings{DownloadFolder: file, Concurrency: 2}); err == nil {
		t.Error("Save accepted a file as the download folder")
	}
}

func TestCorruptSettingsFallBackToDefaults(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, settingsFileName), []byte("{broken"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Damaged settings are not worth refusing to launch over.
	store, err := NewSettingsStore(dir)
	if err != nil {
		t.Fatalf("NewSettingsStore refused a corrupt file: %v", err)
	}
	if store.Get().Concurrency != core.DefaultConcurrency {
		t.Errorf("settings = %+v, want defaults", store.Get())
	}
}

func TestPartialSettingsFileKeepsDefaults(t *testing.T) {
	dir := t.TempDir()
	// A file written by an older version that did not know about concurrency.
	if err := os.WriteFile(filepath.Join(dir, settingsFileName), []byte(`{"cookies":"safari"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	store, err := NewSettingsStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	got := store.Get()
	if got.Cookies != core.BrowserSafari {
		t.Errorf("Cookies = %q, want the value from the file", got.Cookies)
	}
	if got.Concurrency != core.DefaultConcurrency {
		t.Errorf("Concurrency = %d, want the default for a missing field", got.Concurrency)
	}
}

func TestApplyToFillsGapsOnly(t *testing.T) {
	s := Settings{DownloadFolder: "/Users/x/Movies", Cookies: core.BrowserSafari}

	// An empty request picks up the defaults.
	got := s.ApplyTo(core.Options{})
	if got.Output.Folder != "/Users/x/Movies" || got.Network.Cookies != core.BrowserSafari {
		t.Errorf("defaults not applied: %+v", got)
	}

	// An explicit choice is never overridden.
	explicit := core.Options{
		Output:  core.Output{Folder: "/Users/x/Elsewhere"},
		Network: core.Network{Cookies: core.BrowserFirefox},
	}
	got = s.ApplyTo(explicit)
	if got.Output.Folder != "/Users/x/Elsewhere" {
		t.Errorf("Folder = %q, want the explicit choice kept", got.Output.Folder)
	}
	if got.Network.Cookies != core.BrowserFirefox {
		t.Errorf("Cookies = %q, want the explicit choice kept", got.Network.Cookies)
	}
}

func TestSettingsWritesAreAtomic(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewSettingsStore(dir)
	if _, err := store.Save(Settings{DownloadFolder: dir, Concurrency: 3}); err != nil {
		t.Fatal(err)
	}

	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".settings-") {
			t.Errorf("temporary file %s was left behind", e.Name())
		}
	}
}

func TestNewSettingsStoreNeedsDirectory(t *testing.T) {
	if _, err := NewSettingsStore(""); err == nil {
		t.Error("NewSettingsStore accepted an empty directory")
	}
}

func TestSavedAppearance(t *testing.T) {
	dir := t.TempDir()
	if got := savedAppearance(dir); got != AppearanceAuto {
		t.Errorf("with no settings file = %q, want Auto", got)
	}
	// The window is created before startup opens the store, so reading the
	// choice must not be what creates the file or the folder.
	missing := filepath.Join(dir, "not-yet")
	savedAppearance(missing)
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Errorf("reading the appearance created %s", missing)
	}

	if err := os.WriteFile(filepath.Join(dir, settingsFileName), []byte(`{"appearance":"dark"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := savedAppearance(dir); got != AppearanceDark {
		t.Errorf("saved dark, read %q", got)
	}

	if err := os.WriteFile(filepath.Join(dir, settingsFileName), []byte(`{"appearance":"sepia"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := savedAppearance(dir); got != AppearanceAuto {
		t.Errorf("an unknown appearance read as %q, want Auto", got)
	}
}
