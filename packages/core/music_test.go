package core

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// musicHome makes a home folder whose Music library lives at rel, the way
// Music (or iTunes before it) lays one out, and returns the home and the
// "Automatically Add" folder inside it.
func musicHome(t *testing.T, rel string) (string, string) {
	t.Helper()
	home := t.TempDir()
	dir := filepath.Join(home, rel)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return home, dir
}

func TestMusicFolderIsFoundWhereMusicKeepsIt(t *testing.T) {
	for _, rel := range []string{
		"Music/Music/Media.localized/Automatically Add to Music.localized",
		"Music/Music/Media/Automatically Add to Music",
		// A library carried over from iTunes keeps its media folder.
		"Music/iTunes/iTunes Media/Automatically Add to Music.localized",
	} {
		home, want := musicHome(t, rel)
		got, err := MusicFolder{Home: home}.Find()
		if err != nil || got != want {
			t.Errorf("%s: Find = %q, %v; want %q", rel, got, err, want)
		}
	}
}

func TestNoMusicFolderSaysToOpenMusic(t *testing.T) {
	// Music makes the folder the first time it opens.
	if _, err := (MusicFolder{Home: t.TempDir()}).Find(); !errors.Is(err, ErrNoMusicFolder) {
		t.Errorf("Find = %v, want ErrNoMusicFolder", err)
	}
	if err := (MusicFolder{Home: t.TempDir()}).Add("/nowhere.m4a"); !errors.Is(err, ErrNoMusicFolder) {
		t.Errorf("Add = %v, want ErrNoMusicFolder", err)
	}
}

func TestAddingToMusicCopiesAndKeepsTheDownload(t *testing.T) {
	home, dir := musicHome(t, "Music/Music/Media.localized/Automatically Add to Music.localized")
	downloads := t.TempDir()
	song := filepath.Join(downloads, "Song [x].m4a")
	if err := os.WriteFile(song, []byte("audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	music := MusicFolder{Home: home}

	if err := music.Add(song); err != nil {
		t.Fatalf("Add: %v", err)
	}
	// Music has not taken the first one yet: the second must not replace it.
	if err := music.Add(song); err != nil {
		t.Fatalf("second Add: %v", err)
	}

	for _, name := range []string{"Song [x].m4a", "Song [x] 2.m4a"} {
		if got, err := os.ReadFile(filepath.Join(dir, name)); err != nil || string(got) != "audio" {
			t.Errorf("%s: %q, %v; want a full copy", name, got, err)
		}
	}
	if _, err := os.Stat(song); err != nil {
		t.Errorf("the download itself is gone: %v", err)
	}
	// The half-written copy is made beside the folder, never inside it, and
	// nothing of it is left behind.
	leftovers, _ := filepath.Glob(filepath.Join(filepath.Dir(dir), ".lasso-add-*"))
	if len(leftovers) != 0 {
		t.Errorf("temporary copies left behind: %v", leftovers)
	}
}

func TestMusicOpensOnlyWhatItImports(t *testing.T) {
	for path, want := range map[string]bool{
		"a.mp3": true, "a.M4A": true, "a.aac": true, "a.aiff": true, "a.wav": true,
		"a.opus": false, "a.flac": false, "a.ogg": false, "a.webm": false, "a.mp4": false, "a": false,
	} {
		if got := MusicOpens(path); got != want {
			t.Errorf("MusicOpens(%q) = %v, want %v", path, got, want)
		}
	}
}

// recordingMusic is a MusicLibrary that remembers what it was handed.
type recordingMusic struct {
	mu    sync.Mutex
	added []string
	err   error
}

func (m *recordingMusic) Add(path string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return m.err
	}
	m.added = append(m.added, path)
	return nil
}

// fileRunner scripts a download that finishes as path.
func fileRunner(path string) *funcRunner {
	return &funcRunner{run: func(_ context.Context, args []string, stdout, _ func(string)) error {
		if isMetadataCall(args) {
			stdout(`{"id":"x","title":"Song"}`)
			return nil
		}
		stdout(`{"stage":"downloading","downloaded":10,"total":10}`)
		stdout(`{"stage":"complete","path":"` + path + `"}`)
		return nil
	}}
}

func runWithMusic(t *testing.T, runner Runner, music MusicLibrary, o Options) Item {
	t.Helper()
	h := newQueueHarnessWith(t, runner, 1, nil, func(cfg *QueueConfig) { cfg.Music = music })
	item, err := h.q.Add(o, Source{Title: "Song"})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	h.waitFor(t, item.ID, StateDone)
	done, _ := h.q.Get(item.ID)
	return done
}

func TestFinishedAudioGoesToMusicWhenAsked(t *testing.T) {
	music := &recordingMusic{}
	o := Options{URL: "https://example.com/v", Pick: PickAudioM4A, Music: Music{AddToMusic: true}}
	done := runWithMusic(t, fileRunner("/tmp/Song.m4a"), music, o)

	if len(music.added) != 1 || music.added[0] != "/tmp/Song.m4a" {
		t.Errorf("added %v, want the finished file", music.added)
	}
	if done.Notice != "" {
		t.Errorf("Notice = %q, want none for a file that went", done.Notice)
	}
}

func TestMusicIsLeftAloneUnlessAskedAndForAudio(t *testing.T) {
	cases := map[string]Options{
		"not asked": {URL: "https://example.com/v", Pick: PickAudioM4A},
		// A preset could carry the switch onto a video pick.
		"a video": {URL: "https://example.com/v", Pick: Pick1080p, Music: Music{AddToMusic: true}},
	}
	for name, o := range cases {
		music := &recordingMusic{}
		runWithMusic(t, fileRunner("/tmp/Song.m4a"), music, o)
		if len(music.added) != 0 {
			t.Errorf("%s: added %v, want nothing", name, music.added)
		}
	}
}

func TestMusicIsNotHandedWhatItCannotOpen(t *testing.T) {
	// Music would set an Opus file aside in "Not Added" without a word, so it
	// is never handed one, and the item says why.
	music := &recordingMusic{}
	o := Options{URL: "https://example.com/v", Pick: PickAudioOpus, Music: Music{AddToMusic: true}}
	done := runWithMusic(t, fileRunner("/tmp/Song.opus"), music, o)

	if len(music.added) != 0 {
		t.Errorf("added %v, want nothing", music.added)
	}
	if done.State != StateDone || !strings.Contains(done.Notice, "cannot open Opus") {
		t.Errorf("State %q, Notice %q; want done, saying Music cannot open Opus", done.State, done.Notice)
	}
}

func TestSplitTracksGoToMusicWithoutTheRecording(t *testing.T) {
	music := &recordingMusic{}
	o := musicOptions()
	o.Music.AddToMusic = true
	runWithMusic(t, chapterRunner(), music, o)

	want := []string{"/m/01 - Opening Theme.m4a", "/m/02 - Second Movement.m4a", "/m/03 - Finale.m4a"}
	if strings.Join(music.added, "|") != strings.Join(want, "|") {
		t.Errorf("added %v, want the three tracks and not the hour-long recording", music.added)
	}
}

func TestAMusicFailureDoesNotFailTheDownload(t *testing.T) {
	music := &recordingMusic{err: ErrNoMusicFolder}
	o := Options{URL: "https://example.com/v", Pick: PickAudioMP3, Music: Music{AddToMusic: true}}
	done := runWithMusic(t, fileRunner("/tmp/Song MP3.mp3"), music, o)

	if done.State != StateDone || done.FilePath != "/tmp/Song MP3.mp3" {
		t.Errorf("State %q, FilePath %q; want the download done", done.State, done.FilePath)
	}
	if done.Notice == "" || !strings.Contains(done.Detail, "Open Music once") {
		t.Errorf("Notice %q, Detail %q; want the failure and its reason", done.Notice, done.Detail)
	}
}
