// Package history records what Lasso has downloaded.
//
// The queue is deliberately in-memory: it describes work in flight, and
// quitting ends that work. The record of what came out of it is a different
// thing and outlives the session, which is what this package holds.
package history

import (
	"time"

	"github.com/swayyaam/lasso/packages/core"
)

// Entry is one finished download.
//
// It keeps the whole core.Options rather than a summary of them, so an entry
// can be put back on the queue exactly as it ran. The fields that describe how
// it ended are flattened out of core.Item: an entry is a record, not a live
// item, and nothing about it is going to change again.
type Entry struct {
	ID    string `json:"id"`
	URL   string `json:"url"`
	Title string `json:"title"`
	// Options are what produced this file, kept whole so "download again" is
	// exact rather than approximate.
	Options core.Options `json:"options"`
	// FilePath is the file on disk, if one was produced.
	FilePath string `json:"filePath"`
	// State is how it ended: done, failed or cancelled.
	State core.State `json:"state"`
	// Message explains a failure in plain language, empty on success.
	Message string `json:"message"`
	// ErrorKind lets the UI tell retryable failures from permanent ones.
	ErrorKind core.ErrorKind `json:"errorKind"`
	// Notice is a caveat about a download that otherwise succeeded.
	Notice string `json:"notice"`
	// Bytes is the transfer size, when it was known.
	Bytes int64 `json:"bytes"`
	// FinishedAt is Unix milliseconds, matching core.Item.AddedAt — Wails
	// cannot model a time.Time and would emit an untyped value for it.
	FinishedAt int64 `json:"finishedAt"`
}

// FromItem builds an entry from a queue item that has reached a terminal state.
//
// It reports false for an item still in flight: a record of something that has
// not finished would be a lie, and the queue is where in-flight work lives.
func FromItem(item core.Item) (Entry, bool) {
	if !item.State.IsTerminal() {
		return Entry{}, false
	}
	return Entry{
		ID:         item.ID,
		URL:        item.Options.URL,
		Title:      item.Title,
		Options:    item.Options,
		FilePath:   item.FilePath,
		State:      item.State,
		Message:    item.Message,
		ErrorKind:  item.ErrorKind,
		Notice:     item.Notice,
		Bytes:      item.Progress.Downloaded,
		FinishedAt: time.Now().UnixMilli(),
	}, true
}

// Succeeded reports whether the entry produced a file.
func (e Entry) Succeeded() bool { return e.State == core.StateDone }
