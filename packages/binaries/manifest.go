package binaries

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Name identifies a sidecar binary.
type Name string

const (
	YtDlp   Name = "yt-dlp"
	FFmpeg  Name = "ffmpeg"
	FFprobe Name = "ffprobe"
	Deno    Name = "deno"
)

// Layout describes how a binary sits on disk.
//
// Most sidecars are a single executable. yt-dlp is a PyInstaller onedir build:
// a directory holding a launcher plus an _internal tree, which must be copied
// and kept together.
type Layout string

const (
	LayoutFile Layout = "file"
	LayoutDir  Layout = "dir"
)

// Spec describes one sidecar binary as recorded in the manifest that ships
// beside the binaries.
type Spec struct {
	Version    string `json:"version"`
	Layout     Layout `json:"layout"`
	Entrypoint string `json:"entrypoint"`
}

// Manifest is the manifest.json written by scripts/fetch-binaries.sh next to
// the binaries it fetched. It travels with them into the .app bundle.
type Manifest struct {
	SchemaVersion int           `json:"schemaVersion"`
	Platform      string        `json:"platform"`
	Binaries      map[Name]Spec `json:"binaries"`
}

const manifestName = "manifest.json"

// requiredBinaries are the sidecars Lasso cannot run without.
var requiredBinaries = []Name{YtDlp, FFmpeg, FFprobe, Deno}

// ReadManifest loads the manifest from a directory holding sidecar binaries.
func ReadManifest(dir string) (*Manifest, error) {
	path := filepath.Join(dir, manifestName)
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no %s in %s: the sidecar binaries were never fetched (run `make setup`)", manifestName, dir)
		}
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}

	var m Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	if m.SchemaVersion != 1 {
		return nil, fmt.Errorf("%s has schemaVersion %d, this build understands 1", path, m.SchemaVersion)
	}
	for _, name := range requiredBinaries {
		spec, ok := m.Binaries[name]
		if !ok {
			return nil, fmt.Errorf("%s does not list %q", path, name)
		}
		if spec.Entrypoint == "" {
			return nil, fmt.Errorf("%s: %q has no entrypoint", path, name)
		}
		if spec.Layout != LayoutFile && spec.Layout != LayoutDir {
			return nil, fmt.Errorf("%s: %q has unknown layout %q", path, name, spec.Layout)
		}
	}
	return &m, nil
}

// relPath is the path of a binary's executable relative to the directory that
// holds the whole sidecar set.
func (s Spec) relPath(name Name) string {
	if s.Layout == LayoutDir {
		return filepath.Join(string(name), s.Entrypoint)
	}
	return s.Entrypoint
}

// rootRel is the path of whatever gets copied for this binary — the executable
// itself for a file layout, the containing directory for a dir layout —
// relative to the directory that holds the sidecar set.
func (s Spec) rootRel(name Name) string {
	if s.Layout == LayoutDir {
		return string(name)
	}
	return s.Entrypoint
}

// stamp records what Install actually put in place, so later launches can skip
// copying and so a version bump forces a refresh.
type stamp struct {
	Binaries map[Name]string `json:"binaries"`
}

const stampName = ".installed.json"

func readStamp(dir string) stamp {
	var s stamp
	raw, err := os.ReadFile(filepath.Join(dir, stampName))
	if err != nil {
		return stamp{Binaries: map[Name]string{}}
	}
	if err := json.Unmarshal(raw, &s); err != nil || s.Binaries == nil {
		return stamp{Binaries: map[Name]string{}}
	}
	return s
}

func writeStamp(dir string, s stamp) error {
	raw, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, stampName), append(raw, '\n'), 0o644)
}
