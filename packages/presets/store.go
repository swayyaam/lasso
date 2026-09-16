package presets

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/swayyaam/lasso/packages/core"
)

// FileName is the store's file inside Lasso's Application Support directory.
const FileName = "presets.json"

// MaxNameLength bounds a preset name so the UI cannot be broken by a pasted
// wall of text.
const MaxNameLength = 80

// ErrNotFound is returned when no preset has the given id.
var ErrNotFound = errors.New("no such preset")

// ErrReadOnly is returned when a built-in preset is renamed or deleted.
var ErrReadOnly = errors.New("built-in presets cannot be changed")

// Store holds the user's presets and persists them as JSON.
//
// Built-in presets are never written to disk: they ship with the app, so
// persisting them would freeze a copy that later versions could not improve.
type Store struct {
	path string

	mu   sync.Mutex
	user []Preset
}

// file is the on-disk shape. Wrapping the list in an object leaves room to add
// fields later without breaking older files.
type file struct {
	Version int      `json:"version"`
	Presets []Preset `json:"presets"`
}

// NewStore opens the preset store in dir, creating it if needed.
//
// An unreadable or corrupt file is not fatal: the user's presets are a
// convenience, and refusing to launch over them would be worse than starting
// with just the built-ins.
func NewStore(dir string) (*Store, error) {
	if dir == "" {
		return nil, fmt.Errorf("preset store needs a directory")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating %s: %w", dir, err)
	}

	s := &Store{path: filepath.Join(dir, FileName)}
	s.user = s.load()
	return s, nil
}

func (s *Store) load() []Preset {
	raw, err := os.ReadFile(s.path)
	if err != nil {
		return nil
	}
	var f file
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil
	}

	out := make([]Preset, 0, len(f.Presets))
	for _, p := range f.Presets {
		// A file that somehow claims a built-in id would shadow the real one.
		if p.ID == "" || IsBuiltinID(p.ID) {
			continue
		}
		p.BuiltIn = false
		p.Options.URL = ""
		out = append(out, p)
	}
	return out
}

// All returns the built-in presets followed by the user's own.
func (s *Store) All() []Preset {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := Builtins()
	return append(out, s.user...)
}

// UserPresets returns just the user's presets.
func (s *Store) UserPresets() []Preset {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Preset(nil), s.user...)
}

// Get finds a preset by id, built-in or otherwise.
func (s *Store) Get(id string) (Preset, bool) {
	for _, p := range s.All() {
		if p.ID == id {
			return p, true
		}
	}
	return Preset{}, false
}

// Save stores a new preset under the given name.
func (s *Store) Save(name string, o core.Options) (Preset, error) {
	name, err := cleanName(name)
	if err != nil {
		return Preset{}, err
	}

	// A preset describes how to download, never what, so the URL is dropped.
	// Validation would otherwise reject the empty URL a preset must have.
	o.URL = ""

	id, err := newID()
	if err != nil {
		return Preset{}, err
	}
	preset := Preset{ID: id, Name: name, Options: o}

	s.mu.Lock()
	s.user = append(s.user, preset)
	err = s.persistLocked()
	s.mu.Unlock()

	if err != nil {
		return Preset{}, err
	}
	return preset, nil
}

// Rename changes a user preset's name.
func (s *Store) Rename(id, name string) error {
	name, err := cleanName(name)
	if err != nil {
		return err
	}
	if IsBuiltinID(id) {
		return ErrReadOnly
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.user {
		if s.user[i].ID == id {
			s.user[i].Name = name
			return s.persistLocked()
		}
	}
	return ErrNotFound
}

// Update replaces a user preset's options, keeping its id and name.
func (s *Store) Update(id string, o core.Options) error {
	if IsBuiltinID(id) {
		return ErrReadOnly
	}
	o.URL = ""

	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.user {
		if s.user[i].ID == id {
			s.user[i].Options = o
			return s.persistLocked()
		}
	}
	return ErrNotFound
}

// Delete removes a user preset.
func (s *Store) Delete(id string) error {
	if IsBuiltinID(id) {
		return ErrReadOnly
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.user {
		if s.user[i].ID == id {
			s.user = append(s.user[:i], s.user[i+1:]...)
			return s.persistLocked()
		}
	}
	return ErrNotFound
}

// persistLocked writes the user's presets. The caller must hold the lock.
//
// The file is written beside its destination and renamed into place, so a crash
// mid-write cannot leave a truncated file where the presets used to be.
func (s *Store) persistLocked() error {
	raw, err := json.MarshalIndent(file{Version: 1, Presets: s.user}, "", "  ")
	if err != nil {
		return err
	}

	dir := filepath.Dir(s.path)
	tmp, err := os.CreateTemp(dir, ".presets-*")
	if err != nil {
		return fmt.Errorf("saving presets: %w", err)
	}
	defer os.Remove(tmp.Name())

	if _, err := tmp.Write(append(raw, '\n')); err != nil {
		tmp.Close()
		return fmt.Errorf("saving presets: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("saving presets: %w", err)
	}
	if err := os.Rename(tmp.Name(), s.path); err != nil {
		return fmt.Errorf("saving presets: %w", err)
	}
	return nil
}

func cleanName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("a preset needs a name")
	}
	if len(name) > MaxNameLength {
		return "", fmt.Errorf("that name is too long (limit %d characters)", MaxNameLength)
	}
	return name, nil
}

func newID() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("could not create a preset id: %w", err)
	}
	return "user-" + hex.EncodeToString(b[:]), nil
}
