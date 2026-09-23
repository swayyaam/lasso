package core

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Remuxer moves a finished download into a container macOS opens, re-encoding
// nothing.
//
// Some sites still serve AVI and FLV, and QuickTime opens neither — yet what
// is inside them often plays perfectly once it sits in an MP4. Measured on the
// shape archive.org serves, MPEG-4 Part 2 with AC-3: unplayable as AVI, plays
// both tracks once remuxed. The copy is lossless and takes seconds.
//
// It is done here rather than with yt-dlp's --remux-video, which fails the
// whole download when the streams do not fit MP4 — DivX 3 does not, measured —
// turning a file that downloaded fine into an error. Here a failure keeps the
// original and says why.
type Remuxer interface {
	// Remux returns the path of the new file.
	Remux(ctx context.Context, path string) (string, error)
}

// remuxable are containers QuickTime cannot open whose streams usually fit in
// an MP4.
var remuxable = map[string]bool{".avi": true, ".flv": true}

// NeedsRemux reports whether a finished file is in a container worth moving.
func NeedsRemux(path string) bool {
	return remuxable[strings.ToLower(filepath.Ext(path))]
}

// remuxTimeout bounds one remux. Streams are copied, so it is bounded by disk
// speed rather than length — but a long recording is still gigabytes.
const remuxTimeout = 10 * time.Minute

// FFmpegRemuxer remuxes with the bundled ffmpeg.
type FFmpegRemuxer struct {
	// Path is the ffmpeg executable.
	Path string
}

var _ Remuxer = (*FFmpegRemuxer)(nil)

// Remux copies path's streams into an MP4 beside it and removes the original.
//
// It writes to a temporary file and renames, like every other rewrite Lasso
// does, so a failure halfway leaves the original untouched and nothing
// half-written under the final name.
func (r *FFmpegRemuxer) Remux(ctx context.Context, path string) (string, error) {
	if r.Path == "" {
		return "", fmt.Errorf("no ffmpeg to remux with")
	}
	stem := strings.TrimSuffix(path, filepath.Ext(path))
	out := stem + ".mp4"
	if _, err := os.Stat(out); err == nil {
		// Never replace a file that was there first.
		return "", fmt.Errorf("%s is already there", filepath.Base(out))
	}
	tmp := filepath.Join(filepath.Dir(path), ".lasso-remux-"+filepath.Base(stem)+".mp4")

	args := []string{
		"-nostdin",
		"-v", "error",
		"-i", path,
		// Video, and audio if there is any; nothing else. An AVI's odd extra
		// streams are what would otherwise make an MP4 refuse the copy.
		"-map", "0:v",
		"-map", "0:a?",
		"-c", "copy",
		// Index at the front, so the file opens — and previews in Quick Look —
		// without reading all of it first.
		"-movflags", "+faststart",
		"-y", tmp,
	}

	ctx, cancel := context.WithTimeout(ctx, remuxTimeout)
	defer cancel()

	// An argument slice, never a shell string: the filename is the site's title.
	output, err := exec.CommandContext(ctx, r.Path, args...).CombinedOutput()
	if err != nil {
		os.Remove(tmp)
		return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output)))
	}
	if err := os.Rename(tmp, out); err != nil {
		os.Remove(tmp)
		return "", err
	}
	// The MP4 holds the same streams, so the original is only in the way. If
	// it cannot be removed, both exist and the MP4 is the one Lasso points at.
	_ = os.Remove(path)
	return out, nil
}
