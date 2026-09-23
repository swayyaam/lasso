package core

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// recordingRemuxer stands in for ffmpeg in the queue tests.
type recordingRemuxer struct {
	calls []string
	err   error
}

func (r *recordingRemuxer) Remux(_ context.Context, path string) (string, error) {
	r.calls = append(r.calls, path)
	if r.err != nil {
		return "", r.err
	}
	return strings.TrimSuffix(path, filepath.Ext(path)) + ".mp4", nil
}

func finishingAs(path string) *funcRunner {
	return &funcRunner{run: func(_ context.Context, args []string, stdout, _ func(string)) error {
		if isMetadataCall(args) {
			stdout(`{"id":"x","title":"Big Buck Bunny"}`)
			return nil
		}
		stdout(`{"stage":"downloading","downloaded":10,"total":10}`)
		stdout(`{"stage":"complete","path":"` + path + `"}`)
		return nil
	}}
}

func withRemuxer(r Remuxer) func(*QueueConfig) {
	return func(c *QueueConfig) { c.Remuxer = r }
}

func TestAnAVIFinishesAsAnMP4(t *testing.T) {
	remuxer := &recordingRemuxer{}
	h := newQueueHarnessWith(t, finishingAs("/m/Big Buck Bunny.avi"), 1, nil, withRemuxer(remuxer))

	item, _ := h.q.Add(Options{URL: "https://archive.org/details/BigBuckBunny_124", Pick: PickBest}, "Big Buck Bunny")
	h.waitFor(t, item.ID, StateDone)

	done, _ := h.q.Get(item.ID)
	if done.FilePath != "/m/Big Buck Bunny.mp4" {
		t.Errorf("FilePath = %q, want the MP4 — Open and Show in Finder use it", done.FilePath)
	}
	if done.Notice != "" {
		t.Errorf("Notice = %q, want none for a remux that worked", done.Notice)
	}
}

func TestARemuxThatFailsKeepsTheDownload(t *testing.T) {
	remuxer := &recordingRemuxer{err: errors.New("Could not find tag for codec msmpeg4v3")}
	h := newQueueHarnessWith(t, finishingAs("/m/Old Film.avi"), 1, nil, withRemuxer(remuxer))

	item, _ := h.q.Add(Options{URL: "https://example.com/film", Pick: PickBest}, "Old Film")
	h.waitFor(t, item.ID, StateDone)

	done, _ := h.q.Get(item.ID)
	if done.State != StateDone || done.FilePath != "/m/Old Film.avi" {
		t.Errorf("State %q FilePath %q, want a finished download of the original", done.State, done.FilePath)
	}
	if !strings.Contains(done.Notice, "Kept as AVI") || !strings.Contains(done.Detail, "msmpeg4v3") {
		t.Errorf("Notice %q Detail %q, want it said plainly with the reason kept", done.Notice, done.Detail)
	}
}

func TestFilesThatAlreadyPlayAreLeftAlone(t *testing.T) {
	remuxer := &recordingRemuxer{}
	h := newQueueHarnessWith(t, finishingAs("/m/Clip.mp4"), 1, nil, withRemuxer(remuxer))

	item, _ := h.q.Add(Options{URL: "https://example.com/clip", Pick: PickBest}, "Clip")
	h.waitFor(t, item.ID, StateDone)
	if len(remuxer.calls) != 0 {
		t.Errorf("remuxed %v; an MP4 needs nothing", remuxer.calls)
	}
}

// ---- the real ffmpeg ---------------------------------------------------

// testFFmpeg finds the bundled ffmpeg, or skips.
func testFFmpeg(t *testing.T) string {
	t.Helper()
	if path := os.Getenv("LASSO_FFMPEG"); path != "" {
		return path
	}
	_, here, _, _ := runtime.Caller(0)
	path := filepath.Join(filepath.Dir(here), "..", "..", "apps", "desktop", "build", "bin", "darwin-arm64", "ffmpeg")
	if _, err := os.Stat(path); err != nil {
		t.Skip("no fetched ffmpeg; run `make fetch-binaries` or set LASSO_FFMPEG")
	}
	return path
}

func makeAVI(t *testing.T, ffmpeg, path, vcodec, acodec string) {
	t.Helper()
	out, err := exec.Command(ffmpeg, "-v", "error", "-f", "lavfi", "-i", "testsrc=duration=1:size=320x240",
		"-f", "lavfi", "-i", "sine=duration=1", "-c:v", vcodec, "-c:a", acodec, path).CombinedOutput()
	if err != nil {
		t.Fatalf("making %s: %v\n%s", path, err, out)
	}
}

func TestFFmpegRemuxerMovesAnAVIIntoMP4(t *testing.T) {
	ffmpeg := testFFmpeg(t)
	dir := t.TempDir()
	avi := filepath.Join(dir, "Big Buck Bunny [BigBuckBunny_124].avi")
	// archive.org's shape: MPEG-4 Part 2 and AC-3.
	makeAVI(t, ffmpeg, avi, "mpeg4", "ac3")

	out, err := (&FFmpegRemuxer{Path: ffmpeg}).Remux(context.Background(), avi)
	if err != nil {
		t.Fatalf("Remux: %v", err)
	}
	if want := filepath.Join(dir, "Big Buck Bunny [BigBuckBunny_124].mp4"); out != want {
		t.Errorf("out = %q, want %q", out, want)
	}
	if _, err := os.Stat(avi); !os.IsNotExist(err) {
		t.Error("the original AVI is still there beside its MP4")
	}
	assertOnly(t, dir, "Big Buck Bunny [BigBuckBunny_124].mp4")
}

func TestFFmpegRemuxerKeepsWhatMP4CannotHold(t *testing.T) {
	// DivX 3: the case that made yt-dlp's own remux fail the whole download.
	ffmpeg := testFFmpeg(t)
	dir := t.TempDir()
	avi := filepath.Join(dir, "Old Film.avi")
	makeAVI(t, ffmpeg, avi, "msmpeg4", "mp3")

	if _, err := (&FFmpegRemuxer{Path: ffmpeg}).Remux(context.Background(), avi); err == nil {
		t.Fatal("a DivX 3 AVI was reported as remuxed")
	}
	assertOnly(t, dir, "Old Film.avi")
}

func TestFFmpegRemuxerNeverReplacesAFileThatWasThere(t *testing.T) {
	ffmpeg := testFFmpeg(t)
	dir := t.TempDir()
	avi := filepath.Join(dir, "Clip.avi")
	makeAVI(t, ffmpeg, avi, "mpeg4", "ac3")
	os.WriteFile(filepath.Join(dir, "Clip.mp4"), []byte("someone else's"), 0o644)

	if _, err := (&FFmpegRemuxer{Path: ffmpeg}).Remux(context.Background(), avi); err == nil {
		t.Fatal("remuxed over an existing MP4")
	}
	if raw, _ := os.ReadFile(filepath.Join(dir, "Clip.mp4")); string(raw) != "someone else's" {
		t.Error("the existing MP4 was changed")
	}
}

// assertOnly checks dir holds exactly name — nothing half-written left behind.
func assertOnly(t *testing.T, dir, name string) {
	t.Helper()
	entries, _ := os.ReadDir(dir)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if len(names) != 1 || names[0] != name {
		t.Errorf("folder holds %v, want only %q", names, name)
	}
}
