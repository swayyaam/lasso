package main

/*
#cgo CFLAGS: -x objective-c -fmodules -fobjc-arc
#cgo LDFLAGS: -framework AppKit

#import <AppKit/AppKit.h>

// AppKit is main-thread only, and a menu callback arrives on a goroutine.

static void lassoAbout(void) {
	dispatch_async(dispatch_get_main_queue(), ^{ [NSApp orderFrontStandardAboutPanel:nil]; });
}

static void lassoHide(void) {
	dispatch_async(dispatch_get_main_queue(), ^{ [NSApp hide:nil]; });
}

static void lassoHideOthers(void) {
	dispatch_async(dispatch_get_main_queue(), ^{ [NSApp hideOtherApplications:nil]; });
}

static void lassoShowAll(void) {
	dispatch_async(dispatch_get_main_queue(), ^{ [NSApp unhideAllApplications:nil]; });
}

// The Dock badge: the number of downloads not yet finished, or nothing.
static void lassoDockBadge(int count) {
	dispatch_async(dispatch_get_main_queue(), ^{
		NSString *label = count > 0 ? [NSString stringWithFormat:@"%d", count] : nil;
		[[NSApp dockTile] setBadgeLabel:label];
	});
}
*/
import "C"

import (
	"github.com/wailsapp/wails/v2/pkg/menu"
	"github.com/wailsapp/wails/v2/pkg/menu/keys"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// Menu commands the interface carries out. They travel as EventMenu's payload.
const (
	MenuSettings  = "settings"
	MenuDownload  = "download"
	MenuDownloads = "downloads"
	MenuHistory   = "history"
)

// appMenu is the menu bar.
//
// The Lasso menu is built by hand rather than taken from Wails' AppMenu role,
// which is a fixed set with nowhere to put Settings — and Settings belongs
// there, under ⌘,, where every Mac app keeps it. Edit and Window stay as
// roles: Edit is what makes ⌘C and ⌘V work in a text field at all.
func (a *App) appMenu() *menu.Menu {
	m := menu.NewMenu()

	lasso := m.AddSubmenu("Lasso")
	lasso.AddText("About Lasso", nil, func(*menu.CallbackData) { C.lassoAbout() })
	lasso.AddSeparator()
	lasso.AddText("Settings…", keys.CmdOrCtrl(","), a.command(MenuSettings))
	lasso.AddSeparator()
	lasso.AddText("Hide Lasso", keys.CmdOrCtrl("h"), func(*menu.CallbackData) { C.lassoHide() })
	lasso.AddText("Hide Others", keys.Combo("h", keys.CmdOrCtrlKey, keys.OptionOrAltKey), func(*menu.CallbackData) { C.lassoHideOthers() })
	lasso.AddText("Show All", nil, func(*menu.CallbackData) { C.lassoShowAll() })
	lasso.AddSeparator()
	lasso.AddText("Quit Lasso", keys.CmdOrCtrl("q"), func(*menu.CallbackData) {
		if a.ctx != nil {
			runtime.Quit(a.ctx)
		}
	})

	file := m.AddSubmenu("File")
	// ⌘V already opens a link pasted anywhere in the window; this is the same
	// from the menu bar, and works while a text field has focus.
	file.AddText("Paste Link", keys.Combo("v", keys.CmdOrCtrlKey, keys.ShiftKey), func(*menu.CallbackData) { a.pasteLink() })
	file.AddText("Download", keys.CmdOrCtrl("return"), a.command(MenuDownload))

	m.Append(menu.EditMenu())

	view := m.AddSubmenu("View")
	view.AddText("Downloads", keys.CmdOrCtrl("1"), a.command(MenuDownloads))
	view.AddText("History", keys.CmdOrCtrl("y"), a.command(MenuHistory))

	m.Append(menu.WindowMenu())
	return m
}

// command is a menu item the interface carries out.
func (a *App) command(name string) menu.Callback {
	return func(*menu.CallbackData) {
		if a.ctx != nil {
			runtime.EventsEmit(a.ctx, EventMenu, name)
		}
	}
}

// setDockBadge shows how many downloads are unfinished on the Dock icon.
func setDockBadge(count int) {
	C.lassoDockBadge(C.int(count))
}
