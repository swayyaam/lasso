// Package binaries locates, installs, verifies, and updates the sidecar
// binaries Lasso depends on: yt-dlp, ffmpeg, ffprobe and deno.
//
// The binaries ship inside the .app bundle but are always executed from
// ~/Library/Application Support/Lasso/bin. The bundle is read-only, so yt-dlp
// could not update itself from there; copying out also lets Lasso repair
// permissions, quarantine flags and signatures, none of which can be changed
// in place inside a bundle.
//
// The usual entry point is Startup, which resolves the directories, installs
// anything missing or outdated, and confirms every binary actually runs:
//
//	mgr, status, err := binaries.Startup(ctx)
//
// Versions come from the manifest.json that scripts/fetch-binaries.sh writes
// next to the binaries, which keeps binaries.lock.json the single source of
// truth for what is pinned.
//
// This package must not import Wails: Status and Problem are plain types that
// apps/desktop passes straight to the frontend.
package binaries
