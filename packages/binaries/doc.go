// Package binaries locates, installs, verifies, and updates the sidecar
// binaries Lasso depends on (yt-dlp, ffmpeg, ffprobe, deno).
//
// Binaries ship inside the .app bundle but are always executed from
// ~/Library/Application Support/Lasso/bin, never from inside the bundle.
package binaries
