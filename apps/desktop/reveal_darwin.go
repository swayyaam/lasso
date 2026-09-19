package main

import (
	"context"
	"os/exec"
	"time"
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
