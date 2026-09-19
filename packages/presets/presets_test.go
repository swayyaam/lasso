package presets

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/swayyaam/lasso/packages/core"
)

func TestBuiltinsAreComplete(t *testing.T) {
	want := []string{"Best quality", "Archive (MKV)", "Podcast audio", "Music", "Album"}

	builtins := Builtins()
	if len(builtins) != len(want) {
		t.Fatalf("got %d built-ins, want %d", len(builtins), len(want))
	}
	for i, name := range want {
		if builtins[i].Name != name {
			t.Errorf("built-in %d is %q, want %q", i, builtins[i].Name, name)
		}
		if !builtins[i].BuiltIn {
			t.Errorf("%q is not marked built-in", builtins[i].Name)
		}
		if builtins[i].ID == "" {
			t.Errorf("%q has no id", builtins[i].Name)
		}
	}
}

func TestBuiltinsProduceValidArguments(t *testing.T) {
	// Every shipped preset must survive validation and produce usable arguments.
	for _, p := range Builtins() {
		t.Run(p.Name, func(t *testing.T) {
			o := p.Apply("https://example.com/watch?v=abc")
			if err := o.Validate(); err != nil {
				t.Fatalf("preset does not validate: %v", err)
			}
			args := core.BuildArgs(o)
			if len(args) == 0 {
				t.Fatal("no arguments produced")
			}
			if args[len(args)-1] != o.URL {
				t.Error("URL is not last")
			}
		})
	}
}

func TestBuiltinIntent(t *testing.T) {
	byID := map[string]Preset{}
	for _, p := range Builtins() {
		byID[p.ID] = p
	}

	archive := byID[IDArchiveMKV]
	if archive.Options.Container != core.ContainerMKV {
		t.Error("Archive preset does not use MKV")
	}
	if !archive.Options.Subtitles.Embed {
		t.Error("Archive preset does not embed subtitles")
	}

	podcast := byID[IDPodcastAudio]
	if !podcast.Options.Pick.IsAudioOnly() {
		t.Error("Podcast preset is not audio-only")
	}
	if podcast.Options.Enhancements.SponsorBlock != core.SponsorBlockRemove {
		t.Error("Podcast preset does not remove sponsor segments")
	}

	// The music preset takes the site's own stream rather than re-encoding it.
	// Every site this is used with serves lossy audio, so FLAC here would be a
	// larger file of exactly the same sound.
	music := byID[IDMusic]
	if music.Options.Pick != core.PickAudioOriginal {
		t.Errorf("Music preset picks %q, want the un-re-encoded stream", music.Options.Pick)
	}
	if !music.Options.Music.Tags {
		t.Error("Music preset does not tag")
	}
	if music.Options.Music.SplitChapters {
		t.Error("Music preset splits chapters; most music links are one track")
	}

	album := byID[IDAlbum]
	if !album.Options.Music.SplitChapters {
		t.Error("Album preset does not split chapters, which is the whole difference")
	}
	if !album.Options.Music.Tags {
		t.Error("Album preset does not tag")
	}
}

func TestBuiltinsAreImmutable(t *testing.T) {
	// A caller mutating what Builtins returns must not affect the next call.
	first := Builtins()
	first[0].Name = "hacked"
	first[0].Options.Pick = core.PickAudioMP3

	if Builtins()[0].Name == "hacked" {
		t.Error("built-ins were mutated by a caller")
	}
}

func TestPresetApplyDoesNotMutate(t *testing.T) {
	p := Builtins()[0]
	o := p.Apply("https://example.com/x")

	if o.URL != "https://example.com/x" {
		t.Errorf("URL = %q", o.URL)
	}
	if p.Options.URL != "" {
		t.Error("Apply wrote the URL back into the preset")
	}
}

func newStore(t *testing.T) (*Store, string) {
	t.Helper()
	dir := t.TempDir()
	s, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	return s, dir
}

func TestStoreStartsWithBuiltinsOnly(t *testing.T) {
	s, _ := newStore(t)

	if got := len(s.All()); got != len(Builtins()) {
		t.Errorf("All returned %d presets, want just the built-ins", got)
	}
	if got := len(s.UserPresets()); got != 0 {
		t.Errorf("UserPresets returned %d, want none", got)
	}
}

func TestSaveAndRetrieve(t *testing.T) {
	s, _ := newStore(t)

	o := core.Options{Pick: core.Pick1080p, Container: core.ContainerMP4}
	saved, err := s.Save("My preset", o)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if saved.BuiltIn {
		t.Error("a saved preset is marked built-in")
	}
	if !strings.HasPrefix(saved.ID, "user-") {
		t.Errorf("id = %q, want a user id", saved.ID)
	}

	got, ok := s.Get(saved.ID)
	if !ok {
		t.Fatal("saved preset could not be found")
	}
	if got.Options.Pick != core.Pick1080p {
		t.Errorf("Pick = %q", got.Options.Pick)
	}
}

func TestSaveDropsURL(t *testing.T) {
	s, _ := newStore(t)

	// A preset describes how to download, never what.
	saved, err := s.Save("With URL", core.Options{URL: "https://example.com/x", Pick: core.PickBest})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if saved.Options.URL != "" {
		t.Errorf("Options.URL = %q, want it dropped", saved.Options.URL)
	}
}

func TestSaveRejectsBadNames(t *testing.T) {
	s, _ := newStore(t)

	for _, name := range []string{"", "   ", "\t\n"} {
		if _, err := s.Save(name, core.Options{}); err == nil {
			t.Errorf("Save accepted the name %q", name)
		}
	}
	if _, err := s.Save(strings.Repeat("x", MaxNameLength+1), core.Options{}); err == nil {
		t.Error("Save accepted an over-long name")
	}
}

func TestSaveTrimsName(t *testing.T) {
	s, _ := newStore(t)
	saved, err := s.Save("  Padded  ", core.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if saved.Name != "Padded" {
		t.Errorf("Name = %q, want it trimmed", saved.Name)
	}
}

func TestRenameAndDelete(t *testing.T) {
	s, _ := newStore(t)
	saved, _ := s.Save("Original", core.Options{Pick: core.PickBest})

	if err := s.Rename(saved.ID, "Renamed"); err != nil {
		t.Fatalf("Rename: %v", err)
	}
	got, _ := s.Get(saved.ID)
	if got.Name != "Renamed" {
		t.Errorf("Name = %q", got.Name)
	}

	if err := s.Delete(saved.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, ok := s.Get(saved.ID); ok {
		t.Error("preset survived deletion")
	}
}

func TestUpdateOptions(t *testing.T) {
	s, _ := newStore(t)
	saved, _ := s.Save("Mine", core.Options{Pick: core.Pick720p})

	if err := s.Update(saved.ID, core.Options{Pick: core.Pick2160p, URL: "https://example.com/x"}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, _ := s.Get(saved.ID)
	if got.Options.Pick != core.Pick2160p {
		t.Errorf("Pick = %q", got.Options.Pick)
	}
	if got.Options.URL != "" {
		t.Error("Update kept a URL")
	}
	if got.Name != "Mine" {
		t.Errorf("Update changed the name to %q", got.Name)
	}
}

func TestBuiltinsCannotBeChanged(t *testing.T) {
	s, _ := newStore(t)

	for _, op := range []struct {
		name string
		err  error
	}{
		{"rename", s.Rename(IDBestQuality, "Something else")},
		{"delete", s.Delete(IDArchiveMKV)},
		{"update", s.Update(IDMusic, core.Options{})},
	} {
		if !errors.Is(op.err, ErrReadOnly) {
			t.Errorf("%s on a built-in returned %v, want ErrReadOnly", op.name, op.err)
		}
	}
}

func TestOperationsOnUnknownID(t *testing.T) {
	s, _ := newStore(t)

	if !errors.Is(s.Rename("user-nope", "x"), ErrNotFound) {
		t.Error("Rename on an unknown id did not report not-found")
	}
	if !errors.Is(s.Delete("user-nope"), ErrNotFound) {
		t.Error("Delete on an unknown id did not report not-found")
	}
	if !errors.Is(s.Update("user-nope", core.Options{}), ErrNotFound) {
		t.Error("Update on an unknown id did not report not-found")
	}
}

func TestPresetsSurviveReopening(t *testing.T) {
	s, dir := newStore(t)
	saved, _ := s.Save("Persisted", core.Options{Pick: core.PickAudioOpus, Container: core.ContainerMKV})

	reopened, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	got, ok := reopened.Get(saved.ID)
	if !ok {
		t.Fatal("preset did not survive reopening")
	}
	if got.Name != "Persisted" || got.Options.Pick != core.PickAudioOpus {
		t.Errorf("reloaded preset = %+v", got)
	}
}

func TestCorruptFileFallsBackToBuiltins(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Refusing to launch over a damaged convenience file would be worse than
	// starting with the built-ins.
	s, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore refused to open a corrupt file: %v", err)
	}
	if got := len(s.All()); got != len(Builtins()) {
		t.Errorf("All returned %d, want just the built-ins", got)
	}
	if _, err := s.Save("Recovered", core.Options{}); err != nil {
		t.Errorf("could not save after a corrupt file: %v", err)
	}
}

func TestFileCannotShadowBuiltin(t *testing.T) {
	dir := t.TempDir()
	content := `{"version":1,"presets":[{"id":"` + IDBestQuality + `","name":"Impostor","builtIn":true}]}`
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	s, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := s.Get(IDBestQuality)
	if !ok {
		t.Fatal("the real built-in disappeared")
	}
	if got.Name != "Best quality" {
		t.Errorf("built-in was shadowed by %q", got.Name)
	}
}

func TestWritesAreAtomic(t *testing.T) {
	s, dir := newStore(t)
	if _, err := s.Save("One", core.Options{}); err != nil {
		t.Fatal(err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".presets-") {
			t.Errorf("temporary file %s was left behind", e.Name())
		}
	}
}

func TestIDsAreUnique(t *testing.T) {
	s, _ := newStore(t)

	seen := map[string]bool{}
	for i := 0; i < 50; i++ {
		p, err := s.Save("Preset", core.Options{})
		if err != nil {
			t.Fatal(err)
		}
		if seen[p.ID] {
			t.Fatalf("duplicate id %s", p.ID)
		}
		seen[p.ID] = true
	}
}

func TestNewStoreNeedsDirectory(t *testing.T) {
	if _, err := NewStore(""); err == nil {
		t.Error("NewStore accepted an empty directory")
	}
}
