package history

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
)

// FileName is the store's file inside Lasso's Application Support directory.
const FileName = "history.json"

// MaxEntries bounds the record.
//
// History is a convenience, not an archive: a user looking for something they
// downloaded is looking in the recent past, and an unbounded file would grow
// for the lifetime of the install and be read in full at every launch.
const MaxEntries = 500

// ErrNotFound is returned when no entry has the given id.
var ErrNotFound = errors.New("no such history entry")

// Store holds finished downloads and persists them as JSON.
//
// Entries are kept newest first, which is both the order the UI wants and the
// order that makes trimming to MaxEntries a truncation.
type Store struct {
	path string

	mu      sync.Mutex
	entries []Entry
}

// file is the on-disk shape. Wrapping the list in an object leaves room to add
// fields later without breaking older files.
type file struct {
	Version int     `json:"version"`
	Entries []Entry `json:"entries"`
}

// NewStore opens the history in dir, creating the directory if needed.
//
// An unreadable or corrupt file is not fatal. History is a record of the past,
// and refusing to launch — or to download anything new — because the record is
// damaged would be a worse outcome than starting a fresh one.
func NewStore(dir string) (*Store, error) {
	if dir == "" {
		return nil, fmt.Errorf("history store needs a directory")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating %s: %w", dir, err)
	}

	s := &Store{path: filepath.Join(dir, FileName)}
	s.entries = s.load()
	return s, nil
}

func (s *Store) load() []Entry {
	raw, err := os.ReadFile(s.path)
	if err != nil {
		return nil
	}

	var f file
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil
	}
	// Trim on read as well as on write: the cap may have been lowered since
	// the file was written, or the file may have been edited by hand.
	if len(f.Entries) > MaxEntries {
		f.Entries = f.Entries[:MaxEntries]
	}
	return f.Entries
}

// All returns every entry, newest first.
//
// An empty history is an empty slice, never nil. The two are the same thing in
// Go and different things across the Wails boundary, where nil arrives as JSON
// null and a caller reasonably expecting a list gets nothing it can iterate.
func (s *Store) All() []Entry {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]Entry, 0, len(s.entries))
	return append(out, s.entries...)
}

// Add records a finished download and returns the entry as stored.
//
// An id already present is replaced rather than duplicated: an item that was
// retried finishes more than once, and the queue reuses its id, so the second
// outcome is the true one.
func (s *Store) Add(entry Entry) (Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.entries = slices.DeleteFunc(s.entries, func(e Entry) bool { return e.ID == entry.ID })
	s.entries = append([]Entry{entry}, s.entries...)
	if len(s.entries) > MaxEntries {
		s.entries = s.entries[:MaxEntries]
	}

	if err := s.persistLocked(); err != nil {
		return Entry{}, err
	}
	return entry, nil
}

// Remove drops one entry.
func (s *Store) Remove(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	before := len(s.entries)
	s.entries = slices.DeleteFunc(s.entries, func(e Entry) bool { return e.ID == id })
	if len(s.entries) == before {
		return ErrNotFound
	}
	return s.persistLocked()
}

// Clear empties the history.
func (s *Store) Clear() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.entries = nil
	return s.persistLocked()
}

// persistLocked writes via a temporary file and a rename, so an interrupted
// write cannot leave a truncated file behind.
func (s *Store) persistLocked() error {
	// Written as a list even when empty. A cleared history is [] on disk rather
	// than null, which keeps the file readable by anything that expects one.
	entries := s.entries
	if entries == nil {
		entries = []Entry{}
	}
	raw, err := json.MarshalIndent(file{Version: 1, Entries: entries}, "", "  ")
	if err != nil {
		return err
	}

	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".history-*")
	if err != nil {
		return fmt.Errorf("saving history: %w", err)
	}
	defer os.Remove(tmp.Name())

	if _, err := tmp.Write(append(raw, '\n')); err != nil {
		tmp.Close()
		return fmt.Errorf("saving history: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("saving history: %w", err)
	}
	return os.Rename(tmp.Name(), s.path)
}
