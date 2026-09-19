package core

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// TrackTags are the fields a split-out track needs that the whole recording
// cannot supply.
//
// Everything else — artist, date, cover art — is already correct on the file,
// because it was copied from the recording the track was cut out of.
type TrackTags struct {
	// Title is the chapter's own name.
	Title string
	// Track is its position, and Tracks the total, rendered as "3/12".
	Track  int
	Tracks int
	// Album is the recording the track came from.
	Album string
}

// args renders the tags as ffmpeg -metadata pairs.
//
// An empty field is omitted rather than written blank: the value already on the
// file is more likely to be right than nothing at all.
func (t TrackTags) args() []string {
	var args []string
	if t.Title != "" {
		args = append(args, "-metadata", "title="+t.Title)
	}
	if t.Album != "" {
		args = append(args, "-metadata", "album="+t.Album)
	}
	if t.Track > 0 {
		value := strconv.Itoa(t.Track)
		if t.Tracks > 0 {
			value += "/" + strconv.Itoa(t.Tracks)
		}
		args = append(args, "-metadata", "track="+value)
	}
	return args
}

// Empty reports whether there is nothing to write.
func (t TrackTags) Empty() bool { return len(t.args()) == 0 }

// Tagger writes metadata onto a file that already exists.
//
// This exists because yt-dlp cannot do it. --split-chapters cuts a recording
// into tracks with `-c copy`, which carries every tag across unchanged — so all
// twelve tracks of an album end up titled after the album. --postprocessor-args
// cannot help either: it takes a fixed string and writes it literally, so
// "%(section_title)s" lands in the file as those characters.
//
// The interface is here so the queue can be tested without ffmpeg.
type Tagger interface {
	Tag(ctx context.Context, path string, tags TrackTags) error
}

// tagTimeout bounds one retag. It is a metadata rewrite with the audio copied,
// so it is bounded by disk speed rather than length.
const tagTimeout = 2 * time.Minute

// FFmpegTagger rewrites tags with the bundled ffmpeg.
type FFmpegTagger struct {
	// Path is the ffmpeg executable.
	Path string
}

var _ Tagger = (*FFmpegTagger)(nil)

// Tag rewrites path's metadata in place, leaving the audio untouched.
//
// ffmpeg cannot write to the file it is reading, so this goes through a
// temporary file beside it and renames over the original — the same reason the
// settings and history stores do. A failure leaves the original where it was.
func (t *FFmpegTagger) Tag(ctx context.Context, path string, tags TrackTags) error {
	if t.Path == "" {
		return fmt.Errorf("no ffmpeg to tag with")
	}
	if tags.Empty() {
		return nil
	}

	// The extension has to survive: ffmpeg picks its muxer from it.
	dir, base := filepath.Dir(path), filepath.Base(path)
	tmp := filepath.Join(dir, ".lasso-tagging-"+base)

	args := []string{
		"-nostdin",
		"-v", "error",
		"-i", path,
		// Audio first, then cover art if there is any. "?" makes the second
		// optional, so the same command works on a file without one — and
		// without it, every track that has no embedded cover would fail.
		"-map", "0:a",
		"-map", "0:v?",
		// Copy, never re-encode: this is a metadata edit, and a track that
		// came back quieter for having been renamed would be a bad trade.
		"-c", "copy",
		"-disposition:v:0", "attached_pic",
	}
	args = append(args, tags.args()...)
	args = append(args, "-y", tmp)

	ctx, cancel := context.WithTimeout(ctx, tagTimeout)
	defer cancel()

	// An argument slice, never a shell string: a chapter title is whatever the
	// uploader typed.
	cmd := exec.CommandContext(ctx, t.Path, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		os.Remove(tmp)
		return fmt.Errorf("tagging %s: %w: %s", base, err, strings.TrimSpace(string(output)))
	}

	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("replacing %s: %w", base, err)
	}
	return nil
}

// ChapterFile is one track written by --split-chapters.
type ChapterFile struct {
	// Number is the chapter's position, counting from one.
	Number int
	// Path is the file on disk.
	Path string
}

// Title recovers the chapter's own name from the filename.
//
// This is only sound because Lasso chose the template that produced it:
// ChapterTemplate puts the zero-padded number, " - ", then the title. The
// prefix is rebuilt from the number and stripped exactly once, so a chapter
// genuinely called "01 - Intro" survives — the file is "03 - 01 - Intro.m4a"
// and only the leading "03 - " goes.
//
// yt-dlp states the path and the number in its output and never the title on
// its own, so there is nothing better to read.
func (c ChapterFile) Title() string {
	base := filepath.Base(c.Path)
	base = strings.TrimSuffix(base, filepath.Ext(base))

	prefix := fmt.Sprintf("%02d - ", c.Number)
	if after, ok := strings.CutPrefix(base, prefix); ok {
		return after
	}
	return base
}
