package main

import (
	"sync"

	"github.com/swayyaam/lasso/packages/core"
)

// activity follows every download's state from the queue's own callbacks, and
// turns it into the two things outside the window that reflect it: the Dock
// badge, which counts unfinished downloads, and keeping the Mac awake, which
// matters only while something is actually working.
//
// It is kept from the callbacks rather than read back from the queue, which
// would take the queue's lock from inside its own notification.
type activity struct {
	mu     sync.Mutex
	states map[string]core.State
}

// observe records one download's state and reports the new totals.
func (a *activity) observe(id string, state core.State) (unfinished, busy int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.states == nil {
		a.states = map[string]core.State{}
	}
	if state.IsTerminal() {
		delete(a.states, id)
	} else {
		a.states[id] = state
	}
	return a.totals()
}

// forget drops downloads that have left the queue.
func (a *activity) forget(ids []string) (unfinished, busy int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, id := range ids {
		delete(a.states, id)
	}
	return a.totals()
}

func (a *activity) totals() (unfinished, busy int) {
	for _, state := range a.states {
		unfinished++
		if state.Busy() {
			busy++
		}
	}
	return unfinished, busy
}

// reflect puts the totals where they show: the badge, and whether the Mac is
// held awake. A paused download keeps its badge but lets the Mac sleep.
func reflectActivity(unfinished, busy int) {
	setDockBadge(unfinished)
	keepAwake(busy > 0)
}
