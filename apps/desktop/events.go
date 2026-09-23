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
	// EventQueueRemoved carries the ids that have left the queue. It is the one
	// change EventQueueItem cannot describe: there is no item left to send.
	EventQueueRemoved = "queue:removed"
	// EventHistoryChanged says the record of finished downloads has moved on.
	// It carries nothing: the frontend asks for the list, which keeps one
	// definition of what history is rather than two.
	EventHistoryChanged = "history:changed"
	// EventMenu carries a menu command for the interface to carry out, one of
	// the Menu* names.
	EventMenu = "menu"
	// EventLinkWaiting says a link arrived from outside the window. It carries
	// nothing: the interface collects it with TakeIncomingLink, which is also
	// how it finds one that arrived before it was listening.
	EventLinkWaiting = "link:waiting"
)

// EventNames is the set of event names, exposed so TypeScript can subscribe
// using the same strings the backend emits rather than its own copies.
type EventNames struct {
	QueueItem       string `json:"queueItem"`
	QueueProgress   string `json:"queueProgress"`
	BinaryStatus    string `json:"binaryStatus"`
	SettingsChanged string `json:"settingsChanged"`
	QueueRemoved    string `json:"queueRemoved"`
	HistoryChanged  string `json:"historyChanged"`
	Menu            string `json:"menu"`
	LinkWaiting     string `json:"linkWaiting"`
}

// MenuCommands names what EventMenu can carry, handed over for the same reason
// as the event names.
type MenuCommands struct {
	Settings  string `json:"settings"`
	Download  string `json:"download"`
	Downloads string `json:"downloads"`
	History   string `json:"history"`
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
