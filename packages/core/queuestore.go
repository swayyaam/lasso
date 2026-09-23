package core

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
)

// newItemID names a download uniquely across launches.
//
// It was a counter that started again at 1 every launch, and history keeps
// the IDs its entries had, so today's download "3" and yesterday's "3" were
// the same name — and Open and Show in Finder resolve by ID. Random IDs
// cannot collide with anything a previous launch wrote.
func newItemID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand does not fail on macOS; this is only for the type.
		panic(err)
	}
	return hex.EncodeToString(b[:])
}

// savedQueue is the queue as it is kept between launches.
type savedQueue struct {
	Items []Item `json:"items"`
}

// kept reports whether an item belongs in the saved queue, and in what state
// it comes back.
//
// Anything unfinished comes back queued, so it resumes — yt-dlp continues its
// part file. Paused stays paused, because pausing was a decision. Failed stays
// failed, so it can still be retried. Finished and cancelled items are not
// kept: they are in history, which is where they belong.
func kept(item Item) (State, bool) {
	switch item.State {
	case StateQueued, StateFetching, StateDownloading, StatePostProcessing:
		return StateQueued, true
	case StatePaused, StateFailed:
		return item.State, true
	default:
		return "", false
	}
}

// persist writes the queue to disk, if it has somewhere to go.
//
// Called on every state change and removal — never on progress, which a
// resumed download re-reads from its part file anyway. Writes are serialised
// and each takes a fresh snapshot, so the last one to land is the latest.
func (q *Queue) persist() {
	if q.savePath == "" {
		return
	}
	q.saveMu.Lock()
	defer q.saveMu.Unlock()

	q.mu.Lock()
	saved := savedQueue{Items: []Item{}}
	for _, id := range q.order {
		item, ok := q.items[id]
		if !ok {
			continue
		}
		if state, keep := kept(*item); keep {
			copy := *item
			copy.State = state
			saved.Items = append(saved.Items, copy)
		}
	}
	q.mu.Unlock()

	raw, err := json.MarshalIndent(saved, "", "  ")
	if err != nil {
		return
	}
	// Temporary file and rename, like every store Lasso keeps, so an
	// interrupted write never leaves a truncated queue. 0600 because it
	// lists every link being downloaded.
	dir := filepath.Dir(q.savePath)
	if os.MkdirAll(dir, 0o755) != nil {
		return
	}
	tmp, err := os.CreateTemp(dir, ".queue-*")
	if err != nil {
		return
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(append(raw, '\n')); err != nil {
		tmp.Close()
		return
	}
	if tmp.Close() != nil {
		return
	}
	_ = os.Rename(tmp.Name(), q.savePath)
}

// restore loads the queue a previous launch saved.
//
// A file that cannot be read is ignored rather than fatal: losing a saved
// queue is bad, refusing to start is worse.
func (q *Queue) restore() {
	if q.savePath == "" {
		return
	}
	raw, err := os.ReadFile(q.savePath)
	if err != nil {
		return
	}
	var saved savedQueue
	if json.Unmarshal(raw, &saved) != nil {
		return
	}
	for _, item := range saved.Items {
		state, keep := kept(item)
		if !keep || item.ID == "" || item.Options.Validate() != nil {
			continue
		}
		if _, dup := q.items[item.ID]; dup {
			continue
		}
		restored := item
		restored.State = state
		if state == StateQueued {
			// It will report fresh progress as it resumes.
			restored.Progress = Progress{}
		}
		q.items[restored.ID] = &restored
		q.order = append(q.order, restored.ID)
	}
}
