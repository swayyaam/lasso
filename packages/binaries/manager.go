package binaries

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Manager locates, installs, verifies and updates Lasso's sidecar binaries.
//
// A Manager is read-only after construction and safe for concurrent use.
type Manager struct {
	paths    Paths
	manifest *Manifest

	// releasesURL is a field rather than a constant so the updater can be
	// exercised end to end against a local server laid out like GitHub,
	// without reaching the network or waiting on a real release.
	releasesURL string
}

// New builds a Manager from the given paths.
func New(paths Paths) (*Manager, error) {
	manifest, err := ReadManifest(paths.Source)
	if err != nil {
		return nil, err
	}
	if paths.Bin == "" {
		return nil, fmt.Errorf("no bin directory configured")
	}
	return &Manager{
		paths:       paths,
		manifest:    manifest,
		releasesURL: ytDlpReleases,
	}, nil
}

// Discover builds a Manager by resolving the standard locations: the bundled
// binaries inside the .app (or the development tree) and Lasso's Application
// Support directory.
func Discover() (*Manager, error) {
	paths, err := ResolvePaths()
	if err != nil {
		return nil, err
	}
	return New(paths)
}

// Paths returns the directories this manager works with.
func (m *Manager) Paths() Paths { return m.paths }

// Version returns the pinned version of a binary.
func (m *Manager) Version(name Name) string { return m.manifest.Binaries[name].Version }

// Path returns the absolute path of a binary's executable inside the install
// directory. It does not check that the binary exists.
func (m *Manager) Path(name Name) string {
	spec, ok := m.manifest.Binaries[name]
	if !ok {
		return ""
	}
	return filepath.Join(m.paths.Bin, spec.relPath(name))
}

// FFmpegLocation is the value to pass to yt-dlp's --ffmpeg-location. yt-dlp
// accepts a directory and finds both ffmpeg and ffprobe inside it.
func (m *Manager) FFmpegLocation() string { return m.paths.Bin }

// Environ is the environment for a yt-dlp subprocess.
//
// The install directory goes on the front of PATH so yt-dlp finds the bundled
// deno — which it needs for YouTube's JavaScript challenges — without ever
// reaching a system-installed copy of anything.
func (m *Manager) Environ() []string {
	env := os.Environ()
	out := make([]string, 0, len(env)+1)
	replaced := false
	for _, kv := range env {
		if name, value, ok := strings.Cut(kv, "="); ok && name == "PATH" {
			out = append(out, "PATH="+m.paths.Bin+string(os.PathListSeparator)+value)
			replaced = true
			continue
		}
		out = append(out, kv)
	}
	if !replaced {
		out = append(out, "PATH="+m.paths.Bin)
	}
	return out
}
