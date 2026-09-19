package main

/*
#cgo CFLAGS: -x objective-c -fmodules -fobjc-arc
#cgo LDFLAGS: -framework Foundation -framework UserNotifications

#include <stdlib.h>
#include "notify_darwin.h"
*/
import "C"

import (
	"context"
	"os/exec"
	"time"
	"unsafe"

	"github.com/swayyaam/lasso/packages/doctor"
)

// notify posts a macOS notification for a finished download.
//
// It goes through UNUserNotificationCenter so the notification is Lasso's: the
// framework reads the bundle identifier of whoever asked, which means Lasso's
// name and icon on the banner and an entry under Lasso in Notification
// Settings. The previous implementation shelled out to osascript, and every
// notification arrived from Script Editor — that being the process AppleScript
// actually runs in.
//
// This blocks for as long as the permission prompt is unanswered, so callers
// run it off whatever thread they care about.
func notify(body, title string) {
	if body == "" && title == "" {
		return
	}
	if notifyNative(body, title) {
		return
	}
	// The native path is refused when the process is not a bundle — `go test`
	// — and can be refused for a build that is only ad-hoc signed. A
	// notification under the wrong name beats none at all.
	notifyViaAppleScript(body, title)
}

// notifyNative posts through the framework, reporting whether it was accepted.
func notifyNative(body, title string) bool {
	if !bool(C.lassoCanNotify()) {
		return false
	}

	cTitle := C.CString(title)
	defer C.free(unsafe.Pointer(cTitle))
	cBody := C.CString(body)
	defer C.free(unsafe.Pointer(cBody))

	return bool(C.lassoNotify(cTitle, cBody))
}

// notifyScript posts a notification, taking its text from the arguments.
//
// The text is never interpolated into the script. A video title is whatever the
// site says it is, and a title containing AppleScript would otherwise be run —
// the same reason Lasso never builds a shell string for yt-dlp. Passing the
// strings as argv makes them data that AppleScript has no way to evaluate.
const notifyScript = `on run argv
display notification (item 1 of argv) with title (item 2 of argv)
end run`

// notifyViaAppleScript is the fallback. It works anywhere, at the cost of the
// notification being attributed to Script Editor rather than to Lasso.
func notifyViaAppleScript(body, title string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// "--" stops osascript reading the following strings as its own options.
	_ = exec.CommandContext(ctx, "/usr/bin/osascript", "-e", notifyScript, "--", body, title).Run()
}

// NotificationStatus is whether macOS will let Lasso post under its own name.
type NotificationStatus int

const (
	// NotificationsUnavailable means the framework refused this build outright.
	// The usual cause is that it is only ad-hoc signed.
	NotificationsUnavailable NotificationStatus = -1
	// NotificationsNotAsked means the permission prompt has not been shown.
	NotificationsNotAsked NotificationStatus = 0
	// NotificationsRefused means the user said no.
	NotificationsRefused NotificationStatus = 1
	// NotificationsAllowed means they arrive as Lasso.
	NotificationsAllowed NotificationStatus = 2
)

// notificationStatus reports the current authorization without asking for it.
func notificationStatus() NotificationStatus {
	return NotificationStatus(int(C.lassoNotificationStatus()))
}

// notificationState translates the platform status into the doctor's terms.
func notificationState() doctor.NotificationState {
	switch notificationStatus() {
	case NotificationsAllowed:
		return doctor.NotificationsAllowed
	case NotificationsNotAsked:
		return doctor.NotificationsNotAsked
	case NotificationsRefused:
		return doctor.NotificationsRefused
	default:
		return doctor.NotificationsUnavailable
	}
}
