package history

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/swayyaam/lasso/packages/core"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	s, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	return s
}

func doneItem(id, title string) core.Item {
	return core.Item{
		ID:       id,
		Title:    title,
		State:    core.StateDone,
		FilePath: "/tmp/" + title + ".mp4",
		Options:  core.Options{URL: "https://example.com/" + id, Pick: core.Pick1080p},
		Progress: core.Progress{Downloaded: 2048},
	}
}

func TestFromItemRefusesWorkStillInFlight(t *testing.T) {
	// A record of something that has not finished would be a lie, and the item
	// is still going to change.
	for _, state := range []core.State{core.StateQueued, core.StateDownloading, core.StatePostProcessing} {
		if _, ok := FromItem(core.Item{ID: "1", State: state}); ok {
			t.Errorf("state %q produced a history entry", state)
		}
	}
	for _, state := range []core.State{core.StateDone, core.StateFailed, core.StateCancelled} {
		if _, ok := FromItem(core.Item{ID: "1", State: state}); !ok {
			t.Errorf("state %q produced no history entry", state)
		}
	}
}

func TestFromItemKeepsTheOptionsWhole(t *testing.T) {
	// "Download again" has to reproduce the original exactly, so the options
	// travel intact rather than as a summary.
	item := doneItem("1", "Clip")
	item.Options.Subtitles = core.Subtitles{Download: true, Languages: []string{"en"}}

	entry, ok := FromItem(item)
	if !ok {
		t.Fatal("expected an entry")
	}
	if entry.Options.Pick != core.Pick1080p {
		t.Errorf("Pick = %q, want the original", entry.Options.Pick)
	}
	if !entry.Options.WantsSubtitles() {
		t.Error("subtitle options did not survive")
	}
	if entry.FinishedAt == 0 {
		t.Error("entry has no finished time")
	}
}

func TestNewestFirst(t *testing.T) {
	s := newStore(t)
	for _, id := range []string{"1", "2", "3"} {
		entry, _ := FromItem(doneItem(id, "Clip"+id))
		if _, err := s.Add(entry); err != nil {
			t.Fatalf("Add: %v", err)
		}
	}

	all := s.All()
	if len(all) != 3 {
		t.Fatalf("got %d entries, want 3", len(all))
	}
	if all[0].ID != "3" {
		t.Errorf("first entry is %q, want the most recent", all[0].ID)
	}
}

func TestRetryReplacesRatherThanDuplicates(t *testing.T) {
	// The queue reuses an item's id across a retry, so the same download can
	// finish twice. The second outcome is the true one.
	s := newStore(t)

	failed, _ := FromItem(core.Item{ID: "1", Title: "Clip", State: core.StateFailed, Message: "nope"})
	s.Add(failed)
	succeeded, _ := FromItem(doneItem("1", "Clip"))
	s.Add(succeeded)

	all := s.All()
	if len(all) != 1 {
		t.Fatalf("got %d entries, want the retry to replace the failure", len(all))
	}
	if !all[0].Succeeded() {
		t.Errorf("State = %q, want the later outcome to win", all[0].State)
	}
}

func TestSurvivesReopening(t *testing.T) {
	dir := t.TempDir()
	first, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	entry, _ := FromItem(doneItem("1", "Clip"))
	if _, err := first.Add(entry); err != nil {
		t.Fatalf("Add: %v", err)
	}

	// The whole point of history: quitting must not discard it.
	second, err := NewStore(dir)
	if err != nil {
		t.Fatalf("reopening: %v", err)
	}
	all := second.All()
	if len(all) != 1 {
		t.Fatalf("got %d entries after reopening, want 1", len(all))
	}
	if all[0].FilePath != entry.FilePath {
		t.Errorf("FilePath = %q, want it to survive the round trip", all[0].FilePath)
	}
}

func TestTrimsToTheCap(t *testing.T) {
	s := newStore(t)
	for i := range MaxEntries + 20 {
		entry, _ := FromItem(doneItem(string(rune('a'+i%26))+string(rune('0'+i/26)), "Clip"))
		entry.ID = "id" + string(rune(i))
		s.Add(entry)
	}

	if got := len(s.All()); got != MaxEntries {
		t.Errorf("got %d entries, want the store capped at %d", got, MaxEntries)
	}
}

func TestRemoveAndClear(t *testing.T) {
	s := newStore(t)
	for _, id := range []string{"1", "2"} {
		entry, _ := FromItem(doneItem(id, "Clip"))
		s.Add(entry)
	}

	if err := s.Remove("1"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if len(s.All()) != 1 {
		t.Errorf("Remove did not drop the entry")
	}
	if err := s.Remove("nope"); err != ErrNotFound {
		t.Errorf("Remove of a missing id = %v, want ErrNotFound", err)
	}

	if err := s.Clear(); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if len(s.All()) != 0 {
		t.Error("Clear left entries behind")
	}
}

func TestDamagedFileStartsFresh(t *testing.T) {
	// A damaged record must not stop the app downloading anything new.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	s, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore on a damaged file: %v", err)
	}
	if len(s.All()) != 0 {
		t.Error("expected to start empty")
	}
	if _, err := s.Add(mustEntry(t, doneItem("1", "Clip"))); err != nil {
		t.Errorf("Add after a damaged file: %v", err)
	}
}

func TestWritesValidJSON(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewStore(dir)
	s.Add(mustEntry(t, doneItem("1", "Clip")))

	raw, err := os.ReadFile(filepath.Join(dir, FileName))
	if err != nil {
		t.Fatalf("reading the store: %v", err)
	}
	var f file
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatalf("the store is not valid JSON: %v", err)
	}
	if f.Version != 1 {
		t.Errorf("Version = %d, want the file to carry one", f.Version)
	}
}

func mustEntry(t *testing.T, item core.Item) Entry {
	t.Helper()
	entry, ok := FromItem(item)
	if !ok {
		t.Fatal("item produced no entry")
	}
	return entry
}

func TestAllIsNeverNil(t *testing.T) {
	// nil and an empty slice are the same thing in Go and different things once
	// they cross into the frontend, where nil arrives as JSON null and the list
	// cannot be iterated.
	s := newStore(t)

	if got := s.All(); got == nil {
		t.Fatal("All() on an empty history returned nil, want an empty slice")
	}

	entry, _ := FromItem(doneItem("1", "Clip"))
	s.Add(entry)
	if err := s.Clear(); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if got := s.All(); got == nil {
		t.Error("All() after clearing returned nil, want an empty slice")
	}
}

func TestAllMarshalsAsAList(t *testing.T) {
	raw, err := json.Marshal(newStore(t).All())
	if err != nil {
		t.Fatalf("marshalling: %v", err)
	}
	if string(raw) != "[]" {
		t.Errorf("an empty history marshals as %s, want []", raw)
	}
}

func TestFromItemKeepsWhatTheRowShows(t *testing.T) {
	item := core.Item{
		ID: "1", Title: "Clip", State: core.StateDone,
		Uploader: "Someone", Duration: 61, Thumbnail: "https://i.example.com/t.jpg",
		Resolution: "1080p", Bytes: 4096,
		Progress: core.Progress{Downloaded: 1000},
	}
	entry, ok := FromItem(item)
	if !ok {
		t.Fatal("a finished item should make an entry")
	}
	if entry.Uploader != "Someone" || entry.Duration != 61 || entry.Thumbnail != item.Thumbnail || entry.Resolution != "1080p" {
		t.Errorf("entry lost the item's source: %+v", entry)
	}
	if entry.Bytes != 4096 {
		t.Errorf("Bytes = %d, want the file's size over the transfer's", entry.Bytes)
	}
	if src := entry.Source(); src.Title != "Clip" || src.Thumbnail != item.Thumbnail {
		t.Errorf("Source = %+v, want it to round-trip for Download again", src)
	}

	// An item finished before sizes were measured still reports something.
	item.Bytes = 0
	if entry, _ := FromItem(item); entry.Bytes != 1000 {
		t.Errorf("Bytes = %d, want the transfer size as a fallback", entry.Bytes)
	}
}
