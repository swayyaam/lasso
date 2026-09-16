package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/swayyaam/lasso/packages/core"
)

// settingsFileName is the settings file inside Lasso's Application Support
// directory.
const settingsFileName = "settings.json"

// Settings are the preferences shown on the settings screen.
//
// This type crosses into the frontend, so its JSON names are part of the
// generated TypeScript bindings.
type Settings struct {
	// DownloadFolder is where files land when a download does not say otherwise.
	DownloadFolder string `json:"downloadFolder"`
	// Concurrency is how many downloads run at once.
	Concurrency int `json:"concurrency"`
	// Cookies is the browser to take cookies from, empty for none.
	Cookies core.Browser `json:"cookies"`
}

// SettingsStore persists Settings as JSON.
type SettingsStore struct {
	path string

	mu      sync.Mutex
	current Settings
}

// defaultSettings returns the settings a fresh install starts with.
func defaultSettings() Settings {
	folder := ""
	if home, err := os.UserHomeDir(); err == nil {
		folder = filepath.Join(home, "Downloads")
	}
	return Settings{
		DownloadFolder: folder,
		Concurrency:    core.DefaultConcurrency,
		Cookies:        core.BrowserNone,
	}
}

// NewSettingsStore opens the settings file in dir, falling back to defaults if
// it is missing or unreadable. Damaged settings are not worth refusing to
// launch over.
func NewSettingsStore(dir string) (*SettingsStore, error) {
	if dir == "" {
		return nil, fmt.Errorf("settings store needs a directory")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating %s: %w", dir, err)
	}

	s := &SettingsStore{path: filepath.Join(dir, settingsFileName)}
	s.current = s.load()
	return s, nil
}

func (s *SettingsStore) load() Settings {
	settings := defaultSettings()

	raw, err := os.ReadFile(s.path)
	if err != nil {
		return settings
	}
	// Unmarshalling onto the defaults means a file written by an older version
	// keeps sensible values for fields it does not mention.
	if err := json.Unmarshal(raw, &settings); err != nil {
		return defaultSettings()
	}
	return normalise(settings)
}

// normalise repairs values that are out of range, so a hand-edited file cannot
// put the app into a state the UI cannot represent.
func normalise(s Settings) Settings {
	if s.Concurrency <= 0 {
		s.Concurrency = core.DefaultConcurrency
	}
	if s.Concurrency > core.MaxConcurrency {
		s.Concurrency = core.MaxConcurrency
	}
	if s.DownloadFolder == "" {
		s.DownloadFolder = defaultSettings().DownloadFolder
	}
	switch s.Cookies {
	case core.BrowserNone, core.BrowserSafari, core.BrowserChrome,
		core.BrowserFirefox, core.BrowserBrave, core.BrowserArc:
	default:
		s.Cookies = core.BrowserNone
	}
	return s
}

// Get returns the current settings.
func (s *SettingsStore) Get() Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.current
}

// Save validates, stores and persists new settings, returning what was actually
// applied after normalisation.
func (s *SettingsStore) Save(settings Settings) (Settings, error) {
	settings = normalise(settings)

	if settings.DownloadFolder != "" {
		info, err := os.Stat(settings.DownloadFolder)
		if err != nil {
			return Settings{}, fmt.Errorf("that download folder does not exist")
		}
		if !info.IsDir() {
			return Settings{}, fmt.Errorf("that download folder is not a folder")
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.current = settings
	if err := s.persistLocked(); err != nil {
		return Settings{}, err
	}
	return settings, nil
}

// persistLocked writes settings via a temporary file and a rename, so an
// interrupted write cannot leave a truncated file behind.
func (s *SettingsStore) persistLocked() error {
	raw, err := json.MarshalIndent(s.current, "", "  ")
	if err != nil {
		return err
	}

	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".settings-*")
	if err != nil {
		return fmt.Errorf("saving settings: %w", err)
	}
	defer os.Remove(tmp.Name())

	if _, err := tmp.Write(append(raw, '\n')); err != nil {
		tmp.Close()
		return fmt.Errorf("saving settings: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("saving settings: %w", err)
	}
	return os.Rename(tmp.Name(), s.path)
}

// ApplyTo fills in the parts of a download request the settings are
// responsible for, without overriding anything the request set explicitly.
func (s Settings) ApplyTo(o core.Options) core.Options {
	if o.Output.Folder == "" {
		o.Output.Folder = s.DownloadFolder
	}
	if o.Network.Cookies == core.BrowserNone {
		o.Network.Cookies = s.Cookies
	}
	return o
}
