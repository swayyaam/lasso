package main

import (
	"strings"
	"testing"

	"github.com/swayyaam/lasso/packages/binaries"
	"github.com/swayyaam/lasso/packages/core"
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
		"Enqueue":      errOfItem(app.Enqueue(core.Options{}, "")),
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

func TestRevealInFinderRejectsMissingPaths(t *testing.T) {
	app := NewApp()

	if err := app.RevealInFinder(""); err == nil {
		t.Error("RevealInFinder accepted an empty path")
	}
	if err := app.RevealInFinder("/nope/not/here.mp4"); err == nil {
		t.Error("RevealInFinder accepted a path that does not exist")
	} else if !strings.Contains(err.Error(), "no longer there") {
		t.Errorf("error = %q, want plain language", err)
	}
}

type errFake string

func (e errFake) Error() string { return string(e) }

func errOf(_ presets.Preset, err error) error              { return err }
func errOfString(_ string, err error) error                { return err }
func errOfItem(_ core.Item, err error) error               { return err }
func errOfUpdate(_ binaries.UpdateResult, err error) error { return err }
