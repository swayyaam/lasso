package core

import (
	"context"
	"errors"
	"os"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// funcRunner lets each test script yt-dlp's behaviour precisely.
type funcRunner struct {
	run func(ctx context.Context, args []string, stdout, stderr func(string)) error
}

func (f *funcRunner) Run(ctx context.Context, args []string, stdout, stderr func(string)) error {
	return f.run(ctx, args, stdout, stderr)
}

// isMetadataCall reports whether the queue is resolving a title rather than
// downloading.
func isMetadataCall(args []string) bool { return slices.Contains(args, "-J") }

// queueHarness wires a queue to a runner and records every state change.
type queueHarness struct {
	q      *Queue
	cancel context.CancelFunc

	mu     sync.Mutex
	states map[string][]State
	cond   *sync.Cond
}

func newQueueHarness(t *testing.T, runner Runner, concurrency int) *queueHarness {
	t.Helper()
	h := &queueHarness{states: map[string][]State{}}
	h.cond = sync.NewCond(&h.mu)

	q, err := NewQueue(QueueConfig{
		Runner:      runner,
		Concurrency: concurrency,
		// Emit every update so tests see exact values rather than racing a window.
		ProgressInterval: time.Nanosecond,
		OnState: func(item Item) {
			h.mu.Lock()
			h.states[item.ID] = append(h.states[item.ID], item.State)
			h.cond.Broadcast()
			h.mu.Unlock()
		},
	})
	if err != nil {
		t.Fatalf("NewQueue: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	q.Start(ctx)

	h.q, h.cancel = q, cancel
	t.Cleanup(func() { cancel(); q.Close() })
	return h
}

// waitFor blocks until an item reaches a state, or the test times out.
func (h *queueHarness) waitFor(t *testing.T, id string, want State) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)

	done := make(chan struct{})
	go func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		for !slices.Contains(h.states[id], want) {
			if time.Now().After(deadline) {
				break
			}
			h.mu.Unlock()
			time.Sleep(2 * time.Millisecond)
			h.mu.Lock()
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(12 * time.Second):
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	if !slices.Contains(h.states[id], want) {
		t.Fatalf("item %s never reached %q; saw %v", id, want, h.states[id])
	}
}

// succeedingRunner emits a normal download and exits cleanly.
func succeedingRunner() *funcRunner {
	return &funcRunner{run: func(_ context.Context, args []string, stdout, _ func(string)) error {
		if isMetadataCall(args) {
			stdout(`{"_type":"video","id":"x","title":"Resolved Title"}`)
			return nil
		}
		stdout("[download] Destination: video.mp4")
		stdout(`{"stage":"downloading","downloaded":50,"total":100,"estimate":0,"speed":1000,"eta":1,"fragment":0,"fragments":0}`)
		stdout(`{"stage":"downloading","downloaded":100,"total":100,"estimate":0,"speed":1000,"eta":0,"fragment":0,"fragments":0}`)
		stdout(`[Merger] Merging formats into "video.mp4"`)
		return nil
	}}
}

func TestQueueRunsItemToCompletion(t *testing.T) {
	h := newQueueHarness(t, succeedingRunner(), 1)

	item, err := h.q.Add(baseOptions(), "Known Title")
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	h.waitFor(t, item.ID, StateDone)

	final, _ := h.q.Get(item.ID)
	if final.Progress.Percent != 100 {
		t.Errorf("final percent = %v, want 100", final.Progress.Percent)
	}
	if final.Message != "" {
		t.Errorf("a successful download reported %q", final.Message)
	}
}

func TestQueueSkipsFetchingWhenTitleKnown(t *testing.T) {
	var metadataCalls atomic.Int32
	runner := &funcRunner{run: func(_ context.Context, args []string, stdout, _ func(string)) error {
		if isMetadataCall(args) {
			metadataCalls.Add(1)
			stdout(`{"_type":"video","id":"x","title":"T"}`)
			return nil
		}
		return nil
	}}

	h := newQueueHarness(t, runner, 1)
	item, _ := h.q.Add(baseOptions(), "Already Known")
	h.waitFor(t, item.ID, StateDone)

	if n := metadataCalls.Load(); n != 0 {
		t.Errorf("resolved metadata %d times despite knowing the title", n)
	}
}

func TestQueueResolvesMissingTitle(t *testing.T) {
	h := newQueueHarness(t, succeedingRunner(), 1)

	item, _ := h.q.Add(baseOptions(), "")
	h.waitFor(t, item.ID, StateDone)

	final, _ := h.q.Get(item.ID)
	if final.Title != "Resolved Title" {
		t.Errorf("Title = %q, want the resolved title", final.Title)
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	if !slices.Contains(h.states[item.ID], StateFetching) {
		t.Errorf("never passed through fetching: %v", h.states[item.ID])
	}
}

func TestQueueRespectsConcurrencyLimit(t *testing.T) {
	var inFlight, peak atomic.Int32
	release := make(chan struct{})

	runner := &funcRunner{run: func(ctx context.Context, _ []string, _, _ func(string)) error {
		n := inFlight.Add(1)
		for {
			old := peak.Load()
			if n <= old || peak.CompareAndSwap(old, n) {
				break
			}
		}
		defer inFlight.Add(-1)
		select {
		case <-release:
		case <-ctx.Done():
		}
		return nil
	}}

	h := newQueueHarness(t, runner, 2)
	var ids []string
	for i := 0; i < 6; i++ {
		item, _ := h.q.Add(baseOptions(), "t")
		ids = append(ids, item.ID)
	}

	// Give the dispatcher time to start as many as it is willing to.
	time.Sleep(300 * time.Millisecond)
	if got := peak.Load(); got > 2 {
		t.Errorf("ran %d downloads at once, want at most 2", got)
	}

	close(release)
	for _, id := range ids {
		h.waitFor(t, id, StateDone)
	}
	if got := peak.Load(); got > 2 {
		t.Errorf("peak concurrency was %d, want at most 2", got)
	}
}

func TestQueueConcurrencyIsConfigurable(t *testing.T) {
	q, err := NewQueue(QueueConfig{Runner: succeedingRunner()})
	if err != nil {
		t.Fatal(err)
	}
	if got := q.Concurrency(); got != DefaultConcurrency {
		t.Errorf("default concurrency = %d, want %d", got, DefaultConcurrency)
	}

	q.SetConcurrency(4)
	if got := q.Concurrency(); got != 4 {
		t.Errorf("concurrency = %d, want 4", got)
	}
	// Out-of-range values are clamped rather than rejected.
	q.SetConcurrency(0)
	if got := q.Concurrency(); got != DefaultConcurrency {
		t.Errorf("concurrency = %d, want the default for 0", got)
	}
	q.SetConcurrency(999)
	if got := q.Concurrency(); got != MaxConcurrency {
		t.Errorf("concurrency = %d, want it clamped to %d", got, MaxConcurrency)
	}
}

func TestQueueCancelsQueuedItem(t *testing.T) {
	release := make(chan struct{})
	defer close(release)

	runner := &funcRunner{run: func(ctx context.Context, _ []string, _, _ func(string)) error {
		select {
		case <-release:
		case <-ctx.Done():
		}
		return nil
	}}

	h := newQueueHarness(t, runner, 1)
	first, _ := h.q.Add(baseOptions(), "running")
	h.waitFor(t, first.ID, StateDownloading)

	// This one is still waiting its turn.
	waiting, _ := h.q.Add(baseOptions(), "waiting")
	if err := h.q.Cancel(waiting.ID); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	h.waitFor(t, waiting.ID, StateCancelled)
}

func TestQueueCancelsRunningItem(t *testing.T) {
	started := make(chan struct{})
	runner := &funcRunner{run: func(ctx context.Context, _ []string, _, _ func(string)) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	}}

	h := newQueueHarness(t, runner, 1)
	item, _ := h.q.Add(baseOptions(), "t")

	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("download never started")
	}

	if err := h.q.Cancel(item.ID); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	h.waitFor(t, item.ID, StateCancelled)

	final, _ := h.q.Get(item.ID)
	if final.State != StateCancelled {
		t.Errorf("State = %q, want cancelled", final.State)
	}
}

func TestQueueCancelUnknownItem(t *testing.T) {
	h := newQueueHarness(t, succeedingRunner(), 1)
	if err := h.q.Cancel("nope"); err == nil {
		t.Error("Cancel accepted an unknown id")
	}
}

func TestQueueClassifiesFailure(t *testing.T) {
	runner := &funcRunner{run: func(_ context.Context, _ []string, _, stderr func(string)) error {
		stderr("ERROR: [youtube] abc: Video unavailable")
		return errors.New("exit status 1")
	}}

	h := newQueueHarness(t, runner, 1)
	item, _ := h.q.Add(baseOptions(), "t")
	h.waitFor(t, item.ID, StateFailed)

	final, _ := h.q.Get(item.ID)
	if final.ErrorKind != ErrUnavailable {
		t.Errorf("ErrorKind = %q, want unavailable", final.ErrorKind)
	}
	if final.Message == "" {
		t.Error("no plain-language message for a failure")
	}
	if final.Detail == "" {
		t.Error("raw output was not kept for the details toggle")
	}
}

func TestQueueRetriesFailedItem(t *testing.T) {
	var attempts atomic.Int32
	runner := &funcRunner{run: func(_ context.Context, args []string, stdout, stderr func(string)) error {
		if isMetadataCall(args) {
			return nil
		}
		if attempts.Add(1) == 1 {
			stderr("ERROR: Unable to download webpage: connection refused")
			return errors.New("exit status 1")
		}
		stdout(`{"stage":"downloading","downloaded":100,"total":100,"estimate":0,"speed":1,"eta":0,"fragment":0,"fragments":0}`)
		return nil
	}}

	h := newQueueHarness(t, runner, 1)
	item, _ := h.q.Add(baseOptions(), "t")
	h.waitFor(t, item.ID, StateFailed)

	if err := h.q.Retry(item.ID); err != nil {
		t.Fatalf("Retry: %v", err)
	}
	h.waitFor(t, item.ID, StateDone)

	final, _ := h.q.Get(item.ID)
	if final.Message != "" || final.ErrorKind != "" {
		t.Errorf("retry left the old failure behind: %q / %q", final.Message, final.ErrorKind)
	}
	if attempts.Load() != 2 {
		t.Errorf("attempts = %d, want 2", attempts.Load())
	}
}

func TestQueueRetryRejectsWrongStates(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	runner := &funcRunner{run: func(ctx context.Context, _ []string, _, _ func(string)) error {
		select {
		case <-release:
		case <-ctx.Done():
		}
		return nil
	}}

	h := newQueueHarness(t, runner, 1)
	running, _ := h.q.Add(baseOptions(), "t")
	h.waitFor(t, running.ID, StateDownloading)

	if err := h.q.Retry(running.ID); err == nil {
		t.Error("Retry accepted a running item")
	}
	if err := h.q.Retry("nope"); err == nil {
		t.Error("Retry accepted an unknown id")
	}
}

func TestQueueRetryRejectsFinishedItem(t *testing.T) {
	h := newQueueHarness(t, succeedingRunner(), 1)
	item, _ := h.q.Add(baseOptions(), "t")
	h.waitFor(t, item.ID, StateDone)

	if err := h.q.Retry(item.ID); err == nil {
		t.Error("Retry accepted an item that already finished")
	}
}

func TestQueueAddValidatesOptions(t *testing.T) {
	h := newQueueHarness(t, succeedingRunner(), 1)

	bad := baseOptions()
	bad.URL = "file:///etc/passwd"
	if _, err := h.q.Add(bad, "t"); err == nil {
		t.Error("Add accepted an invalid URL")
	}
	if len(h.q.Items()) != 0 {
		t.Error("an invalid item was added to the queue")
	}
}

func TestQueuePreservesOrder(t *testing.T) {
	h := newQueueHarness(t, succeedingRunner(), 1)

	var added []string
	for i := 0; i < 5; i++ {
		item, _ := h.q.Add(baseOptions(), "t")
		added = append(added, item.ID)
	}

	var got []string
	for _, item := range h.q.Items() {
		got = append(got, item.ID)
	}
	if !slices.Equal(added, got) {
		t.Errorf("order = %v, want %v", got, added)
	}
}

func TestQueueCloseCancelsInFlight(t *testing.T) {
	started := make(chan struct{})
	var stopped atomic.Bool

	runner := &funcRunner{run: func(ctx context.Context, _ []string, _, _ func(string)) error {
		select {
		case <-started:
		default:
			close(started)
		}
		<-ctx.Done()
		stopped.Store(true)
		return ctx.Err()
	}}

	h := newQueueHarness(t, runner, 1)
	if _, err := h.q.Add(baseOptions(), "t"); err != nil {
		t.Fatal(err)
	}

	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("download never started")
	}

	h.q.Close()
	if !stopped.Load() {
		t.Error("Close returned while a download was still running")
	}
	if _, err := h.q.Add(baseOptions(), "t"); err == nil {
		t.Error("Add succeeded after Close")
	}
}

func TestQueueNeedsRunner(t *testing.T) {
	if _, err := NewQueue(QueueConfig{}); err == nil {
		t.Error("NewQueue accepted a nil runner")
	}
}

func TestStateIsTerminal(t *testing.T) {
	terminal := []State{StateDone, StateFailed, StateCancelled}
	active := []State{StateQueued, StateFetching, StateDownloading, StatePostProcessing}

	for _, s := range terminal {
		if !s.IsTerminal() {
			t.Errorf("%q should be terminal", s)
		}
	}
	for _, s := range active {
		if s.IsTerminal() {
			t.Errorf("%q should not be terminal", s)
		}
	}
}

// TestQueueWithRealProcessCancellation runs the queue against a real process
// tree, confirming the whole group dies rather than just the direct child.
func TestQueueWithRealProcessCancellation(t *testing.T) {
	if _, err := os.Stat("/bin/sh"); err != nil {
		t.Skip("no shell available")
	}

	runner := &ExecRunner{Path: "/bin/sh"}
	// Stand in for yt-dlp spawning ffmpeg.
	wrapped := &funcRunner{run: func(ctx context.Context, _ []string, stdout, stderr func(string)) error {
		return runner.Run(ctx, []string{"-c", `sleep 300 & echo "[download] Destination: x.mp4"; wait`}, stdout, stderr)
	}}

	h := newQueueHarness(t, wrapped, 1)
	item, _ := h.q.Add(baseOptions(), "t")
	h.waitFor(t, item.ID, StateDownloading)

	if err := h.q.Cancel(item.ID); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	h.waitFor(t, item.ID, StateCancelled)
}
