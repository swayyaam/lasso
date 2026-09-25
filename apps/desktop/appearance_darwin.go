package main

/*
#cgo CFLAGS: -x objective-c -fmodules -fobjc-arc
#cgo LDFLAGS: -framework AppKit

#import <AppKit/AppKit.h>

// 0 follows the system, 1 is light, 2 is dark.
static NSAppearance *lassoAppearance(int choice) {
	switch (choice) {
	case 1: return [NSAppearance appearanceNamed:NSAppearanceNameAqua];
	case 2: return [NSAppearance appearanceNamed:NSAppearanceNameDarkAqua];
	default: return nil;
	}
}

// Wails names the window's appearance when it creates it, and a window's own
// appearance outranks the application's, so both are set: the application for
// menus and panels, the window for its traffic lights and the webview, whose
// prefers-color-scheme follows it.
static void lassoSetAppearance(int choice) {
	dispatch_async(dispatch_get_main_queue(), ^{
		NSAppearance *appearance = lassoAppearance(choice);
		NSApp.appearance = appearance;
		for (NSWindow *window in NSApp.windows) {
			window.appearance = appearance;
		}
	});
}

// Readable before NSApp exists, which is when the window's first colour is
// chosen. Set to "Dark" whenever the Mac is dark, including under Auto.
static int lassoSystemIsDark(void) {
	NSString *style = [[NSUserDefaults standardUserDefaults] stringForKey:@"AppleInterfaceStyle"];
	return [style isEqualToString:@"Dark"] ? 1 : 0;
}
*/
import "C"

import (
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
)

// Canvas colours, matching --color-canvas in each appearance. The window
// shows its own colour until the page paints and wherever a resize outruns
// it, so a mismatch shows as a flash.
var (
	lightCanvas = options.RGBA{R: 255, G: 255, B: 255, A: 255}
	darkCanvas  = options.RGBA{R: 8, G: 8, B: 8, A: 255}
)

// windowAppearance is what the window is created with: nothing for Auto, so
// it follows the Mac, and the named appearance otherwise.
func windowAppearance(choice Appearance) mac.AppearanceType {
	switch choice {
	case AppearanceLight:
		return mac.NSAppearanceNameAqua
	case AppearanceDark:
		return mac.NSAppearanceNameDarkAqua
	}
	return ""
}

// windowCanvas is the colour the window opens with, before the page decides.
func windowCanvas(choice Appearance) *options.RGBA {
	dark := choice == AppearanceDark || (choice == AppearanceAuto && C.lassoSystemIsDark() == 1)
	if dark {
		return &darkCanvas
	}
	return &lightCanvas
}

// setAppearance changes the running app's appearance. The page follows by
// itself: its colours come from the setting and prefers-color-scheme.
func setAppearance(choice Appearance) {
	var n C.int
	switch choice {
	case AppearanceLight:
		n = 1
	case AppearanceDark:
		n = 2
	}
	C.lassoSetAppearance(n)
}
