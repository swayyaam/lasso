package main

import (
	"bufio"
	"context"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/swayyaam/lasso/packages/core"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// incoming holds a link that reached Lasso from outside the window — a
// lasso:// link, a link dropped on the Dock, a .webloc opened with Lasso, the
// File menu's Paste Link — until the interface collects it.
//
// It is held rather than only sent, because the first of these usually
// arrives before there is an interface to send it to: a lasso:// link that
// launches Lasso is delivered while the window is still loading. The frontend
// takes it once it is listening, and again each time it is told one waits.
type incoming struct {
	mu   sync.Mutex
	link string
}

// openExternal accepts a link from outside, brings the window forward, and
// tells the interface one is waiting. Anything IncomingLink refuses is
// dropped with a log line: there is no one to show an error to yet, and a
// page that sent a bad link is owed nothing.
func (a *App) openExternal(raw string) {
	link, err := core.IncomingLink(raw)
	if err != nil {
		log.Printf("lasso: ignored an incoming link: %v", err)
		return
	}

	a.incoming.mu.Lock()
	a.incoming.link = link
	a.incoming.mu.Unlock()

	if a.ctx != nil {
		runtime.WindowUnminimise(a.ctx)
		runtime.WindowShow(a.ctx)
		runtime.EventsEmit(a.ctx, EventLinkWaiting)
	}
}

// TakeIncomingLink returns the link waiting from outside the window, if any,
// and forgets it so it is opened once.
func (a *App) TakeIncomingLink() string {
	a.incoming.mu.Lock()
	defer a.incoming.mu.Unlock()
	link := a.incoming.link
	a.incoming.link = ""
	return link
}

// onURLOpen is macOS handing Lasso a URL: a lasso:// link, or a web link
// dropped on the Dock icon.
func (a *App) onURLOpen(url string) {
	a.openExternal(url)
}

// onFileOpen is a file opened with Lasso: a .webloc from Safari or the Finder,
// or a .url shortcut. Anything else is not something Lasso opens.
func (a *App) onFileOpen(path string) {
	link, err := shortcutLink(path)
	if err != nil {
		log.Printf("lasso: could not read a link from %s: %v", filepath.Base(path), err)
		return
	}
	a.openExternal(link)
}

// pasteLink is File › Paste Link: whatever link is on the clipboard, opened
// as if it had been pasted into the window.
func (a *App) pasteLink() {
	if a.ctx == nil {
		return
	}
	text, err := runtime.ClipboardGetText(a.ctx)
	if err != nil {
		return
	}
	a.openExternal(text)
}

// shortcutLimit bounds how much of a shortcut file is read. A real one is a
// few hundred bytes; anything larger is not a shortcut.
const shortcutLimit = 64 << 10

// shortcutLink reads the link out of a .webloc or .url file.
func shortcutLink(path string) (string, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".webloc":
		return weblocLink(path)
	case ".url":
		return internetShortcutLink(path)
	default:
		return "", core.ErrNotALink
	}
}

// weblocLink reads a .webloc, which is a property list with a URL key —
// binary when the Finder writes it, XML when Safari does. plutil reads both,
// and is part of macOS, so it is named by its full path rather than looked
// up on a PATH that belongs to the user.
func weblocLink(path string) (string, error) {
	if info, err := os.Stat(path); err != nil {
		return "", err
	} else if info.Size() > shortcutLimit {
		return "", core.ErrNotALink
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "/usr/bin/plutil", "-extract", "URL", "raw", "-o", "-", "--", path).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// internetShortcutLink reads a Windows .url file: an INI section with a
// URL= line.
func internetShortcutLink(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	scanner := bufio.NewScanner(io.LimitReader(f, shortcutLimit))
	for scanner.Scan() {
		if link, ok := strings.CutPrefix(strings.TrimSpace(scanner.Text()), "URL="); ok {
			return strings.TrimSpace(link), nil
		}
	}
	return "", core.ErrNotALink
}
