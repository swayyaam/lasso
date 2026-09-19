package core

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"sync"
	"time"
)

// State is where a queue item stands.
type State string

const (
	StateQueued         State = "queued"
	StateFetching       State = "fetching"
	StateDownloading    State = "downloading"
	StatePostProcessing State = "post-processing"
	// StatePaused is stopped but resumable. It is not terminal: yt-dlp leaves
	// its .part file in place and picks up where it left off.
	StatePaused    State = "paused"
	StateDone      State = "done"
	StateFailed    State = "failed"
	StateCancelled State = "cancelled"
)

// IsTerminal reports whether a state is final: nothing further happens without
// the user asking for it.
func (s State) IsTerminal() bool {
	return s == StateDone || s == StateFailed || s == StateCancelled
}

// DefaultConcurrency is how many downloads run at once out of the box.
const DefaultConcurrency = 2

// MaxConcurrency bounds what the setting can be raised to. Beyond this, sites
// start rate-limiting and everything gets slower.
const MaxConcurrency = 8

// Item is one entry in the download queue.
type Item struct {
	ID       string   `json:"id"`
	Options  Options  `json:"options"`
	Title    string   `json:"title"`
	State    State    `json:"state"`
	Progress Progress `json:"progress"`
	// AddedAt is Unix milliseconds rather than a time.Time: the frontend gets
	// a real number from the generated bindings instead of an untyped value.
	AddedAt int64 `json:"addedAt"`

	// Message is a plain-language explanation when the item failed.
	Message string `json:"message"`
	// Detail is yt-dlp's raw output, shown behind a details toggle.
	Detail string `json:"detail"`
	// ErrorKind lets the UI distinguish failures that are worth retrying.
	ErrorKind ErrorKind `json:"errorKind"`

	// Notice is a caveat about a download that otherwise succeeded — currently
	// only that subtitles had to be skipped. Detail carries the reason.
	Notice string `json:"notice"`

	// FilePath is the file yt-dlp actually produced, known only once the
	// download is done. It is what "Show in Finder" reveals, and it is not
	// Progress.Filename: that names the file being written, which any
	// post-processor replaces and deletes.
	FilePath string `json:"filePath"`
}

// QueueConfig configures a Queue.
type QueueConfig struct {
	Runner      Runner
	Concurrency int
	// ProgressInterval throttles progress callbacks per item. Zero uses
	// DefaultProgressInterval.
	ProgressInterval time.Duration
	// OnState is called whenever an item's state changes. It must not block.
	OnState func(Item)
	// OnProgress is called with throttled progress updates. It must not block.
	OnProgress func(id string, p Progress)
	// OnRemove is called with the ids that have left the queue. A removal is
	// the one change OnState cannot describe: there is no item left to send.
	OnRemove func(ids []string)
	// SubtitleRetryDelay is how long to wait before retrying a download whose
	// subtitles failed. Zero uses DefaultSubtitleRetryDelay.
	SubtitleRetryDelay time.Duration
	// Tagger rewrites the tags on tracks split out of a chaptered recording.
	// Without one, splitting still works but every track keeps the whole
	// recording's title, so the feature is off when this is nil.
	Tagger Tagger
}

// DefaultSubtitleRetryDelay is the pause before a second attempt at subtitles.
//
// The common cause is the site rate-limiting its subtitle endpoint, which a
// short wait often clears. Long enough to matter, short enough that a user
// watching the queue does not think it has stalled.
const DefaultSubtitleRetryDelay = 3 * time.Second

// Queue runs downloads with a bounded number in flight.
//
// Items are started in the order they were added. Cancelling kills the whole
// process group so no ffmpeg is left behind, and the queue is deliberately
// in-memory: quitting Lasso discards it.
type Queue struct {
	runner   Runner
	tagger   Tagger
	emitter  *ProgressEmitter
	onState  func(Item)
	onRemove func(ids []string)
	nextID   int
	// subsRetryIn is how long to wait before retrying a download whose
	// subtitles failed.
	subsRetryIn time.Duration

	mu      sync.Mutex
	cond    *sync.Cond
	limit   int
	running int
	closed  bool

	items   map[string]*Item
	order   []string
	cancels map[string]context.CancelFunc
	// pausing marks ids whose cancellation is a pause. The process is killed
	// the same way either way; this is what tells the two apart afterwards.
	pausing map[string]bool

	wg sync.WaitGroup
}

// NewQueue creates a queue. Call Start to begin dispatching.
func NewQueue(cfg QueueConfig) (*Queue, error) {
	if cfg.Runner == nil {
		return nil, fmt.Errorf("queue needs a runner")
	}
	concurrency := cfg.Concurrency
	if concurrency <= 0 {
		concurrency = DefaultConcurrency
	}
	if concurrency > MaxConcurrency {
		concurrency = MaxConcurrency
	}
	interval := cfg.ProgressInterval
	if interval == 0 {
		interval = DefaultProgressInterval
	}

	retryIn := cfg.SubtitleRetryDelay
	if retryIn == 0 {
		retryIn = DefaultSubtitleRetryDelay
	}

	q := &Queue{
		runner:      cfg.Runner,
		tagger:      cfg.Tagger,
		onState:     cfg.OnState,
		onRemove:    cfg.OnRemove,
		limit:       concurrency,
		subsRetryIn: retryIn,
		items:       map[string]*Item{},
		cancels:     map[string]context.CancelFunc{},
		pausing:     map[string]bool{},
	}
	q.cond = sync.NewCond(&q.mu)
	q.emitter = NewProgressEmitter(interval, func(id string, p Progress) {
		q.recordProgress(id, p)
		if cfg.OnProgress != nil {
			cfg.OnProgress(id, p)
		}
	})
	return q, nil
}

// Start begins dispatching. It returns immediately; the dispatcher runs until
// ctx is cancelled or Close is called.
func (q *Queue) Start(ctx context.Context) {
	q.wg.Add(1)
	go q.dispatch(ctx)

	// Unblock the dispatcher when the caller's context ends.
	go func() {
		<-ctx.Done()
		q.Close()
	}()
}

// Add puts a new download on the queue.
//
// Title may be empty, in which case the queue resolves it before downloading,
// which is what the fetching state covers.
func (q *Queue) Add(o Options, title string) (Item, error) {
	if err := o.Validate(); err != nil {
		return Item{}, err
	}

	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		return Item{}, fmt.Errorf("the queue is shutting down")
	}
	q.nextID++
	id := strconv.Itoa(q.nextID)
	item := &Item{
		ID:      id,
		Options: o,
		Title:   title,
		State:   StateQueued,
		AddedAt: time.Now().UnixMilli(),
	}
	q.items[id] = item
	q.order = append(q.order, id)
	snapshot := *item
	q.cond.Broadcast()
	q.mu.Unlock()

	q.notify(snapshot)
	return snapshot, nil
}

// Items returns a snapshot of the queue in the order items were added.
func (q *Queue) Items() []Item {
	q.mu.Lock()
	defer q.mu.Unlock()

	out := make([]Item, 0, len(q.order))
	for _, id := range q.order {
		if item, ok := q.items[id]; ok {
			out = append(out, *item)
		}
	}
	return out
}

// Get returns one item.
func (q *Queue) Get(id string) (Item, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	item, ok := q.items[id]
	if !ok {
		return Item{}, false
	}
	return *item, true
}

// SetConcurrency changes how many downloads run at once. Downloads already in
// flight are left alone; lowering the limit takes effect as they finish.
func (q *Queue) SetConcurrency(n int) {
	if n <= 0 {
		n = DefaultConcurrency
	}
	if n > MaxConcurrency {
		n = MaxConcurrency
	}
	q.mu.Lock()
	q.limit = n
	q.cond.Broadcast()
	q.mu.Unlock()
}

// Concurrency reports the current limit.
func (q *Queue) Concurrency() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.limit
}

// Cancel stops an item. A queued item is cancelled outright; a running one has
// its process group killed.
func (q *Queue) Cancel(id string) error {
	q.mu.Lock()
	item, ok := q.items[id]
	if !ok {
		q.mu.Unlock()
		return fmt.Errorf("no such download")
	}
	if item.State.IsTerminal() {
		q.mu.Unlock()
		return nil
	}

	cancel, running := q.cancels[id]
	if running {
		q.mu.Unlock()
		// The runner turns this into a process-group kill.
		cancel()
		return nil
	}

	item.State = StateCancelled
	item.Message = "Cancelled."
	snapshot := *item
	q.cond.Broadcast()
	q.mu.Unlock()

	q.notify(snapshot)
	return nil
}

// Retry puts a failed or cancelled item back on the queue.
func (q *Queue) Retry(id string) error {
	q.mu.Lock()
	item, ok := q.items[id]
	if !ok {
		q.mu.Unlock()
		return fmt.Errorf("no such download")
	}
	if !item.State.IsTerminal() {
		q.mu.Unlock()
		return fmt.Errorf("that download is still running")
	}
	if item.State == StateDone {
		q.mu.Unlock()
		return fmt.Errorf("that download already finished")
	}

	item.State = StateQueued
	item.Progress = Progress{}
	item.Message = ""
	item.Detail = ""
	item.ErrorKind = ""
	item.Notice = ""
	item.FilePath = ""
	snapshot := *item
	q.cond.Broadcast()
	q.mu.Unlock()

	q.emitter.Forget(id)
	q.notify(snapshot)
	return nil
}

// Pause stops a download so it can be picked up again later.
//
// There is no such thing as suspending yt-dlp usefully: holding the process
// open keeps the connection open, and sites time it out. So a pause is a stop
// that keeps its work — the process group is killed exactly as cancelling
// kills it, and the .part file it leaves behind is what resuming continues
// from.
func (q *Queue) Pause(id string) error {
	q.mu.Lock()
	item, ok := q.items[id]
	if !ok {
		q.mu.Unlock()
		return fmt.Errorf("no such download")
	}
	if item.State == StatePaused {
		q.mu.Unlock()
		return nil
	}
	if item.State.IsTerminal() {
		q.mu.Unlock()
		return fmt.Errorf("that download has already finished")
	}

	cancel, running := q.cancels[id]
	if !running {
		// Still waiting its turn: it can simply stop waiting.
		item.State = StatePaused
		item.Message = ""
		snapshot := *item
		q.cond.Broadcast()
		q.mu.Unlock()

		q.notify(snapshot)
		return nil
	}

	// Recorded before the kill so the run that is about to end knows which of
	// the two this was.
	q.pausing[id] = true
	q.mu.Unlock()

	cancel()
	return nil
}

// Resume puts a paused download back in line.
//
// It is queued rather than started outright, so resuming several at once still
// respects the concurrency limit.
func (q *Queue) Resume(id string) error {
	q.mu.Lock()
	item, ok := q.items[id]
	if !ok {
		q.mu.Unlock()
		return fmt.Errorf("no such download")
	}
	if item.State != StatePaused {
		q.mu.Unlock()
		return fmt.Errorf("that download is not paused")
	}

	item.State = StateQueued
	item.Message = ""
	snapshot := *item
	q.cond.Broadcast()
	q.mu.Unlock()

	q.notify(snapshot)
	return nil
}

// Remove drops an item from the queue.
//
// Anything not actually running can go — finished, paused, or still waiting
// its turn. Removing a running download would leave it reporting into nothing,
// so the caller cancels or pauses first and removes after.
func (q *Queue) Remove(id string) error {
	q.mu.Lock()
	if _, ok := q.items[id]; !ok {
		q.mu.Unlock()
		return fmt.Errorf("no such download")
	}
	if _, running := q.cancels[id]; running {
		// A running download would be left reporting into nothing. Finished,
		// paused and still-waiting items are not running, so they can go.
		q.mu.Unlock()
		return fmt.Errorf("that download is still running")
	}
	q.deleteLocked(id)
	q.mu.Unlock()

	q.emitter.Forget(id)
	q.notifyRemoved([]string{id})
	return nil
}

// ClearFinished drops every item that has finished, failed or been cancelled,
// and reports how many went. Running and queued items are left alone.
func (q *Queue) ClearFinished() int {
	q.mu.Lock()
	var removed []string
	for _, id := range slices.Clone(q.order) {
		if item, ok := q.items[id]; ok && item.State.IsTerminal() {
			q.deleteLocked(id)
			removed = append(removed, id)
		}
	}
	q.mu.Unlock()

	for _, id := range removed {
		q.emitter.Forget(id)
	}
	q.notifyRemoved(removed)
	return len(removed)
}

// deleteLocked drops an item and its place in the order. The caller must hold
// the lock.
func (q *Queue) deleteLocked(id string) {
	delete(q.items, id)
	q.order = slices.DeleteFunc(q.order, func(other string) bool { return other == id })
}

func (q *Queue) notifyRemoved(ids []string) {
	if len(ids) > 0 && q.onRemove != nil {
		q.onRemove(ids)
	}
}

// Close stops dispatching and cancels everything in flight.
func (q *Queue) Close() {
	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		return
	}
	q.closed = true
	cancels := make([]context.CancelFunc, 0, len(q.cancels))
	for _, cancel := range q.cancels {
		cancels = append(cancels, cancel)
	}
	q.cond.Broadcast()
	q.mu.Unlock()

	for _, cancel := range cancels {
		cancel()
	}
	q.wg.Wait()
}

// dispatch starts queued items as capacity allows.
func (q *Queue) dispatch(ctx context.Context) {
	defer q.wg.Done()

	for {
		q.mu.Lock()
		for !q.closed && (q.running >= q.limit || q.nextQueued() == "") {
			q.cond.Wait()
		}
		if q.closed {
			q.mu.Unlock()
			return
		}

		id := q.nextQueued()
		item := q.items[id]
		item.State = StateFetching
		if item.Title != "" {
			item.State = StateDownloading
		}
		snapshot := *item
		q.running++

		runCtx, cancel := context.WithCancel(ctx)
		q.cancels[id] = cancel
		q.mu.Unlock()

		q.notify(snapshot)

		q.wg.Add(1)
		go func(id string) {
			defer q.wg.Done()
			q.run(runCtx, id)

			q.mu.Lock()
			q.running--
			delete(q.cancels, id)
			// The run has read this already. Clearing it here covers the case
			// where it never did: a download that finished in the moment
			// between Pause marking it and the kill arriving would otherwise
			// leave the mark behind, and the next cancellation of the same
			// item would be read as a pause.
			delete(q.pausing, id)
			q.cond.Broadcast()
			q.mu.Unlock()
			cancel()
		}(id)
	}
}

// nextQueued returns the oldest queued item's id, or "" if there is none.
// The caller must hold the lock.
func (q *Queue) nextQueued() string {
	for _, id := range q.order {
		if item, ok := q.items[id]; ok && item.State == StateQueued {
			return id
		}
	}
	return ""
}

// run performs one download from start to finish.
func (q *Queue) run(ctx context.Context, id string) {
	item, ok := q.Get(id)
	if !ok {
		return
	}

	if item.Title == "" {
		if meta, err := FetchMetadata(ctx, q.runner, item.Options); err == nil {
			q.setTitle(id, meta.Title)
		}
		// A failed resolve is not fatal on its own: the download attempt below
		// produces a better error than a title lookup would.
		if ctx.Err() == nil {
			q.transition(id, StateDownloading, nil)
		}
	}

	err, result := q.attempt(ctx, id, item.Options)

	// A subtitle fetch that fails takes the whole download with it: yt-dlp
	// treats it as fatal and has no flag to ignore only that. Losing an
	// otherwise-finished video because a caption file 429'd is the wrong
	// trade, so it gets one retry and then continues without them.
	if err != nil && ctx.Err() == nil && item.Options.WantsSubtitles() && IsSubtitleFailure(result.Output) {
		if q.sleep(ctx, q.subsRetryIn) {
			err, result = q.attempt(ctx, id, item.Options)
		}

		if err != nil && ctx.Err() == nil && IsSubtitleFailure(result.Output) {
			reason := result.Output
			err, result = q.attempt(ctx, id, item.Options.WithoutSubtitles())
			if err == nil {
				q.retagChapters(ctx, id, item, result)
				q.finishWithNotice(id, result.FilePath, "Downloaded without subtitles", reason)
				return
			}
		}
	}

	if err != nil {
		q.fail(id, result.Output, err, ctx)
		return
	}

	q.retagChapters(ctx, id, item, result)
	q.finish(id, result.FilePath)
}

// retagChapters gives each split-out track its own title, number and album.
//
// yt-dlp cuts the tracks with the audio copied, which carries every tag across
// unchanged — so without this an album's twelve tracks are all titled after the
// album, and a music library shows twelve copies of one name.
//
// A failure here is reported on the item but does not fail the download: the
// tracks exist and play, and losing them over a metadata rewrite would be a bad
// trade. The notice is how the user learns the tags need a look.
func (q *Queue) retagChapters(ctx context.Context, id string, item Item, result attemptResult) {
	if q.tagger == nil || len(result.Chapters) == 0 || ctx.Err() != nil {
		return
	}

	album := item.Title
	total := len(result.Chapters)

	var failures []string
	for _, chapter := range result.Chapters {
		tags := TrackTags{
			Title:  chapter.Title(),
			Track:  chapter.Number,
			Tracks: total,
			Album:  album,
		}
		if err := q.tagger.Tag(ctx, chapter.Path, tags); err != nil {
			failures = append(failures, err.Error())
		}
	}

	if len(failures) == 0 {
		return
	}
	q.mu.Lock()
	if stored, ok := q.items[id]; ok {
		stored.Notice = fmt.Sprintf("Split into %d tracks, but %d could not be tagged", total, len(failures))
		stored.Detail = joinLines(failures)
	}
	q.mu.Unlock()
}

// attemptResult is what one run of yt-dlp produced.
type attemptResult struct {
	// Output is whatever yt-dlp wrote to stderr.
	Output string
	// FilePath is the file it finished with.
	FilePath string
	// Chapters are the per-track files --split-chapters wrote.
	Chapters []ChapterFile
}

// attempt runs one download and returns the failure, if any, along with what
// the run produced.
func (q *Queue) attempt(ctx context.Context, id string, o Options) (error, attemptResult) {
	parser := NewProgressParser()
	var errLines []string

	err := q.runner.Run(ctx, ExecArgs(o),
		func(line string) {
			if progress, ok := parser.Line(line); ok {
				q.emitter.Update(id, progress)
			}
		},
		func(line string) {
			if progress, ok := parser.Line(line); ok {
				q.emitter.Update(id, progress)
				return
			}
			errLines = append(errLines, line)
		},
	)

	// Whatever happened, the last progress value must reach the UI rather than
	// stay trapped inside a throttle window.
	q.emitter.Flush(id)
	return err, attemptResult{
		Output:   joinLines(errLines),
		FilePath: parser.OutputPath(),
		Chapters: parser.ChapterFiles(),
	}
}

// sleep waits unless the download is cancelled first, reporting whether the
// wait completed.
func (q *Queue) sleep(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}

func (q *Queue) fail(id, output string, err error, ctx context.Context) {
	downloadErr := ClassifyError(output, err)
	// A cancelled context reaches us as a plain exit error, because the process
	// was killed rather than failing on its own.
	if ctx.Err() != nil {
		downloadErr = ClassifyError(output, ctx.Err())
	}

	state := StateFailed
	if downloadErr.Kind == ErrCancelled {
		state = StateCancelled

		q.mu.Lock()
		paused := q.pausing[id]
		delete(q.pausing, id)
		q.mu.Unlock()

		if paused {
			// Whatever was transferred stays on disk, and the progress already
			// recorded stays on the item, so resuming shows where it got to.
			q.transition(id, StatePaused, nil)
			return
		}
	}
	q.transition(id, state, downloadErr)
}

// finishWithNotice completes an item that succeeded with a caveat.
func (q *Queue) finishWithNotice(id, filePath, notice, detail string) {
	q.mu.Lock()
	if item, ok := q.items[id]; ok {
		item.Notice = notice
		item.Detail = detail
	}
	q.mu.Unlock()

	q.finish(id, filePath)
}

func (q *Queue) finish(id, filePath string) {
	q.mu.Lock()
	item, ok := q.items[id]
	if !ok {
		q.mu.Unlock()
		return
	}
	item.FilePath = filePath
	item.State = StateDone
	item.Progress.Stage = StagePostProcessing
	item.Progress.Percent = 100
	item.Message = ""
	item.ErrorKind = ""
	snapshot := *item
	q.mu.Unlock()

	q.notify(snapshot)
}

func (q *Queue) transition(id string, state State, downloadErr *DownloadError) {
	q.mu.Lock()
	item, ok := q.items[id]
	if !ok {
		q.mu.Unlock()
		return
	}
	item.State = state
	if downloadErr != nil {
		item.Message = downloadErr.Message
		item.Detail = downloadErr.Raw
		item.ErrorKind = downloadErr.Kind
	}
	snapshot := *item
	q.mu.Unlock()

	q.notify(snapshot)
}

func (q *Queue) setTitle(id, title string) {
	if title == "" {
		return
	}
	q.mu.Lock()
	if item, ok := q.items[id]; ok {
		item.Title = title
	}
	q.mu.Unlock()
}

// recordProgress stores the latest progress on the item so a fresh snapshot
// carries it.
func (q *Queue) recordProgress(id string, p Progress) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if item, ok := q.items[id]; ok && !item.State.IsTerminal() {
		item.Progress = p
		switch p.Stage {
		case StageDownloading:
			item.State = StateDownloading
		case StagePostProcessing:
			item.State = StatePostProcessing
		}
	}
}

// notify calls the state callback. It is always called without the lock held,
// so a callback that reaches back into the queue cannot deadlock it.
func (q *Queue) notify(item Item) {
	if q.onState != nil {
		q.onState(item)
	}
}

func joinLines(lines []string) string {
	out := ""
	for i, l := range lines {
		if i > 0 {
			out += "\n"
		}
		out += l
	}
	return out
}
