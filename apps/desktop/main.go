// Command lasso is the desktop application: a minimal macOS front end for
// yt-dlp.
//
// The application logic lives in packages/core, packages/binaries and
// packages/presets, none of which import Wails. This package is the shell that
// binds them to a window.
package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app := NewApp()

	err := wails.Run(&options.App{
		Title:  "Lasso",
		Width:  1080,
		Height: 740,
		// Below this the two panes stop making sense and the layout stacks.
		MinWidth:  880,
		MinHeight: 600,
		AssetServer: &assetserver.Options{
			Assets: assets,
			// Anything the embedded frontend does not answer falls through to
			// here, which is how cached thumbnails reach the webview.
			Handler: app.assetHandler(),
		},
		// The design system is a dark canvas; anything lighter shows as a
		// flash while the window paints.
		BackgroundColour: &options.RGBA{R: 1, G: 1, B: 2, A: 1},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		Bind: []any{
			app,
		},
		Mac: &mac.Options{
			// Traffic lights sit inside the app's own header bar: a stock
			// title bar would show as a grey strip against the dark canvas.
			TitleBar:             mac.TitleBarHiddenInset(),
			Appearance:           mac.NSAppearanceNameDarkAqua,
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
			About: &mac.AboutInfo{
				Title:   "Lasso",
				Message: "A minimal front end for yt-dlp.",
			},
		},
	})
	if err != nil {
		log.Fatalf("lasso: %v", err)
	}
}
