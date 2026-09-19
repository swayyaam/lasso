package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/swayyaam/lasso/packages/binaries"
	"github.com/swayyaam/lasso/packages/core"
	"github.com/swayyaam/lasso/packages/doctor"
	"github.com/swayyaam/lasso/packages/history"
	"github.com/swayyaam/lasso/packages/presets"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App is the object Wails binds. Every exported method becomes callable from
// TypeScript, and every type they mention becomes a generated model.
//
// The interesting logic lives in packages/core, packages/binaries and
// packages/presets; this type is the adapter that turns their plain Go types
// into bound methods and events.
type App struct {
	ctx context.Context

	mu       sync.RWMutex
	manager  *binaries.Manager
	queue    *core.Queue
	presets  *presets.Store
	settings *SettingsStore
	history  *history.Store
	thumbs   *core.ThumbnailCache
	status   binaries.Status

	// startupErr records a failure that leaves the app unable to download, so
	// every call can report it rather than panicking on a nil dependency.
	startupErr error
}

// NewApp creates the application.
func NewApp() *App { return &App{} }

// startup runs when Wails is ready. It brings the sidecar binaries up, opens
// the stores, and starts the download queue.
//
// A failure here is reported through BinaryStatus rather than crashing: the
// settings screen still needs to open so the user can see what is wrong.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	support, err := binaries.SupportDir()
	if err != nil {
		a.fail(fmt.Errorf("could not find Lasso's application folder: %w", err))
		return
	}

	settingsStore, err := NewSettingsStore(support)
	if err != nil {
		a.fail(err)
		return
	}
	presetStore, err := presets.NewStore(support)
	if err != nil {
		a.fail(err)
		return
	}
	historyStore, err := history.NewStore(support)
	if err != nil {
		a.fail(err)
		return
	}

	a.mu.Lock()
	a.settings = settingsStore
	a.presets = presetStore
	a.history = historyStore
	a.thumbs = core.NewThumbnailCache(filepath.Join(support, "thumbnails"), core.DefaultThumbnailCacheBytes)
	a.mu.Unlock()

	manager, status, err := binaries.Startup(ctx)
	a.mu.Lock()
	a.manager = manager
	a.status = status
	a.mu.Unlock()

	runtime.EventsEmit(ctx, EventBinaryStatus, status)
	if err != nil {
		a.fail(err)
		return
	}
	if !status.Ready {
		// Not fatal: the UI shows the problems and offers to retry.
		return
	}

	if err := a.startQueue(ctx, manager, settingsStore.Get().Concurrency); err != nil {
		a.fail(err)
	}
}

func (a *App) startQueue(ctx context.Context, manager *binaries.Manager, concurrency int) error {
	queue, err := core.NewQueue(core.QueueConfig{
		Runner: &core.ExecRunner{
			Path: manager.Path(binaries.YtDlp),
			Env:  manager.Environ(),
		},
		Concurrency: concurrency,
		OnState: func(item core.Item) {
			runtime.EventsEmit(ctx, EventQueueItem, item)
			a.record(ctx, item)
			a.announce(item)
		},
		OnProgress: func(id string, p core.Progress) {
			runtime.EventsEmit(ctx, EventQueueProgress, ProgressEvent{ID: id, Progress: p})
		},
		OnRemove: func(ids []string) {
			runtime.EventsEmit(ctx, EventQueueRemoved, ids)
		},
	})
	if err != nil {
		return err
	}
	queue.Start(ctx)

	a.mu.Lock()
	a.queue = queue
	a.mu.Unlock()
	return nil
}

func (a *App) fail(err error) {
	a.mu.Lock()
	a.startupErr = err
	a.mu.Unlock()
}

// shutdown stops in-flight downloads so no ffmpeg outlives the window.
func (a *App) shutdown(context.Context) {
	a.mu.RLock()
	queue := a.queue
	a.mu.RUnlock()

	if queue != nil {
		queue.Close()
	}
}

// ---- Bound methods ----

// Events returns the event names the backend emits, so the frontend subscribes
// with the same strings rather than its own copies.
func (a *App) Events() EventNames {
	return EventNames{
		QueueItem:       EventQueueItem,
		QueueProgress:   EventQueueProgress,
		BinaryStatus:    EventBinaryStatus,
		SettingsChanged: EventSettingsChanged,
		QueueRemoved:    EventQueueRemoved,
		HistoryChanged:  EventHistoryChanged,
	}
}

// BinaryStatus reports whether the sidecar binaries are usable.
func (a *App) BinaryStatus() binaries.Status {
	a.mu.RLock()
	defer a.mu.RUnlock()

	status := a.status
	if a.startupErr != nil && len(status.Problems) == 0 {
		status.Problems = []binaries.Problem{{
			Message: binaries.UserMessage(a.startupErr),
			Detail:  a.startupErr.Error(),
		}}
	}
	return status
}

// Versions reports the version of each sidecar binary, for the settings screen.
func (a *App) Versions() map[binaries.Name]string {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.status.Versions == nil {
		return map[binaries.Name]string{}
	}
	return a.status.Versions
}

// FetchMetadata resolves a link.
//
// This is the call the UI shows a loading state for: it reaches the network and
// takes a second or two even when everything is warm.
func (a *App) FetchMetadata(url string) (*core.Metadata, error) {
	runner, settings, err := a.runnerAndSettings()
	if err != nil {
		return nil, err
	}

	o := settings.ApplyTo(core.Options{URL: url, Pick: core.PickBest})
	o.DenoPath = a.denoPath()
	return core.FetchMetadata(a.ctx, runner, o)
}

// Thumbnail returns a URL for a preview image, downloading and downscaling it
// to the requested width on first use. The URL is served by the app's own
// asset handler, never by the network.
//
// The frontend must ask for the width it is actually rendering rather than
// pointing an img tag at a remote URL: that keeps every outbound request in the
// backend and means an image is decoded once, not on every render.
func (a *App) Thumbnail(url string, width int) (string, error) {
	a.mu.RLock()
	cache := a.thumbs
	a.mu.RUnlock()

	if cache == nil {
		return "", fmt.Errorf("Lasso is still starting up")
	}

	path, err := cache.Get(a.ctx, url, width)
	if err != nil {
		return "", err
	}
	// The webview cannot load a file:// path from the asset scheme, so hand
	// back a URL the asset handler serves.
	return thumbnailURL(path), nil
}

// ShowCommand renders the yt-dlp command for a set of options, for the
// "show command" panel.
func (a *App) ShowCommand(o core.Options) (string, error) {
	a.mu.RLock()
	manager, settings := a.manager, a.settings
	a.mu.RUnlock()

	if manager == nil || settings == nil {
		return "", fmt.Errorf("Lasso is still starting up")
	}

	o = settings.Get().ApplyTo(o)
	o.FFmpegLocation = manager.FFmpegLocation()
	o.DenoPath = manager.Path(binaries.Deno)
	if err := o.Validate(); err != nil {
		return "", err
	}
	return core.ShowCommand(manager.Path(binaries.YtDlp), o), nil
}

// Enqueue adds a download to the queue.
func (a *App) Enqueue(o core.Options, title string) (core.Item, error) {
	a.mu.RLock()
	queue, manager, settings := a.queue, a.manager, a.settings
	a.mu.RUnlock()

	if queue == nil || manager == nil || settings == nil {
		return core.Item{}, fmt.Errorf("Lasso cannot download yet: check Settings for details")
	}

	o = settings.Get().ApplyTo(o)
	o.FFmpegLocation = manager.FFmpegLocation()
	o.DenoPath = manager.Path(binaries.Deno)
	o.ArcProfileDir = arcProfileDir()

	// Checked before the download starts rather than discovered halfway
	// through it, which wastes the transfer and leaves a part file behind.
	if err := checkDiskSpace(o.Output.Folder); err != nil {
		return core.Item{}, err
	}

	return queue.Add(o, title)
}

// QueueItems returns the whole queue, for the initial render and after a
// window reload.
func (a *App) QueueItems() []core.Item {
	a.mu.RLock()
	queue := a.queue
	a.mu.RUnlock()

	if queue == nil {
		return []core.Item{}
	}
	return queue.Items()
}

// Cancel stops a download, killing the whole process group so no ffmpeg is
// left behind.
func (a *App) Cancel(id string) error {
	a.mu.RLock()
	queue := a.queue
	a.mu.RUnlock()

	if queue == nil {
		return fmt.Errorf("nothing is downloading")
	}
	return queue.Cancel(id)
}

// Pause stops a download but keeps what it has transferred, so it can be
// picked up again later.
func (a *App) Pause(id string) error {
	a.mu.RLock()
	queue := a.queue
	a.mu.RUnlock()

	if queue == nil {
		return fmt.Errorf("nothing is downloading")
	}
	return queue.Pause(id)
}

// Resume puts a paused download back in line.
func (a *App) Resume(id string) error {
	a.mu.RLock()
	queue := a.queue
	a.mu.RUnlock()

	if queue == nil {
		return fmt.Errorf("nothing is paused")
	}
	return queue.Resume(id)
}

// Retry puts a failed or cancelled download back on the queue.
func (a *App) Retry(id string) error {
	a.mu.RLock()
	queue := a.queue
	a.mu.RUnlock()

	if queue == nil {
		return fmt.Errorf("nothing to retry")
	}
	return queue.Retry(id)
}

// RemoveFromQueue drops a finished download from the queue. Its history entry
// is left alone: clearing the queue is tidying, not forgetting.
func (a *App) RemoveFromQueue(id string) error {
	a.mu.RLock()
	queue := a.queue
	a.mu.RUnlock()

	if queue == nil {
		return fmt.Errorf("there is nothing to remove")
	}
	return queue.Remove(id)
}

// ClearFinished drops every finished, failed and cancelled download from the
// queue, and reports how many went.
func (a *App) ClearFinished() int {
	a.mu.RLock()
	queue := a.queue
	a.mu.RUnlock()

	if queue == nil {
		return 0
	}
	return queue.ClearFinished()
}

// ---- History ----

// History returns finished downloads, newest first.
func (a *App) History() []history.Entry {
	a.mu.RLock()
	store := a.history
	a.mu.RUnlock()

	if store == nil {
		return []history.Entry{}
	}
	return store.All()
}

// ForgetHistoryEntry removes one entry from the record.
func (a *App) ForgetHistoryEntry(id string) error {
	store, err := a.historyStore()
	if err != nil {
		return err
	}
	if err := store.Remove(id); err != nil {
		return err
	}
	a.emit(EventHistoryChanged)
	return nil
}

// ClearHistory empties the record.
func (a *App) ClearHistory() error {
	store, err := a.historyStore()
	if err != nil {
		return err
	}
	if err := store.Clear(); err != nil {
		return err
	}
	a.emit(EventHistoryChanged)
	return nil
}

// DownloadAgain puts a past download back on the queue with the options it
// originally ran with.
func (a *App) DownloadAgain(id string) (core.Item, error) {
	store, err := a.historyStore()
	if err != nil {
		return core.Item{}, err
	}

	for _, entry := range store.All() {
		if entry.ID == id {
			return a.Enqueue(entry.Options, entry.Title)
		}
	}
	return core.Item{}, fmt.Errorf("that download is no longer in your history")
}

// ---- Presets ----

// Presets returns the built-in presets followed by the user's own.
func (a *App) Presets() []presets.Preset {
	a.mu.RLock()
	store := a.presets
	a.mu.RUnlock()

	if store == nil {
		return presets.Builtins()
	}
	return store.All()
}

// SavePreset stores the given options under a new name.
func (a *App) SavePreset(name string, o core.Options) (presets.Preset, error) {
	store, err := a.presetStore()
	if err != nil {
		return presets.Preset{}, err
	}
	return store.Save(name, o)
}

// RenamePreset renames one of the user's presets.
func (a *App) RenamePreset(id, name string) error {
	store, err := a.presetStore()
	if err != nil {
		return err
	}
	return store.Rename(id, name)
}

// UpdatePreset replaces a user preset's options.
func (a *App) UpdatePreset(id string, o core.Options) error {
	store, err := a.presetStore()
	if err != nil {
		return err
	}
	return store.Update(id, o)
}

// DeletePreset removes one of the user's presets.
func (a *App) DeletePreset(id string) error {
	store, err := a.presetStore()
	if err != nil {
		return err
	}
	return store.Delete(id)
}

// ---- Settings ----

// Settings returns the current preferences.
func (a *App) Settings() Settings {
	a.mu.RLock()
	store := a.settings
	a.mu.RUnlock()

	if store == nil {
		return defaultSettings()
	}
	return store.Get()
}

// SaveSettings validates and stores new preferences, returning what was applied.
func (a *App) SaveSettings(settings Settings) (Settings, error) {
	a.mu.RLock()
	store, queue := a.settings, a.queue
	a.mu.RUnlock()

	if store == nil {
		return Settings{}, fmt.Errorf("Lasso is still starting up")
	}

	applied, err := store.Save(settings)
	if err != nil {
		return Settings{}, err
	}
	if queue != nil {
		queue.SetConcurrency(applied.Concurrency)
	}
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, EventSettingsChanged, applied)
	}
	return applied, nil
}

// ChooseFolder opens a folder picker and returns what was chosen, or an empty
// string if the user cancelled.
func (a *App) ChooseFolder() (string, error) {
	if a.ctx == nil {
		return "", fmt.Errorf("Lasso is still starting up")
	}
	return runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title:                "Choose a download folder",
		CanCreateDirectories: true,
	})
}

// RevealInFinder opens the enclosing folder of a finished download.
func (a *App) RevealInFinder(path string) error {
	if path == "" {
		return fmt.Errorf("that download has no file yet")
	}
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("that file is no longer there")
	}
	// Uses the system file manager, not a shell, so a filename cannot be
	// interpreted as anything but a path.
	return openInFinder(path)
}

// OpenFullDiskAccessSettings opens the System Settings pane that lets Lasso
// read a protected cookie jar. It is the remedy offered alongside a
// cookie-access failure.
func (a *App) OpenFullDiskAccessSettings() error {
	return openFullDiskAccessSettings()
}

// OpenFile opens a finished download in whichever app macOS uses for it.
//
// The counterpart to RevealInFinder: showing the file is what you want when
// you are about to move it, and opening it is what you want the rest of the
// time.
func (a *App) OpenFile(path string) error {
	if path == "" {
		return fmt.Errorf("that download has no file yet")
	}
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("that file is no longer there")
	}
	return openFile(path)
}

// ---- Diagnostics ----

// Diagnose runs every check and reports what it found.
//
// It is deliberately free of side effects, so the queue can offer it straight
// from a failed download without the offer itself changing anything.
func (a *App) Diagnose() (doctor.Report, error) {
	d, err := a.doctor()
	if err != nil {
		return doctor.Report{}, err
	}
	return d.Run(a.ctx), nil
}

// ApplyFix repairs one check and returns what it did, so the user can see that
// pressing the button had an effect.
func (a *App) ApplyFix(id string) (string, error) {
	d, err := a.doctor()
	if err != nil {
		return "", err
	}
	return d.Fix(a.ctx, id)
}

// ShouldSuggestDoctor reports whether a failure of this kind is worth offering
// diagnostics for. The frontend asks rather than deciding, so the rule has one
// definition and is testable in Go.
func (a *App) ShouldSuggestDoctor(kind core.ErrorKind) bool {
	return doctor.SuggestsDoctor(kind)
}

// doctor assembles a Doctor from the app's current state.
//
// It is built per call rather than held, because every input can change while
// the app is open — the folder and browser from settings, the binaries from an
// update — and a diagnosis of a stale configuration is worse than none.
func (a *App) doctor() (*doctor.Doctor, error) {
	a.mu.RLock()
	manager, settingsStore := a.manager, a.settings
	a.mu.RUnlock()

	if manager == nil || settingsStore == nil {
		return nil, fmt.Errorf("Lasso is still starting up")
	}
	settings := settingsStore.Get()

	return doctor.New(doctor.Config{
		Manager:          manager,
		DownloadFolder:   settings.DownloadFolder,
		Cookies:          settings.Cookies,
		FreeBytes:        freeBytes,
		MinimumFreeBytes: minimumFreeBytes,
		CookieProbe:      a.probeCookies,
		BrowserInstalled: browserInstalled,
	})
}

// probeCookies asks yt-dlp to open the browser's cookie jar.
//
// There is no way to answer this by inspection. Whether a jar can be read
// depends on macOS's permission state, on whether the browser holds a lock,
// and for Chromium on whether the keychain releases the decryption key — so
// the only honest test is the thing that will actually do it, failing against
// a site that costs nothing to ask.
func (a *App) probeCookies(ctx context.Context, browser core.Browser) error {
	runner, _, err := a.runnerAndSettings()
	if err != nil {
		return err
	}

	o := core.Options{URL: cookieProbeURL, Pick: core.PickBest}
	o.Network.Cookies = browser
	o.ArcProfileDir = arcProfileDir()
	o.DenoPath = a.denoPath()

	ctx, cancel := context.WithTimeout(ctx, cookieProbeTimeout)
	defer cancel()

	var errLines []string
	runErr := runner.Run(ctx, core.CookieProbeArgs(o),
		func(string) {},
		func(line string) { errLines = append(errLines, line) },
	)
	if runErr == nil {
		return nil
	}

	output := strings.Join(errLines, "\n")
	// Only a cookie problem is this check's business. The probe URL may be
	// unreachable, the site may be rate-limiting, the machine may be offline —
	// none of which says anything about whether the jar can be opened.
	if classified := core.ClassifyError(output, runErr); classified.Kind == core.ErrCookieAccess {
		return classified
	}
	return nil
}

// ---- yt-dlp updates ----

// UpdateYtDlp runs yt-dlp's self-update against the copy in Application
// Support and reports what happened.
func (a *App) UpdateYtDlp() (binaries.UpdateResult, error) {
	a.mu.RLock()
	manager := a.manager
	a.mu.RUnlock()

	if manager == nil {
		return binaries.UpdateResult{}, fmt.Errorf("Lasso is still starting up")
	}

	result, err := manager.UpdateYtDlp(a.ctx)
	if err != nil {
		return result, err
	}

	// Re-check everything so the settings screen shows the new version.
	status := binaries.Status{BinDir: manager.Paths().Bin, Versions: map[binaries.Name]string{}}
	for _, r := range manager.Verify(a.ctx) {
		if r.OK {
			status.Versions[r.Name] = r.Version
			continue
		}
		status.Problems = append(status.Problems, binaries.Problem{
			Name:    r.Name,
			Message: binaries.UserMessage(r.Err),
			Detail:  r.Err.Error(),
		})
	}
	status.Ready = len(status.Problems) == 0

	a.mu.Lock()
	a.status = status
	a.mu.Unlock()

	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, EventBinaryStatus, status)
	}
	return result, nil
}

// ---- helpers ----

// record writes a finished download into the history.
//
// It is driven off the queue's state callback rather than called at each
// finishing site, so there is one place that decides what counts as finished —
// and history.FromItem, not this, is what enforces it.
func (a *App) record(ctx context.Context, item core.Item) {
	entry, ok := history.FromItem(item)
	if !ok {
		return
	}

	a.mu.RLock()
	store := a.history
	a.mu.RUnlock()
	if store == nil {
		return
	}

	// A history write that fails must not take the download with it: the file
	// is on disk either way, and the user has already been told it finished.
	if _, err := store.Add(entry); err != nil {
		return
	}
	runtime.EventsEmit(ctx, EventHistoryChanged)
}

// announce posts a notification for a download that has ended.
//
// Only for outcomes worth interrupting someone over. A cancellation was the
// user's own doing a moment ago and needs no announcement, and a pause even
// less.
func (a *App) announce(item core.Item) {
	switch item.State {
	case core.StateDone:
		notify(item.Title, "Download finished")
	case core.StateFailed:
		notify(item.Title, "Download failed")
	}
}

func (a *App) historyStore() (*history.Store, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.history == nil {
		return nil, fmt.Errorf("Lasso is still starting up")
	}
	return a.history, nil
}

// emit sends an event when the app has a context to send it on.
func (a *App) emit(name string, data ...any) {
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, name, data...)
	}
}

func (a *App) presetStore() (*presets.Store, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.presets == nil {
		return nil, fmt.Errorf("Lasso is still starting up")
	}
	return a.presets, nil
}

func (a *App) runnerAndSettings() (core.Runner, Settings, error) {
	a.mu.RLock()
	manager, store := a.manager, a.settings
	a.mu.RUnlock()

	if manager == nil || store == nil {
		return nil, Settings{}, fmt.Errorf("Lasso cannot reach yt-dlp yet: check Settings for details")
	}
	runner := &core.ExecRunner{
		Path: manager.Path(binaries.YtDlp),
		Env:  manager.Environ(),
	}
	return runner, store.Get(), nil
}

// denoPath is the bundled JavaScript runtime yt-dlp needs for YouTube.
func (a *App) denoPath() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.manager == nil {
		return ""
	}
	return a.manager.Path(binaries.Deno)
}

// arcProfileDir is where Arc keeps its Chromium profile. yt-dlp has no "arc"
// browser, so Arc cookies are requested as Chromium with this path.
func arcProfileDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	dir := filepath.Join(home, "Library", "Application Support", "Arc", "User Data")
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return ""
	}
	return dir
}

// assetMiddleware serves the runtime files the embedded frontend does not
// contain — currently just cached thumbnails.
//
// This is middleware rather than the asset server's fallback Handler because
// the fallback is only reached when nothing else answers. Under `wails dev`
// the Vite dev server answers everything, including unknown paths, with the
// SPA index page — so a thumbnail request would come back as HTML and every
// preview would be blank in development. Middleware runs first, so the same
// code serves both modes.
func (a *App) assetMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, thumbURLPrefix) {
			next.ServeHTTP(w, r)
			return
		}

		a.mu.RLock()
		cache := a.thumbs
		a.mu.RUnlock()

		if cache == nil {
			http.NotFound(w, r)
			return
		}
		thumbnailHandler{dir: cache.Dir()}.ServeHTTP(w, r)
	})
}
