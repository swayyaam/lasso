package main

/*
#cgo CFLAGS: -x objective-c -fmodules -fobjc-arc
#cgo LDFLAGS: -framework AppKit

#import <AppKit/AppKit.h>

// Bring the copy of Lasso that is already running to the front.
static void lassoActivate(int pid) {
	NSRunningApplication *app = [NSRunningApplication runningApplicationWithProcessIdentifier:pid];
	if (app == nil) {
		return;
	}
	if (@available(macOS 14.0, *)) {
		[app activateWithOptions:NSApplicationActivateAllWindows];
	} else {
		// Before macOS 14 an app could only come forward over the current
		// one by asking to ignore it; the option is deprecated from 14 on,
		// which is the branch above.
#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Wdeprecated-declarations"
		[app activateWithOptions:NSApplicationActivateAllWindows | NSApplicationActivateIgnoringOtherApps];
#pragma clang diagnostic pop
	}
}
*/
import "C"

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// errInstanceHeld means another copy of Lasso holds the support folder.
var errInstanceHeld = errors.New("another copy of Lasso is running")

// instanceLock keeps Lasso to one copy per support folder.
//
// Two copies share one settings file, one history and — once it is saved — one
// queue, and each overwrites what the other wrote. That is not hypothetical:
// running the build output beside the installed app is how a settings change
// went missing earlier. So the lock is on the support folder itself, not on the
// window: two copies pointed at different folders (LASSO_SUPPORT_DIR) do not
// share anything and may both run.
//
// Wails has a single-instance option, but its lock is held until the process
// exits, and the updater restarts by launching the new version a moment before
// the old one quits — so the new version would find the old one, defer to it,
// and exit, leaving nothing running at all. This lock can be released first.
type instanceLock struct {
	file *os.File
}

// instanceLockFile is the lock's name inside the support folder.
const instanceLockFile = "lasso.lock"

// acquireInstance takes the lock, or reports which process holds it.
//
// flock is released by the kernel when its holder exits, however it exits, so
// a crash never leaves Lasso locked out of its own folder.
func acquireInstance(support string) (*instanceLock, int, error) {
	if err := os.MkdirAll(support, 0o755); err != nil {
		return nil, 0, err
	}
	f, err := os.OpenFile(filepath.Join(support, instanceLockFile), os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, 0, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		raw, _ := io.ReadAll(f)
		f.Close()
		holder, _ := strconv.Atoi(strings.TrimSpace(string(raw)))
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, holder, errInstanceHeld
		}
		return nil, holder, fmt.Errorf("locking the support folder: %w", err)
	}
	// Record who holds it, so a second copy knows which window to bring up.
	if err := f.Truncate(0); err == nil {
		_, _ = f.WriteAt([]byte(strconv.Itoa(os.Getpid())), 0)
	}
	return &instanceLock{file: f}, 0, nil
}

// Release gives the lock up while this process is still running — which is
// exactly what the updater's restart needs. Safe to call more than once.
func (l *instanceLock) Release() {
	if l == nil || l.file == nil {
		return
	}
	_ = syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN)
	_ = l.file.Close()
	l.file = nil
}

// activateInstance brings the copy holding the lock to the front.
func activateInstance(pid int) {
	if pid > 0 {
		C.lassoActivate(C.int(pid))
	}
}
