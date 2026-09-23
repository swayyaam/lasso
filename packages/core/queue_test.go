package core

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
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
	return newQueueHarnessWith(t, runner, concurrency, nil)
}

// newQueueHarnessWith is newQueueHarness plus a tagger, for the tests that care
// about what happens to split-out tracks.
func newQueueHarnessWith(t *testing.T, runner Runner, concurrency int, tagger Tagger, extra ...func(*QueueConfig)) *queueHarness {
	t.Helper()
	h := &queueHarness{states: map[string][]State{}}
	h.cond = sync.NewCond(&h.mu)

	cfg := QueueConfig{
		Runner:      runner,
		Tagger:      tagger,
		Concurrency: concurrency,
		// Emit every update so tests see exact values rather than racing a window.
		ProgressInterval: time.Nanosecond,
		OnState: func(item Item) {
			h.mu.Lock()
			h.states[item.ID] = append(h.states[item.ID], item.State)
			h.cond.Broadcast()
			h.mu.Unlock()
		},
	}
	for _, edit := range extra {
		edit(&cfg)
	}
	q, err := NewQueue(cfg)
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

	item, err := h.q.Add(uniqueOptions(), "Known Title")
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
	item, _ := h.q.Add(uniqueOptions(), "Already Known")
	h.waitFor(t, item.ID, StateDone)

	if n := metadataCalls.Load(); n != 0 {
		t.Errorf("resolved metadata %d times despite knowing the title", n)
	}
}

func TestQueueResolvesMissingTitle(t *testing.T) {
	h := newQueueHarness(t, succeedingRunner(), 1)

	item, _ := h.q.Add(uniqueOptions(), "")
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
		item, _ := h.q.Add(uniqueOptions(), "t")
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
	first, _ := h.q.Add(uniqueOptions(), "running")
	h.waitFor(t, first.ID, StateDownloading)

	// This one is still waiting its turn.
	waiting, _ := h.q.Add(uniqueOptions(), "waiting")
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
	item, _ := h.q.Add(uniqueOptions(), "t")

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
	item, _ := h.q.Add(uniqueOptions(), "t")
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
	item, _ := h.q.Add(uniqueOptions(), "t")
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
	running, _ := h.q.Add(uniqueOptions(), "t")
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
	item, _ := h.q.Add(uniqueOptions(), "t")
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
		item, _ := h.q.Add(uniqueOptions(), "t")
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
	if _, err := h.q.Add(uniqueOptions(), "t"); err != nil {
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
	if _, err := h.q.Add(uniqueOptions(), "t"); err == nil {
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
	item, _ := h.q.Add(uniqueOptions(), "t")
	h.waitFor(t, item.ID, StateDownloading)

	if err := h.q.Cancel(item.ID); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	h.waitFor(t, item.ID, StateCancelled)
}

// subtitleOptions is a request that asks for subtitles.
func subtitleOptions() Options {
	o := baseOptions()
	o.Subtitles = Subtitles{Download: true, Embed: true, Languages: []string{"en"}}
	return o
}

// wantsSubs reports whether a yt-dlp invocation asked for subtitles.
func wantsSubs(args []string) bool {
	return slices.Contains(args, "--write-subs") || slices.Contains(args, "--embed-subs")
}

// subtitleFailingRunner fails any attempt that asks for subtitles, the way a
// rate-limited subtitle endpoint does, and succeeds otherwise.
func subtitleFailingRunner(attempts *atomic.Int32, withSubs *atomic.Int32) *funcRunner {
	return &funcRunner{run: func(_ context.Context, args []string, stdout, stderr func(string)) error {
		if isMetadataCall(args) {
			return nil
		}
		attempts.Add(1)
		if wantsSubs(args) {
			withSubs.Add(1)
			stderr("ERROR: Unable to download video subtitles for 'en': HTTP Error 429: Too Many Requests")
			return errors.New("exit status 1")
		}
		stdout(`{"stage":"downloading","downloaded":100,"total":100,"estimate":0,"speed":1,"eta":0,"fragment":0,"fragments":0}`)
		return nil
	}}
}

// newSubtitleHarness wires a queue with a negligible retry delay.
func newSubtitleHarness(t *testing.T, runner Runner) *queueHarness {
	t.Helper()
	h := &queueHarness{states: map[string][]State{}}
	h.cond = sync.NewCond(&h.mu)

	q, err := NewQueue(QueueConfig{
		Runner:             runner,
		Concurrency:        1,
		ProgressInterval:   time.Nanosecond,
		SubtitleRetryDelay: time.Millisecond,
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

// TestSubtitleFailureDoesNotFailTheDownload is the point of the whole
// mechanism: yt-dlp treats a failed subtitle fetch as fatal, and losing a
// finished video because a caption file was rate-limited is the wrong trade.
func TestSubtitleFailureDoesNotFailTheDownload(t *testing.T) {
	var attempts, withSubs atomic.Int32
	h := newSubtitleHarness(t, subtitleFailingRunner(&attempts, &withSubs))

	item, err := h.q.Add(subtitleOptions(), "t")
	if err != nil {
		t.Fatal(err)
	}
	h.waitFor(t, item.ID, StateDone)

	final, _ := h.q.Get(item.ID)
	if final.State != StateDone {
		t.Fatalf("State = %q, want done", final.State)
	}
	if final.Notice == "" {
		t.Error("no notice explaining that subtitles were skipped")
	}
	if !strings.Contains(final.Notice, "subtitle") {
		t.Errorf("Notice = %q, want it to mention subtitles", final.Notice)
	}
	if !strings.Contains(final.Detail, "Unable to download video subtitles") {
		t.Errorf("Detail = %q, want the reason kept", final.Detail)
	}
	if final.Message != "" {
		t.Errorf("Message = %q, want no failure message on a successful item", final.Message)
	}
}

// TestSubtitlesAreRetriedOnceBeforeBeingDropped covers the backoff: a
// rate-limited endpoint often clears within seconds, so subtitles are worth
// one more try before giving up on them.
func TestSubtitlesAreRetriedOnceBeforeBeingDropped(t *testing.T) {
	var attempts, withSubs atomic.Int32
	h := newSubtitleHarness(t, subtitleFailingRunner(&attempts, &withSubs))

	item, _ := h.q.Add(subtitleOptions(), "t")
	h.waitFor(t, item.ID, StateDone)

	if got := withSubs.Load(); got != 2 {
		t.Errorf("tried subtitles %d times, want 2 (the first attempt and one retry)", got)
	}
	if got := attempts.Load(); got != 3 {
		t.Errorf("ran %d downloads, want 3 (two with subtitles, one without)", got)
	}
}

// TestSubtitleRetrySucceedsOnSecondAttempt covers the case the retry exists
// for: the endpoint recovers and the subtitles are kept.
func TestSubtitleRetrySucceedsOnSecondAttempt(t *testing.T) {
	var withSubs atomic.Int32
	runner := &funcRunner{run: func(_ context.Context, args []string, stdout, stderr func(string)) error {
		if isMetadataCall(args) {
			return nil
		}
		if wantsSubs(args) && withSubs.Add(1) == 1 {
			stderr("ERROR: Unable to download video subtitles for 'en': HTTP Error 429: Too Many Requests")
			return errors.New("exit status 1")
		}
		stdout(`{"stage":"downloading","downloaded":100,"total":100,"estimate":0,"speed":1,"eta":0,"fragment":0,"fragments":0}`)
		return nil
	}}

	h := newSubtitleHarness(t, runner)
	item, _ := h.q.Add(subtitleOptions(), "t")
	h.waitFor(t, item.ID, StateDone)

	final, _ := h.q.Get(item.ID)
	if final.Notice != "" {
		t.Errorf("Notice = %q, want none when the retry kept the subtitles", final.Notice)
	}
	if got := withSubs.Load(); got != 2 {
		t.Errorf("subtitle attempts = %d, want 2", got)
	}
}

// TestNonSubtitleFailureStillFails guards the blast radius: only subtitle
// failures are salvaged, never a broken video.
func TestNonSubtitleFailureStillFails(t *testing.T) {
	var attempts atomic.Int32
	runner := &funcRunner{run: func(_ context.Context, args []string, _, stderr func(string)) error {
		if isMetadataCall(args) {
			return nil
		}
		attempts.Add(1)
		stderr("ERROR: [youtube] abc: Video unavailable")
		return errors.New("exit status 1")
	}}

	h := newSubtitleHarness(t, runner)
	item, _ := h.q.Add(subtitleOptions(), "t")
	h.waitFor(t, item.ID, StateFailed)

	final, _ := h.q.Get(item.ID)
	if final.ErrorKind != ErrUnavailable {
		t.Errorf("ErrorKind = %q, want unavailable", final.ErrorKind)
	}
	if got := attempts.Load(); got != 1 {
		t.Errorf("ran %d downloads, want 1: a broken video must not be retried as a subtitle problem", got)
	}
}

// TestSubtitleSalvageStillFailsWhenTheVideoIsBroken covers a download that
// fails for a second reason once subtitles are dropped.
func TestSubtitleSalvageStillFailsWhenTheVideoIsBroken(t *testing.T) {
	runner := &funcRunner{run: func(_ context.Context, args []string, _, stderr func(string)) error {
		if isMetadataCall(args) {
			return nil
		}
		if wantsSubs(args) {
			stderr("ERROR: Unable to download video subtitles for 'en': HTTP Error 429: Too Many Requests")
		} else {
			stderr("ERROR: unable to download webpage: connection refused")
		}
		return errors.New("exit status 1")
	}}

	h := newSubtitleHarness(t, runner)
	item, _ := h.q.Add(subtitleOptions(), "t")
	h.waitFor(t, item.ID, StateFailed)

	final, _ := h.q.Get(item.ID)
	if final.ErrorKind != ErrNetwork {
		t.Errorf("ErrorKind = %q, want the real failure after subtitles were dropped", final.ErrorKind)
	}
}

// TestRequestsWithoutSubtitlesAreNotRetried keeps the mechanism from firing on
// downloads that never asked for subtitles.
func TestRequestsWithoutSubtitlesAreNotRetried(t *testing.T) {
	var attempts atomic.Int32
	runner := &funcRunner{run: func(_ context.Context, args []string, _, stderr func(string)) error {
		if isMetadataCall(args) {
			return nil
		}
		attempts.Add(1)
		stderr("ERROR: Unable to download video subtitles for 'en': HTTP Error 429")
		return errors.New("exit status 1")
	}}

	h := newSubtitleHarness(t, runner)
	item, _ := h.q.Add(uniqueOptions(), "t") // no subtitles requested
	h.waitFor(t, item.ID, StateFailed)

	if got := attempts.Load(); got != 1 {
		t.Errorf("ran %d downloads, want 1", got)
	}
}

func TestRetryClearsTheSubtitleNotice(t *testing.T) {
	var attempts, withSubs atomic.Int32
	h := newSubtitleHarness(t, subtitleFailingRunner(&attempts, &withSubs))

	item, _ := h.q.Add(subtitleOptions(), "t")
	h.waitFor(t, item.ID, StateDone)

	if final, _ := h.q.Get(item.ID); final.Notice == "" {
		t.Fatal("expected a notice to clear")
	}
	// A finished item cannot be retried, so check the field is reset on the
	// path that can: a failed one.
	h.q.mu.Lock()
	h.q.items[item.ID].State = StateFailed
	h.q.mu.Unlock()

	if err := h.q.Retry(item.ID); err != nil {
		t.Fatalf("Retry: %v", err)
	}
	if final, _ := h.q.Get(item.ID); final.Notice != "" {
		t.Errorf("Notice = %q, want it cleared on retry", final.Notice)
	}
}

func TestIsSubtitleFailure(t *testing.T) {
	yes := []string{
		"ERROR: Unable to download video subtitles for 'en': HTTP Error 429: Too Many Requests",
		"ERROR: unable to download subtitles",
		"Error downloading subtitles: something",
		"ERROR: Unable to extract subtitles",
	}
	no := []string{
		"ERROR: [youtube] abc: Video unavailable",
		"ERROR: unable to download webpage",
		"",
		"HTTP Error 429: Too Many Requests",
	}

	for _, s := range yes {
		if !IsSubtitleFailure(s) {
			t.Errorf("IsSubtitleFailure(%q) = false, want true", s)
		}
	}
	for _, s := range no {
		if IsSubtitleFailure(s) {
			t.Errorf("IsSubtitleFailure(%q) = true, want false", s)
		}
	}
}

func TestWithoutSubtitles(t *testing.T) {
	o := subtitleOptions()
	if !o.WantsSubtitles() {
		t.Fatal("WantsSubtitles = false for a request that asks for them")
	}

	stripped := o.WithoutSubtitles()
	if stripped.WantsSubtitles() {
		t.Error("WithoutSubtitles left subtitle options behind")
	}
	if hasFlag(BuildArgs(stripped), "--write-subs") || hasFlag(BuildArgs(stripped), "--embed-subs") {
		t.Error("stripped options still produce subtitle flags")
	}
	// The original must be untouched.
	if !o.WantsSubtitles() {
		t.Error("WithoutSubtitles mutated its receiver")
	}
	// Everything else survives.
	if stripped.Pick != o.Pick || stripped.URL != o.URL {
		t.Error("WithoutSubtitles changed more than the subtitles")
	}
}

func TestFinishedItemCarriesThePostProcessedFile(t *testing.T) {
	// yt-dlp writes one file, a post-processor replaces it with another, and
	// deletes the first. The item must end up naming the survivor.
	runner := &funcRunner{run: func(_ context.Context, args []string, stdout, _ func(string)) error {
		if isMetadataCall(args) {
			stdout(`{"id":"x","title":"Song"}`)
			return nil
		}
		stdout(`[download] Destination: /tmp/Song.webm`)
		stdout(`{"stage":"downloading","downloaded":10,"total":10}`)
		stdout(`[ExtractAudio] Destination: /tmp/Song.mp3`)
		stdout(`{"stage":"complete","path":"/tmp/Song.mp3"}`)
		return nil
	}}

	h := newQueueHarness(t, runner, 1)
	item, err := h.q.Add(Options{URL: "https://example.com/v", Pick: PickAudioMP3}, "Song")
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	h.waitFor(t, item.ID, StateDone)

	done, _ := h.q.Get(item.ID)
	if done.FilePath != "/tmp/Song.mp3" {
		t.Errorf("FilePath = %q, want the file that still exists", done.FilePath)
	}
}

func TestRetryClearsTheFinishedFile(t *testing.T) {
	var attempt atomic.Int32
	runner := &funcRunner{run: func(_ context.Context, args []string, stdout, stderr func(string)) error {
		if isMetadataCall(args) {
			stdout(`{"id":"x","title":"Clip"}`)
			return nil
		}
		if attempt.Add(1) == 1 {
			stdout(`{"stage":"complete","path":"/tmp/Clip.mp4"}`)
			return nil
		}
		stderr("ERROR: Video unavailable")
		return errors.New("exit status 1")
	}}

	h := newQueueHarness(t, runner, 1)
	item, _ := h.q.Add(Options{URL: "https://example.com/v", Pick: PickBest}, "Clip")
	h.waitFor(t, item.ID, StateDone)

	// A finished item cannot be retried, so cancel-then-retry is not available
	// either; force the item back through a failure to prove the field resets.
	h.q.mu.Lock()
	h.q.items[item.ID].State = StateFailed
	h.q.mu.Unlock()

	if err := h.q.Retry(item.ID); err != nil {
		t.Fatalf("Retry: %v", err)
	}
	h.waitFor(t, item.ID, StateFailed)

	failed, _ := h.q.Get(item.ID)
	if failed.FilePath != "" {
		t.Errorf("FilePath = %q, want a failed retry not to keep pointing at the old file", failed.FilePath)
	}
}

func TestRemoveOnlyTakesFinishedItems(t *testing.T) {
	// A running item has a download reporting into it; removing it would leave
	// that report with nowhere to go.
	block := make(chan struct{})
	runner := &funcRunner{run: func(ctx context.Context, args []string, stdout, _ func(string)) error {
		if isMetadataCall(args) {
			stdout(`{"id":"x","title":"Clip"}`)
			return nil
		}
		select {
		case <-block:
		case <-ctx.Done():
		}
		return nil
	}}

	h := newQueueHarness(t, runner, 1)
	item, _ := h.q.Add(Options{URL: "https://example.com/v", Pick: PickBest}, "Clip")
	h.waitFor(t, item.ID, StateDownloading)

	if err := h.q.Remove(item.ID); err == nil {
		t.Error("Remove accepted an item that was still running")
	}
	if _, ok := h.q.Get(item.ID); !ok {
		t.Error("the item was removed anyway")
	}

	close(block)
	h.waitFor(t, item.ID, StateDone)

	if err := h.q.Remove(item.ID); err != nil {
		t.Fatalf("Remove of a finished item: %v", err)
	}
	if _, ok := h.q.Get(item.ID); ok {
		t.Error("Remove left the item in the queue")
	}
}

func TestClearFinishedLeavesWorkAlone(t *testing.T) {
	block := make(chan struct{})
	runner := &funcRunner{run: func(ctx context.Context, args []string, stdout, stderr func(string)) error {
		if isMetadataCall(args) {
			stdout(`{"id":"x","title":"Clip"}`)
			return nil
		}
		// Only the first item is held open; the concurrency limit keeps the
		// rest queued behind it.
		select {
		case <-block:
		case <-ctx.Done():
		}
		return nil
	}}

	h := newQueueHarness(t, runner, 1)
	running, _ := h.q.Add(Options{URL: "https://example.com/1", Pick: PickBest}, "Running")
	h.waitFor(t, running.ID, StateDownloading)

	queued, _ := h.q.Add(Options{URL: "https://example.com/2", Pick: PickBest}, "Queued")
	cancelled, _ := h.q.Add(Options{URL: "https://example.com/3", Pick: PickBest}, "Cancelled")
	if err := h.q.Cancel(cancelled.ID); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	h.waitFor(t, cancelled.ID, StateCancelled)

	if got := h.q.ClearFinished(); got != 1 {
		t.Errorf("ClearFinished removed %d, want only the cancelled one", got)
	}
	if _, ok := h.q.Get(cancelled.ID); ok {
		t.Error("the cancelled item survived")
	}
	if _, ok := h.q.Get(running.ID); !ok {
		t.Error("ClearFinished removed a running item")
	}
	if _, ok := h.q.Get(queued.ID); !ok {
		t.Error("ClearFinished removed a queued item")
	}

	close(block)
}

func TestRemovalIsAnnounced(t *testing.T) {
	// A removal is the one change the state callback cannot describe: there is
	// no item left to send.
	var removed []string
	var mu sync.Mutex

	q, err := NewQueue(QueueConfig{
		Runner:   &funcRunner{run: func(context.Context, []string, func(string), func(string)) error { return nil }},
		OnRemove: func(ids []string) { mu.Lock(); removed = append(removed, ids...); mu.Unlock() },
	})
	if err != nil {
		t.Fatalf("NewQueue: %v", err)
	}
	defer q.Close()

	item, _ := q.Add(Options{URL: "https://example.com/v", Pick: PickBest}, "Clip")
	q.mu.Lock()
	q.items[item.ID].State = StateDone
	q.mu.Unlock()

	if err := q.Remove(item.ID); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if !slices.Contains(removed, item.ID) {
		t.Errorf("removed = %v, want it to name the item", removed)
	}
}

func TestRemovingKeepsTheRestInOrder(t *testing.T) {
	q, err := NewQueue(QueueConfig{
		Runner: &funcRunner{run: func(context.Context, []string, func(string), func(string)) error { return nil }},
	})
	if err != nil {
		t.Fatalf("NewQueue: %v", err)
	}
	defer q.Close()

	var ids []string
	for i := range 3 {
		item, _ := q.Add(Options{URL: "https://example.com/" + strconv.Itoa(i), Pick: PickBest}, "Clip")
		ids = append(ids, item.ID)
	}

	q.mu.Lock()
	q.items[ids[1]].State = StateDone
	q.mu.Unlock()
	if err := q.Remove(ids[1]); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	var got []string
	for _, item := range q.Items() {
		got = append(got, item.ID)
	}
	if !slices.Equal(got, []string{ids[0], ids[2]}) {
		t.Errorf("order = %v, want the middle item gone and the rest in place", got)
	}
}

func TestPauseIsNotCancel(t *testing.T) {
	// Both kill the process group, so the only thing separating them is what
	// the queue records afterwards. Getting this wrong loses the download.
	block := make(chan struct{})
	var attempts atomic.Int32

	runner := &funcRunner{run: func(ctx context.Context, args []string, stdout, _ func(string)) error {
		if isMetadataCall(args) {
			stdout(`{"id":"x","title":"Clip"}`)
			return nil
		}
		attempts.Add(1)
		stdout(`{"stage":"downloading","downloaded":50,"total":100}`)
		select {
		case <-block:
			stdout(`{"stage":"complete","path":"/tmp/Clip.mp4"}`)
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}}

	h := newQueueHarness(t, runner, 1)
	item, _ := h.q.Add(Options{URL: "https://example.com/v", Pick: PickBest}, "Clip")
	h.waitFor(t, item.ID, StateDownloading)

	if err := h.q.Pause(item.ID); err != nil {
		t.Fatalf("Pause: %v", err)
	}
	h.waitFor(t, item.ID, StatePaused)

	paused, _ := h.q.Get(item.ID)
	if paused.State != StatePaused {
		t.Fatalf("State = %q, want %q", paused.State, StatePaused)
	}
	if paused.ErrorKind != "" {
		t.Errorf("ErrorKind = %q, want a pause not to be recorded as a failure", paused.ErrorKind)
	}
	// The bar should still say where it got to, or resuming looks like starting.
	if paused.Progress.Downloaded != 50 {
		t.Errorf("Downloaded = %d, want the progress so far to survive", paused.Progress.Downloaded)
	}

	close(block)
	if err := h.q.Resume(item.ID); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	h.waitFor(t, item.ID, StateDone)

	if got := attempts.Load(); got != 2 {
		t.Errorf("ran %d times, want the resume to have started a second attempt", got)
	}
}

func TestPausedItemIsNotDispatched(t *testing.T) {
	// A paused item must not be picked up again on its own; only Resume queues
	// it. Otherwise pausing would do nothing but restart the download.
	var started atomic.Int32
	runner := &funcRunner{run: func(_ context.Context, args []string, stdout, _ func(string)) error {
		if isMetadataCall(args) {
			stdout(`{"id":"x","title":"Clip"}`)
			return nil
		}
		started.Add(1)
		return nil
	}}

	// Dispatching is deliberately not started yet: racing Pause against the
	// dispatcher would make this test pass or fail on timing rather than on
	// the behaviour it is checking.
	q, err := NewQueue(QueueConfig{Runner: runner, ProgressInterval: time.Nanosecond})
	if err != nil {
		t.Fatalf("NewQueue: %v", err)
	}
	defer q.Close()

	item, _ := q.Add(Options{URL: "https://example.com/v", Pick: PickBest}, "Clip")
	if err := q.Pause(item.ID); err != nil {
		t.Fatalf("Pause: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	q.Start(ctx)
	// Now give the dispatcher every chance to pick it up.
	time.Sleep(50 * time.Millisecond)

	paused, _ := q.Get(item.ID)
	if paused.State != StatePaused {
		t.Fatalf("State = %q, want %q", paused.State, StatePaused)
	}
	if got := started.Load(); got != 0 {
		t.Errorf("the download ran %d times while paused, want 0", got)
	}

	// And it does run once resumed, so the test above is not passing because
	// nothing was ever dispatchable.
	if err := q.Resume(item.ID); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	waitUntil(t, func() bool { return started.Load() == 1 }, "the resumed download to start")
}

func TestPauseRefusesAFinishedDownload(t *testing.T) {
	runner := &funcRunner{run: func(_ context.Context, args []string, stdout, _ func(string)) error {
		if isMetadataCall(args) {
			stdout(`{"id":"x","title":"Clip"}`)
		}
		return nil
	}}

	h := newQueueHarness(t, runner, 1)
	item, _ := h.q.Add(Options{URL: "https://example.com/v", Pick: PickBest}, "Clip")
	h.waitFor(t, item.ID, StateDone)

	if err := h.q.Pause(item.ID); err == nil {
		t.Error("Pause accepted a download that had already finished")
	}
	if err := h.q.Resume(item.ID); err == nil {
		t.Error("Resume accepted a download that was never paused")
	}
}

func TestPausedItemCanBeRemoved(t *testing.T) {
	// It is not running, so there is nothing for removal to strand.
	runner := &funcRunner{run: func(context.Context, []string, func(string), func(string)) error { return nil }}

	q, err := NewQueue(QueueConfig{Runner: runner})
	if err != nil {
		t.Fatalf("NewQueue: %v", err)
	}
	defer q.Close()

	item, _ := q.Add(Options{URL: "https://example.com/v", Pick: PickBest}, "Clip")
	if err := q.Pause(item.ID); err != nil {
		t.Fatalf("Pause: %v", err)
	}
	if err := q.Remove(item.ID); err != nil {
		t.Errorf("Remove of a paused item: %v", err)
	}
}

func TestPauseMarkDoesNotOutliveTheDownload(t *testing.T) {
	// Pause marks the id and then kills the process. A download that finishes
	// in between is never told it was paused, so the mark has to be cleared
	// anyway — otherwise the next cancellation of this item reads as a pause.
	runner := &funcRunner{run: func(_ context.Context, args []string, stdout, _ func(string)) error {
		if isMetadataCall(args) {
			stdout(`{"id":"x","title":"Clip"}`)
		}
		return nil
	}}

	h := newQueueHarness(t, runner, 1)
	item, _ := h.q.Add(Options{URL: "https://example.com/v", Pick: PickBest}, "Clip")
	h.waitFor(t, item.ID, StateDone)

	// Stand in for the race: the mark is set, but the run has already ended.
	h.q.mu.Lock()
	h.q.pausing[item.ID] = true
	h.q.items[item.ID].State = StateFailed
	h.q.mu.Unlock()

	if err := h.q.Retry(item.ID); err != nil {
		t.Fatalf("Retry: %v", err)
	}
	h.waitFor(t, item.ID, StateDone)

	// The state is announced from inside the run; the mark is cleared just
	// after it returns, so this waits rather than assuming an order.
	waitUntil(t, func() bool {
		h.q.mu.Lock()
		defer h.q.mu.Unlock()
		return !h.q.pausing[item.ID]
	}, "the pause mark to be cleared once the run ended")
}

// waitUntil polls for a condition, failing the test if it never holds.
func waitUntil(t *testing.T, cond func() bool, what string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Errorf("timed out waiting for %s", what)
}

func TestTheSameDownloadIsNotQueuedTwiceWhileRunning(t *testing.T) {
	block := make(chan struct{})
	runner := &funcRunner{run: func(ctx context.Context, args []string, stdout, _ func(string)) error {
		if isMetadataCall(args) {
			return nil
		}
		select {
		case <-block:
		case <-ctx.Done():
		}
		return nil
	}}
	h := newQueueHarnessWith(t, runner, 1, nil)
	defer close(block)

	o := Options{URL: "https://example.com/v", Pick: PickBest}
	if _, err := h.q.Add(o, "Clip"); err != nil {
		t.Fatalf("first Add: %v", err)
	}
	if _, err := h.q.Add(o, "Clip"); err == nil {
		t.Error("an identical download was queued while the first was still going")
	}

	// A different quality of the same link is a real thing to want.
	o720 := o
	o720.Pick = Pick720p
	if _, err := h.q.Add(o720, "Clip"); err != nil {
		t.Errorf("the same link at another quality was refused: %v", err)
	}
}

// uniqueOptions is baseOptions with a URL no other call returns.
//
// The queue refuses a second identical download while the first is running,
// so tests that queue several items to exercise concurrency, cancellation or
// ordering each need their own link — which is also what they stand for.
var uniqueCounter atomic.Int64

func uniqueOptions() Options {
	o := baseOptions()
	o.URL = fmt.Sprintf("https://example.com/watch?v=%d", uniqueCounter.Add(1))
	return o
}

// statesOf returns every state an item has been reported in, in order.
func (h *queueHarness) statesOf(id string) []State {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]State(nil), h.states[id]...)
}
