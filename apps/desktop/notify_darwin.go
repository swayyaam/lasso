package main

import (
	"context"
	"os/exec"
	"time"
)

// notifyScript posts a notification, taking its text from the arguments.
//
// The text is never interpolated into the script. A video title is whatever the
// site says it is, and a title containing AppleScript would otherwise be run —
// the same reason Lasso never builds a shell string for yt-dlp. Passing the
// strings as argv makes them data that AppleScript has no way to evaluate.
const notifyScript = `on run argv
display notification (item 1 of argv) with title (item 2 of argv)
end run`

// notify posts a macOS notification. A failure is not worth reporting: the
// download already succeeded or failed on its own terms, and the user may
// simply have notifications turned off.
func notify(body, title string) {
	if body == "" && title == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// "--" stops osascript reading the following strings as its own options.
	_ = exec.CommandContext(ctx, "/usr/bin/osascript", "-e", notifyScript, "--", body, title).Run()
}
