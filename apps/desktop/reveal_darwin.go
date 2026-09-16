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
