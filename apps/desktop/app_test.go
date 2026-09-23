package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/swayyaam/lasso/packages/binaries"
	"github.com/swayyaam/lasso/packages/core"
	"github.com/swayyaam/lasso/packages/history"
	"github.com/swayyaam/lasso/packages/presets"
)

// TestEventNamesMatchConstants guards the one thing that would silently break
// the frontend: Events() is how TypeScript learns which strings to subscribe
// to, so it must report exactly what the backend emits.
func TestEventNamesMatchConstants(t *testing.T) {
	got := NewApp().Events()

	if got.QueueItem != EventQueueItem {
		t.Errorf("QueueItem = %q, want %q", got.QueueItem, EventQueueItem)
	}
	if got.QueueProgress != EventQueueProgress {
		t.Errorf("QueueProgress = %q, want %q", got.QueueProgress, EventQueueProgress)
	}
	if got.BinaryStatus != EventBinaryStatus {
		t.Errorf("BinaryStatus = %q, want %q", got.BinaryStatus, EventBinaryStatus)
	}
	if got.SettingsChanged != EventSettingsChanged {
		t.Errorf("SettingsChanged = %q, want %q", got.SettingsChanged, EventSettingsChanged)
	}
}

func TestEventNamesAreDistinct(t *testing.T) {
	names := []string{EventQueueItem, EventQueueProgress, EventBinaryStatus, EventSettingsChanged}
	seen := map[string]bool{}
	for _, n := range names {
		if n == "" {
			t.Error("an event name is empty")
		}
		if seen[n] {
			t.Errorf("duplicate event name %q", n)
		}
		seen[n] = true
	}
}

// TestCallsBeforeStartupFailSafely covers the window between the window opening
// and startup finishing, and the case where the binaries never came up: every
// bound method must return a message rather than panic on a nil dependency.
func TestCallsBeforeStartupFailSafely(t *testing.T) {
	app := NewApp()

	if got := app.QueueItems(); len(got) != 0 {
		t.Errorf("QueueItems = %v, want empty", got)
	}
	if got := app.Presets(); len(got) != len(presets.Builtins()) {
		t.Errorf("Presets returned %d, want the built-ins", len(got))
	}
	if got := app.Settings(); got.Concurrency != core.DefaultConcurrency {
		t.Errorf("Settings = %+v, want defaults", got)
	}
	if got := app.Versions(); len(got) != 0 {
		t.Errorf("Versions = %v, want empty", got)
	}

	for name, err := range map[string]error{
		"Cancel":       app.Cancel("1"),
		"Retry":        app.Retry("1"),
		"SavePreset":   errOf(app.SavePreset("x", core.Options{})),
		"RenamePreset": app.RenamePreset("x", "y"),
		"UpdatePreset": app.UpdatePreset("x", core.Options{}),
		"DeletePreset": app.DeletePreset("x"),
		"ShowCommand":  errOfString(app.ShowCommand(core.Options{})),
		"Enqueue":      errOfItem(app.Enqueue(core.Options{}, core.Source{})),
		"Thumbnail":    errOfString(app.Thumbnail("https://example.com/x.jpg", 100)),
		"UpdateYtDlp":  errOfUpdate(app.UpdateYtDlp()),
		"ChooseFolder": errOfString(app.ChooseFolder()),
	} {
		if err == nil {
			t.Errorf("%s returned nil before startup, want an error", name)
		}
	}
}

func TestBinaryStatusReportsStartupFailure(t *testing.T) {
	app := NewApp()
	app.fail(errFake("binaries are missing"))

	status := app.BinaryStatus()
	if status.Ready {
		t.Error("Ready = true after a startup failure")
	}
	if len(status.Problems) == 0 {
		t.Fatal("no problem reported for a startup failure")
	}
	if status.Problems[0].Message == "" {
		t.Error("problem has no message for the UI")
	}
}

func TestOpenAndRevealTakeDownloadsNotPaths(t *testing.T) {
	// The whole point: a path handed over from the interface — here, the kind
	// something malicious in the page would try — resolves to nothing.
	app := NewApp()
	store, err := history.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	app.history = store

	for _, sneaky := range []string{"/System/Applications/Calculator.app", "/etc/hosts", "", "../../etc/hosts"} {
		if err := app.OpenFile(sneaky); err == nil {
			t.Errorf("OpenFile(%q) went ahead; it should only open a download Lasso knows", sneaky)
		}
		if err := app.RevealInFinder(sneaky); err == nil {
			t.Errorf("RevealInFinder(%q) went ahead", sneaky)
		}
	}

	// A real download resolves by its ID.
	file := filepath.Join(t.TempDir(), "clip.mp4")
	os.WriteFile(file, []byte("x"), 0o644)
	store.Add(history.Entry{ID: "abc123", Title: "clip", FilePath: file, State: core.StateDone})
	if got, err := app.fileOf("abc123"); err != nil || got != file {
		t.Errorf("fileOf(abc123) = %q, %v; want the entry's file", got, err)
	}

	// And one whose file has gone says so plainly.
	os.Remove(file)
	if _, err := app.fileOf("abc123"); err == nil || !strings.Contains(err.Error(), "no longer there") {
		t.Errorf("err = %v, want plain language about the missing file", err)
	}
}

func TestTheInterfaceCannotChooseTheDownloadFolder(t *testing.T) {
	o := fromInterface(core.Options{Output: core.Output{Folder: "/Users/someone/Library/LaunchAgents", Template: "%(title)s.%(ext)s"}})
	if o.Output.Folder != "" {
		t.Errorf("folder = %q, want it left to Settings", o.Output.Folder)
	}
	if o.Output.Template == "" {
		t.Error("the filename template, which the interface may set, was dropped")
	}
	if fromInterface(core.Options{Playback: core.Playback{AV1: true}}).Playback.AV1 {
		t.Error("the interface was allowed to say what this Mac plays")
	}
	got := Settings{DownloadFolder: "/Users/someone/Movies"}.ApplyTo(o)
	if got.Output.Folder != "/Users/someone/Movies" {
		t.Errorf("folder = %q, want the Settings folder", got.Output.Folder)
	}
}

type errFake string

func (e errFake) Error() string { return string(e) }

func errOf(_ presets.Preset, err error) error              { return err }
func errOfString(_ string, err error) error                { return err }
func errOfItem(_ core.Item, err error) error               { return err }
func errOfUpdate(_ binaries.UpdateResult, err error) error { return err }

// TestAssetMiddlewarePassesOtherPathsThrough confirms the middleware only
// claims thumbnail requests and leaves the rest of the app's assets alone.
func TestAssetMiddlewarePassesOtherPathsThrough(t *testing.T) {
	app := NewApp()
	var reached bool
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	})

	rec := httptest.NewRecorder()
	app.assetMiddleware(next).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/index.html", nil))

	if !reached {
		t.Error("middleware swallowed a request that was not for a thumbnail")
	}
}

func TestBrowserInstalledFindsSafari(t *testing.T) {
	// Safari ships with macOS, so on any Mac this test can run on it is there.
	// The point of the check is that it works without any permission grant:
	// /Applications is readable when a browser's data folder is not.
	if !browserInstalled(core.BrowserSafari) {
		t.Error("Safari not found; the application lookup is not working")
	}
}

func TestBrowserInstalledRejectsWhatItDoesNotKnow(t *testing.T) {
	if browserInstalled(core.BrowserNone) {
		t.Error("reported a browser for the no-cookies setting")
	}
	if browserInstalled(core.Browser("netscape")) {
		t.Error("reported a browser it has no bundle name for")
	}
}

func TestNotifyIsSafeOutsideAnAppBundle(t *testing.T) {
	// UNUserNotificationCenter raises rather than returning an error when
	// there is no bundle identifier, and a raise from Objective-C takes the
	// process down. `go test` is exactly that case, so this asserts the guard
	// holds — if it regresses, this test does not fail, it crashes the run.
	notify("A download finished", "Lasso")
	notify("", "")
}

func TestThisMacsPlaybackIsDetected(t *testing.T) {
	// No assertion about the answer — it depends on the chip — only that
	// asking VideoToolbox works and reports the same thing twice.
	if detectPlayback() != detectPlayback() {
		t.Error("detectPlayback is not stable")
	}
	t.Logf("this Mac decodes AV1 in hardware: %v", detectPlayback().AV1)
}
