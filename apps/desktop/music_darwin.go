package main

import (
	"os"

	"github.com/swayyaam/lasso/packages/core"
)

// musicLibrary is the Music app's "Automatically Add to Music" folder, which
// imports whatever lands in it. It is looked for on each add, so a Music
// opened for the first time after Lasso still works.
func musicLibrary() core.MusicFolder {
	home, _ := os.UserHomeDir()
	return core.MusicFolder{Home: home}
}

// MusicReady reports whether Music has its "Automatically Add" folder yet, so
// Settings can say to open Music once rather than offer a switch that fails.
func (a *App) MusicReady() bool {
	_, err := musicLibrary().Find()
	return err == nil
}
