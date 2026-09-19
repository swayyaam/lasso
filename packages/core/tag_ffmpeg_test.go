package core

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// findFFmpeg locates a real ffmpeg, or skips.
//
// Lasso ships its own and never uses the system one, so this is not how the app
// finds it — but a test that exercises the real command needs a real binary,
// and skipping where there is none beats asserting against a fake.
func findFFmpeg(t *testing.T) string {
	t.Helper()
	path, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("no ffmpeg on PATH; run with the bundled bin directory on PATH to exercise this")
	}
	return path
}

// makeTrack writes a short m4a with tags already on it, standing in for a file
// yt-dlp cut out of a longer recording.
func makeTrack(t *testing.T, ffmpeg, path string) {
	t.Helper()
	cmd := exec.Command(ffmpeg,
		"-v", "error",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=1",
		"-c:a", "aac",
		"-metadata", "artist=Various Artists",
		"-metadata", "title=Greatest Hits",
		"-y", path,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("building the fixture: %v: %s", err, out)
	}
}

// readTags reads a file's format tags back.
func readTags(t *testing.T, ffmpeg, path string) map[string]string {
	t.Helper()
	probe := filepath.Join(filepath.Dir(ffmpeg), "ffprobe")
	if _, err := os.Stat(probe); err != nil {
		t.Skip("no ffprobe beside ffmpeg")
	}

	out, err := exec.Command(probe,
		"-v", "error",
		"-show_entries", "format_tags",
		"-of", "default=noprint_wrappers=1",
		path,
	).Output()
	if err != nil {
		t.Fatalf("probing: %v", err)
	}

	tags := map[string]string{}
	for _, line := range strings.Split(string(out), "\n") {
		key, value, ok := strings.Cut(strings.TrimPrefix(strings.TrimSpace(line), "TAG:"), "=")
		if ok {
			tags[strings.ToLower(key)] = value
		}
	}
	return tags
}

func TestFFmpegTaggerRewritesOnlyWhatItWasGiven(t *testing.T) {
	ffmpeg := findFFmpeg(t)
	path := filepath.Join(t.TempDir(), "01 - Opening Theme.m4a")
	makeTrack(t, ffmpeg, path)

	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	tagger := &FFmpegTagger{Path: ffmpeg}
	err = tagger.Tag(context.Background(), path, TrackTags{
		Title:  "Opening Theme",
		Track:  1,
		Tracks: 3,
		Album:  "Greatest Hits",
	})
	if err != nil {
		t.Fatalf("Tag: %v", err)
	}

	tags := readTags(t, ffmpeg, path)
	if tags["title"] != "Opening Theme" {
		t.Errorf("title = %q, want the chapter's own name", tags["title"])
	}
	if tags["album"] != "Greatest Hits" {
		t.Errorf("album = %q", tags["album"])
	}
	if tags["track"] != "1/3" {
		t.Errorf("track = %q, want 1/3", tags["track"])
	}
	// The artist came from the recording and was not passed in, so it has to
	// survive: the tags already on the file are more likely right than blank.
	if tags["artist"] != "Various Artists" {
		t.Errorf("artist = %q, want the existing tag kept", tags["artist"])
	}

	// The audio is copied, not re-encoded. Sizes differ only by the tag delta.
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if diff := after.Size() - before.Size(); diff > 4096 || diff < -4096 {
		t.Errorf("size changed by %d bytes, which is more than a tag rewrite", diff)
	}
}

func TestFFmpegTaggerLeavesTheFileAloneOnFailure(t *testing.T) {
	ffmpeg := findFFmpeg(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "not-audio.m4a")
	if err := os.WriteFile(path, []byte("this is not a media file"), 0o644); err != nil {
		t.Fatal(err)
	}

	tagger := &FFmpegTagger{Path: ffmpeg}
	if err := tagger.Tag(context.Background(), path, TrackTags{Title: "Nope"}); err == nil {
		t.Fatal("tagging a non-media file succeeded")
	}

	// The original must still be there, and the temporary file must not.
	if content, err := os.ReadFile(path); err != nil || string(content) != "this is not a media file" {
		t.Error("the original file was damaged by a failed tagging")
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".lasso-tagging-") {
			t.Errorf("left a temporary file behind: %s", e.Name())
		}
	}
}

func TestFFmpegTaggerNeedsSomethingToDo(t *testing.T) {
	// No ffmpeg run at all when there are no tags, so this passes without one.
	tagger := &FFmpegTagger{Path: "/nonexistent/ffmpeg"}
	if err := tagger.Tag(context.Background(), "/nonexistent/file.m4a", TrackTags{}); err != nil {
		t.Errorf("Tag with nothing to write = %v, want it to do nothing", err)
	}
}
