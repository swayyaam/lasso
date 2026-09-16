package main

import "github.com/swayyaam/lasso/packages/core"

// Event names the backend emits. They are defined once here and handed to the
// frontend by App.Events, so a rename cannot leave the two sides disagreeing
// about a string literal.
const (
	// EventQueueItem carries a core.Item whenever its state changes.
	EventQueueItem = "queue:item"
	// EventQueueProgress carries a ProgressEvent for a running download. It is
	// throttled in core; the frontend receives at most one per item per window.
	EventQueueProgress = "queue:progress"
	// EventBinaryStatus carries a binaries.Status after a check or an update.
	EventBinaryStatus = "binaries:status"
	// EventSettingsChanged carries the Settings after they are saved.
	EventSettingsChanged = "settings:changed"
)

// EventNames is the set of event names, exposed so TypeScript can subscribe
// using the same strings the backend emits rather than its own copies.
type EventNames struct {
	QueueItem       string `json:"queueItem"`
	QueueProgress   string `json:"queueProgress"`
	BinaryStatus    string `json:"binaryStatus"`
	SettingsChanged string `json:"settingsChanged"`
}

// ProgressEvent is one throttled progress update for a queue item.
//
// It carries core.Progress unchanged rather than a parallel payload type:
// there should be exactly one definition of what progress means, and the
// generated TypeScript is built from it.
type ProgressEvent struct {
	ID       string        `json:"id"`
	Progress core.Progress `json:"progress"`
}
