package core

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func savedTo(path string) func(*QueueConfig) {
	return func(c *QueueConfig) { c.SavePath = path }
}

// blockingRunner downloads until it is cancelled, like a long file.
func blockingRunner() *funcRunner {
	return &funcRunner{run: func(ctx context.Context, args []string, stdout, _ func(string)) error {
		if isMetadataCall(args) {
			return nil
		}
		stdout(`{"stage":"downloading","downloaded":10,"total":100}`)
		<-ctx.Done()
		return ctx.Err()
	}}
}

func readSaved(t *testing.T, path string) []Item {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the saved queue: %v", err)
	}
	var saved savedQueue
	if err := json.Unmarshal(raw, &saved); err != nil {
		t.Fatalf("the saved queue is not JSON: %v", err)
	}
	return saved.Items
}

func TestQuittingMidDownloadResumesNextLaunch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "queue.json")

	h := newQueueHarnessWith(t, blockingRunner(), 1, nil, savedTo(path))
	item, _ := h.q.Add(uniqueOptions(), Source{Title: "Long video"})
	h.waitFor(t, item.ID, StateDownloading)
	h.q.Close() // quitting Lasso

	// It was not cancelled by anyone, so it must not be recorded as such —
	// that is what put interrupted downloads into history as "cancelled".
	for _, state := range h.statesOf(item.ID) {
		if state == StateCancelled {
			t.Fatal("quitting recorded an interrupted download as cancelled")
		}
	}
	saved := readSaved(t, path)
	if len(saved) != 1 || saved[0].ID != item.ID || saved[0].State != StateQueued {
		t.Fatalf("saved = %+v, want the download kept as queued", saved)
	}

	// Next launch: it comes back and runs to completion under the same ID.
	next := newQueueHarnessWith(t, finishingAs("/m/Long video.mp4"), 1, nil, savedTo(path))
	next.waitFor(t, item.ID, StateDone)
	if len(readSaved(t, path)) != 0 {
		t.Error("a finished download stayed in the saved queue")
	}
}

func TestACancelIsNotBroughtBack(t *testing.T) {
	path := filepath.Join(t.TempDir(), "queue.json")
	h := newQueueHarnessWith(t, blockingRunner(), 1, nil, savedTo(path))

	item, _ := h.q.Add(uniqueOptions(), Source{Title: "Not wanted"})
	h.waitFor(t, item.ID, StateDownloading)
	h.q.Cancel(item.ID)
	h.waitFor(t, item.ID, StateCancelled)
	h.q.Close()

	if saved := readSaved(t, path); len(saved) != 0 {
		t.Errorf("saved = %+v; a download the user cancelled came back", saved)
	}
}

func TestPausedAndFailedComeBackAsTheyWere(t *testing.T) {
	path := filepath.Join(t.TempDir(), "queue.json")
	failing := &funcRunner{run: func(_ context.Context, args []string, _, stderr func(string)) error {
		if isMetadataCall(args) {
			return nil
		}
		stderr("ERROR: [youtube] x: Video unavailable")
		return errors.New("exit status 1")
	}}

	h := newQueueHarnessWith(t, failing, 1, nil, savedTo(path))
	broken, _ := h.q.Add(uniqueOptions(), Source{Title: "Broken"})
	h.waitFor(t, broken.ID, StateFailed)
	h.q.Close()

	h2 := newQueueHarnessWith(t, blockingRunner(), 1, nil, savedTo(path))
	paused, _ := h2.q.Add(uniqueOptions(), Source{Title: "Later"})
	h2.waitFor(t, paused.ID, StateDownloading)
	h2.q.Pause(paused.ID)
	h2.waitFor(t, paused.ID, StatePaused)
	h2.q.Close()

	// The third launch sees both, as they were. A runner that would finish
	// anything proves the paused one is not quietly restarted.
	h3 := newQueueHarnessWith(t, finishingAs("/m/x.mp4"), 1, nil, savedTo(path))
	time.Sleep(100 * time.Millisecond)
	got := map[string]State{}
	for _, it := range h3.q.Items() {
		got[it.ID] = it.State
	}
	if got[broken.ID] != StateFailed {
		t.Errorf("failed download came back as %q, want failed so it can be retried", got[broken.ID])
	}
	if got[paused.ID] != StatePaused {
		t.Errorf("paused download came back as %q, want paused — pausing was a decision", got[paused.ID])
	}
}

func TestIDsDoNotRepeatAcrossLaunches(t *testing.T) {
	// History keeps the IDs its entries had, and Open and Show in Finder
	// resolve by ID. A counter restarting at 1 made "3" mean two things.
	seen := map[string]bool{}
	for launch := 0; launch < 3; launch++ {
		h := newQueueHarnessWith(t, blockingRunner(), 1, nil)
		for i := 0; i < 5; i++ {
			item, _ := h.q.Add(uniqueOptions(), Source{Title: "x"})
			if seen[item.ID] {
				t.Fatalf("ID %q was issued twice across launches", item.ID)
			}
			seen[item.ID] = true
		}
		h.q.Close()
	}
}

func TestTheSavedQueueIsPrivateAndARuinedOneIsIgnored(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "queue.json")
	h := newQueueHarnessWith(t, blockingRunner(), 1, nil, savedTo(path))
	item, _ := h.q.Add(uniqueOptions(), Source{Title: "x"})
	h.waitFor(t, item.ID, StateDownloading)
	h.q.Close()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("queue.json is %v; it lists every link being downloaded, so 0600", mode)
	}

	os.WriteFile(path, []byte("{not json"), 0o600)
	q, err := NewQueue(QueueConfig{Runner: blockingRunner(), SavePath: path})
	if err != nil {
		t.Fatalf("a ruined save stopped the queue from starting: %v", err)
	}
	if len(q.Items()) != 0 {
		t.Error("a ruined save produced items")
	}
}
