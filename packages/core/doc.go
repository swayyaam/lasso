// Package core implements Lasso's yt-dlp integration: metadata fetching, the
// argument builder, progress parsing, error classification, and the download
// queue.
//
// This package must stay free of Wails imports so it remains testable on its
// own. Anything that needs to reach the UI does so through the types declared
// here, which apps/desktop adapts into Wails events.
package core
