package core

import (
	"sync"
	"testing"
	"time"
)

// recorder captures what actually reached the UI.
type recorder struct {
	mu   sync.Mutex
	seen []Progress
	ids  []string
}

func (r *recorder) emit(id string, p Progress) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ids = append(r.ids, id)
	r.seen = append(r.seen, p)
}

func (r *recorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.seen)
}

func (r *recorder) last() Progress {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.seen[len(r.seen)-1]
}

// newTestEmitter returns an emitter driven by a clock the test controls.
func newTestEmitter(interval time.Duration) (*ProgressEmitter, *recorder, *time.Time) {
	clock := time.Unix(0, 0)
	rec := &recorder{}
	e := NewProgressEmitter(interval, rec.emit)
	e.now = func() time.Time { return clock }
	return e, rec, &clock
}

func TestEmitterPassesFirstUpdateStraightThrough(t *testing.T) {
	e, rec, _ := newTestEmitter(DefaultProgressInterval)

	e.Update("a", Progress{Percent: 1})
	if rec.count() != 1 {
		t.Fatalf("emitted %d updates, want the first one through immediately", rec.count())
	}
}

func TestEmitterCoalescesInsideWindow(t *testing.T) {
	e, rec, clock := newTestEmitter(150 * time.Millisecond)

	e.Update("a", Progress{Percent: 1})
	for i := 2; i <= 50; i++ {
		*clock = clock.Add(time.Millisecond)
		e.Update("a", Progress{Percent: float64(i)})
	}

	// 50 updates inside one window must collapse to the single first emit.
	if rec.count() != 1 {
		t.Errorf("emitted %d updates, want 1", rec.count())
	}
}

func TestEmitterResumesAfterWindow(t *testing.T) {
	e, rec, clock := newTestEmitter(150 * time.Millisecond)

	e.Update("a", Progress{Percent: 1})
	*clock = clock.Add(150 * time.Millisecond)
	e.Update("a", Progress{Percent: 2})

	if rec.count() != 2 {
		t.Errorf("emitted %d updates, want 2 once the window elapsed", rec.count())
	}
	if rec.last().Percent != 2 {
		t.Errorf("last percent = %v, want 2", rec.last().Percent)
	}
}

func TestEmitterKeepsOnlyNewestPending(t *testing.T) {
	e, rec, clock := newTestEmitter(150 * time.Millisecond)

	e.Update("a", Progress{Percent: 1})
	*clock = clock.Add(10 * time.Millisecond)
	e.Update("a", Progress{Percent: 50})
	e.Update("a", Progress{Percent: 60})
	e.Update("a", Progress{Percent: 70})

	e.Flush("a")

	if rec.count() != 2 {
		t.Fatalf("emitted %d updates, want the first plus one flush", rec.count())
	}
	if got := rec.last().Percent; got != 70 {
		t.Errorf("flushed percent = %v, want the newest value 70", got)
	}
}

// TestFlushDeliversFinalProgress is the bug this design exists to prevent: a
// download that finishes mid-window must not leave the UI stuck below 100%.
func TestFlushDeliversFinalProgress(t *testing.T) {
	e, rec, clock := newTestEmitter(150 * time.Millisecond)

	e.Update("a", Progress{Percent: 12})
	*clock = clock.Add(5 * time.Millisecond)
	e.Update("a", Progress{Percent: 100, Downloaded: 500, Total: 500})

	if rec.last().Percent == 100 {
		t.Fatal("test is not exercising the throttle: the final update was not held")
	}

	e.Flush("a")

	if got := rec.last().Percent; got != 100 {
		t.Errorf("after Flush the UI shows %v%%, want 100", got)
	}
}

func TestFlushWithNothingPendingDoesNothing(t *testing.T) {
	e, rec, _ := newTestEmitter(150 * time.Millisecond)

	e.Update("a", Progress{Percent: 1})
	e.Flush("a")
	e.Flush("a")

	if rec.count() != 1 {
		t.Errorf("emitted %d updates, want no duplicates from empty flushes", rec.count())
	}
}

func TestEmitterThrottlesEachItemSeparately(t *testing.T) {
	// A busy playlist must not let one item's traffic mute another's.
	e, rec, _ := newTestEmitter(150 * time.Millisecond)

	e.Update("a", Progress{Percent: 1})
	e.Update("b", Progress{Percent: 1})
	e.Update("c", Progress{Percent: 1})
	e.Update("a", Progress{Percent: 2})

	if rec.count() != 3 {
		t.Errorf("emitted %d updates, want one per item", rec.count())
	}
}

func TestForgetDropsState(t *testing.T) {
	e, rec, _ := newTestEmitter(150 * time.Millisecond)

	e.Update("a", Progress{Percent: 1})
	e.Forget("a")
	// After Forget the item is new again, so its next update goes straight out.
	e.Update("a", Progress{Percent: 2})

	if rec.count() != 2 {
		t.Errorf("emitted %d updates, want the item treated as new after Forget", rec.count())
	}
	// Forget takes the emitter's lock, so inspect state only after it returns.
	e.Forget("a")
	e.mu.Lock()
	_, leftover := e.state["a"]
	e.mu.Unlock()
	if leftover {
		t.Error("Forget left state behind")
	}
}

func TestEmitterIsConcurrencySafe(t *testing.T) {
	rec := &recorder{}
	e := NewProgressEmitter(time.Millisecond, rec.emit)

	var wg sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				e.Update(id, Progress{Percent: float64(i % 100)})
			}
			e.Flush(id)
			e.Forget(id)
		}(string(rune('a' + worker)))
	}
	wg.Wait()

	if rec.count() == 0 {
		t.Error("no updates survived concurrent use")
	}
}

func TestZeroIntervalEmitsEverything(t *testing.T) {
	e, rec, _ := newTestEmitter(0)
	for i := 0; i < 10; i++ {
		e.Update("a", Progress{Percent: float64(i)})
	}
	if rec.count() != 10 {
		t.Errorf("emitted %d updates, want all 10 with throttling disabled", rec.count())
	}
}

func TestDefaultIntervalIsInTheAgreedRange(t *testing.T) {
	if DefaultProgressInterval < 100*time.Millisecond || DefaultProgressInterval > 250*time.Millisecond {
		t.Errorf("DefaultProgressInterval = %v, want 100-250ms", DefaultProgressInterval)
	}
}
