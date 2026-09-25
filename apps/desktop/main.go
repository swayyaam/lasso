// Command lasso is the desktop application: a minimal macOS front end for
// yt-dlp.
//
// The application logic lives in packages/core, packages/binaries and
// packages/presets, none of which import Wails. This package is the shell that
// binds them to a window.
package main

import (
	"embed"
	"errors"
	"log"

	"github.com/swayyaam/lasso/packages/binaries"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app := NewApp()

	// One copy per support folder; see instanceLock. A second copy brings the
	// first to the front and leaves — the same thing launching a running app
	// from the Dock does.
	//
	// Not while generating bindings, which runs this same main to read the
	// bound methods; see bindings_on.go.
	appearance := AppearanceAuto
	if support, err := binaries.SupportDir(); err == nil {
		appearance = savedAppearance(support)
		if !generatingBindings {
			lock, holder, err := acquireInstance(support)
			if errors.Is(err, errInstanceHeld) {
				activateInstance(holder)
				return
			}
			if err != nil {
				// Not being able to lock is no reason not to run; it only
				// loses the protection.
				log.Printf("lasso: running without the single-instance lock: %v", err)
			}
			app.instance = lock
		}
	}

	err := wails.Run(&options.App{
		Title:  "Lasso",
		Width:  1080,
		Height: 740,
		// Narrow enough for a half-screen window on a 13-inch display; below
		// it a thumbnail, a title and two labelled buttons stop fitting a row.
		MinWidth:  880,
		MinHeight: 600,
		AssetServer: &assetserver.Options{
			Assets: assets,
			// Middleware, not Handler: it runs ahead of asset serving, so
			// cached thumbnails are served the same way in development, where
			// the Vite dev server would otherwise answer every unknown path.
			Middleware: app.assetMiddleware,
		},
		// Matches --color-canvas in the appearance the window opens in.
		// Anything else shows as a flash while the page paints.
		BackgroundColour: windowCanvas(appearance),
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		Menu:             app.appMenu(),
		Bind: []any{
			app,
		},
		Mac: &mac.Options{
			// Traffic lights sit inside the app's own header bar: a stock
			// title bar would show as a grey strip against the canvas.
			TitleBar: mac.TitleBarHiddenInset(),
			// The saved choice, so the traffic lights and any native menu or
			// dialog match the canvas from the first frame; none for Auto.
			Appearance:           windowAppearance(appearance),
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
			About: &mac.AboutInfo{
				Title:   "Lasso",
				Message: "A minimal front end for yt-dlp.",
			},
			// A lasso:// link, or a web link dropped on the Dock icon.
			OnUrlOpen: app.onURLOpen,
			// A .webloc or .url opened with Lasso, or dropped on its icon.
			OnFileOpen: app.onFileOpen,
		},
	})
	if err != nil {
		log.Fatalf("lasso: %v", err)
	}
}
