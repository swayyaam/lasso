package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/swayyaam/lasso/packages/core"
)

// openInFinder reveals a file in Finder.
//
// It uses exec with an argument slice rather than a shell, so a filename
// containing quotes, spaces or shell metacharacters is just a filename.
func openInFinder(path string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, "/usr/bin/open", "-R", "--", path).Run()
}

// openFile opens a file with whatever macOS considers its default app.
//
// Like openInFinder, it execs with an argument slice, so a filename containing
// shell metacharacters is just a filename. The "--" matters here too: a file
// whose name begins with a dash would otherwise be read as an option.
func openFile(path string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, "/usr/bin/open", "--", path).Run()
}

// fullDiskAccessURL opens System Settings at the pane that governs Safari's
// cookie container.
//
// Safari keeps its cookies inside a protected container, so no file permission
// makes them readable — only Full Disk Access does. Chrome and Firefox usually
// need nothing, which is why the error offers switching browsers as the other
// way out.
const fullDiskAccessURL = "x-apple.systempreferences:com.apple.preference.security?Privacy_AllFiles"

// openFullDiskAccessSettings takes the user to the setting that fixes an
// unreadable cookie jar, so the remedy is one click rather than a hunt.
func openFullDiskAccessSettings() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, "/usr/bin/open", fullDiskAccessURL).Run()
}

// browserApps maps a cookie source to the application bundles it could be.
//
// Looking for the application is how the doctor separates "that browser is not
// installed" from "macOS will not let Lasso read its data" — the two report
// identically through yt-dlp, because a protected folder answers a process
// without permission with "no such file" rather than "permission denied".
//
// /Applications is not itself protected, so this check works without any grant.
var browserApps = map[core.Browser][]string{
	core.BrowserSafari:  {"Safari.app"},
	core.BrowserChrome:  {"Google Chrome.app"},
	core.BrowserFirefox: {"Firefox.app"},
	core.BrowserBrave:   {"Brave Browser.app"},
	core.BrowserArc:     {"Arc.app"},
}

// browserSearchPaths are where an application can live. A user-installed copy
// under ~/Applications is as real as one in /Applications.
func browserSearchPaths() []string {
	paths := []string{"/Applications", "/System/Applications", "/System/Cryptexes/App/System/Applications"}
	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, "Applications"))
	}
	return paths
}

// browserInstalled reports whether a browser is on this Mac.
func browserInstalled(browser core.Browser) bool {
	bundles, ok := browserApps[browser]
	if !ok {
		return false
	}
	for _, dir := range browserSearchPaths() {
		for _, bundle := range bundles {
			if info, err := os.Stat(filepath.Join(dir, bundle)); err == nil && info.IsDir() {
				return true
			}
		}
	}
	return false
}
