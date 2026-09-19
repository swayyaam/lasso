package main

import (
	"fmt"
	"syscall"
)

// minimumFreeBytes is the floor below which Lasso will not start a download.
//
// It is not an estimate of what the download needs — that is not known until
// yt-dlp has picked a format, and for a playlist not even then. It is the point
// at which starting anything is a bad idea: a merge writes a third file the
// size of the other two combined, so finishing needs room well past the
// download itself.
const minimumFreeBytes = 500 << 20 // 500 MB

// freeBytes reports the space available to this user on the volume holding dir.
//
// Bavail rather than Bfree: some of the free space on a volume is reserved for
// root, and a download cannot use it.
func freeBytes(dir string) (int64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(dir, &stat); err != nil {
		return 0, fmt.Errorf("checking free space on %s: %w", dir, err)
	}
	return int64(stat.Bavail) * int64(stat.Bsize), nil
}

// checkDiskSpace refuses a download when the destination is essentially full.
//
// A failure to measure is not a failure to download: an unreadable volume is
// the download's problem to report, with a better error than this could give.
func checkDiskSpace(dir string) error {
	if dir == "" {
		return nil
	}
	free, err := freeBytes(dir)
	if err != nil {
		return nil
	}
	if free >= minimumFreeBytes {
		return nil
	}
	return fmt.Errorf("only %s free in %s — not enough room to download safely", formatBytes(free), dir)
}

// formatBytes renders a size the way the interface does, so the two never
// disagree about what a gigabyte is.
func formatBytes(bytes int64) string {
	const unit = 1000
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	value, exp := float64(bytes), 0
	for value >= unit && exp < 4 {
		value /= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", value, "KMGT"[exp-1])
}
