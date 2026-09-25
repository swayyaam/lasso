package main

/*
#cgo LDFLAGS: -framework IOKit -framework CoreFoundation

#include <IOKit/pwr_mgt/IOPMLib.h>

// The name shows in `pmset -g assertions` and Activity Monitor's Energy tab,
// which is where someone goes to ask why their Mac did not sleep.
static IOReturn lassoHoldAwake(IOPMAssertionID *id) {
	return IOPMAssertionCreateWithName(
		kIOPMAssertionTypePreventUserIdleSystemSleep,
		kIOPMAssertionLevelOn,
		CFSTR("Lasso is downloading"),
		id);
}
*/
import "C"

import (
	"log"
	"sync"
)

// awake is the one power assertion Lasso holds, or none.
var awake struct {
	mu   sync.Mutex
	id   C.IOPMAssertionID
	held bool
}

// keepAwake holds the Mac out of idle sleep while downloads are working, and
// lets it go when they stop.
//
// Idle sleep only: the display still sleeps on its own schedule, and closing
// the lid on battery still sleeps the Mac — a download is not a reason to
// override something the person did on purpose.
func keepAwake(hold bool) {
	awake.mu.Lock()
	defer awake.mu.Unlock()

	switch {
	case hold && !awake.held:
		if ret := C.lassoHoldAwake(&awake.id); ret != C.kIOReturnSuccess {
			// Not worth failing anything over: the download carries on, and
			// only risks pausing if the Mac sleeps.
			log.Printf("lasso: could not keep the Mac awake: IOReturn %#x", uint32(ret))
			return
		}
		awake.held = true
	case !hold && awake.held:
		C.IOPMAssertionRelease(awake.id)
		awake.held = false
	}
}
