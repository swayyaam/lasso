package core

import (
	"sync"
	"time"
)

// DefaultProgressInterval is how often a running download is allowed to report
// progress to the UI.
//
// yt-dlp emits updates far faster than a person can read them, and on a large
// playlist every item is emitting at once. Coalescing to this interval keeps
// the UI responsive without losing the value that matters — the most recent
// one always survives, and terminal states bypass the throttle entirely.
const DefaultProgressInterval = 150 * time.Millisecond

// ProgressEmitter coalesces progress updates per item.
//
// Update keeps only the newest value inside a window; Flush forces whatever is
// pending out, which is what guarantees a finished download shows 100% rather
// than whatever the last un-throttled update happened to say.
//
// It is safe for concurrent use: several downloads report at once.
type ProgressEmitter struct {
	interval time.Duration
	now      func() time.Time
	emit     func(id string, p Progress)

	mu    sync.Mutex
	state map[string]*throttleState
}

type throttleState struct {
	lastEmit time.Time
	pending  *Progress
}

// NewProgressEmitter returns an emitter that calls emit at most once per
// interval per id. A zero interval emits everything, which is useful in tests.
func NewProgressEmitter(interval time.Duration, emit func(id string, p Progress)) *ProgressEmitter {
	return &ProgressEmitter{
		interval: interval,
		now:      time.Now,
		emit:     emit,
		state:    map[string]*throttleState{},
	}
}

// Update offers a progress value for an item. It is emitted immediately if the
// item's window has elapsed, and held as pending otherwise.
func (e *ProgressEmitter) Update(id string, p Progress) {
	e.mu.Lock()
	s, ok := e.state[id]
	if !ok {
		s = &throttleState{}
		e.state[id] = s
	}

	now := e.now()
	if !s.lastEmit.IsZero() && now.Sub(s.lastEmit) < e.interval {
		// Inside the window: keep only the newest value.
		held := p
		s.pending = &held
		e.mu.Unlock()
		return
	}

	s.lastEmit = now
	s.pending = nil
	emit := e.emit
	e.mu.Unlock()

	if emit != nil {
		emit(id, p)
	}
}

// Flush emits an item's pending value, if any, ignoring the window.
//
// Call it whenever a download reaches a state the user must see — finished,
// failed, cancelled — so the last update is never left stuck inside a window.
func (e *ProgressEmitter) Flush(id string) {
	e.mu.Lock()
	s, ok := e.state[id]
	if !ok || s.pending == nil {
		e.mu.Unlock()
		return
	}
	p := *s.pending
	s.pending = nil
	s.lastEmit = e.now()
	emit := e.emit
	e.mu.Unlock()

	if emit != nil {
		emit(id, p)
	}
}

// Forget drops an item's throttling state. Call it when an item leaves the
// queue, so a long-lived emitter does not accumulate entries.
func (e *ProgressEmitter) Forget(id string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.state, id)
}
