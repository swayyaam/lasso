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
			// Middleware, not Handler: it runs ahead of asset serving, so
			// cached thumbnails are served the same way in development, where
			// the Vite dev server would otherwise answer every unknown path.
			Middleware: app.assetMiddleware,
		},
		// Matches --color-canvas. Anything else shows as a flash while the
		// window paints.
		BackgroundColour: &options.RGBA{R: 255, G: 255, B: 255, A: 1},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		Bind: []any{
			app,
		},
		Mac: &mac.Options{
			// Traffic lights sit inside the app's own header bar: a stock
			// title bar would show as a grey strip against the canvas.
			TitleBar: mac.TitleBarHiddenInset(),
			// Aqua, so the traffic lights and any native menu or dialog match
			// the white canvas rather than sitting dark on it.
			Appearance:           mac.NSAppearanceNameAqua,
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
