package core

import (
	"context"
	"strings"
	"testing"
)

func TestASoundCloudSetOpensOnAudio(t *testing.T) {
	// A real flat listing of a public-domain set: SoundcloudSet, its tracks
	// Soundcloud, and not one title or length among them.
	m, err := ParseMetadata(readFixture(t, "soundcloud-set.json"), Playback{})
	if err != nil {
		t.Fatal(err)
	}
	if !m.IsPlaylist() {
		t.Fatal("not a playlist")
	}
	if m.Quality.HasVideo || !m.Quality.HasAudio {
		t.Errorf("quality = video %v, audio %v; a SoundCloud set has sound and nothing else", m.Quality.HasVideo, m.Quality.HasAudio)
	}
	if !m.Quality.OriginalTakesCover {
		t.Error("a SoundCloud set's tracks lost their cover art: every format the site serves takes one")
	}
	if len(m.Quality.Tiers) != 0 {
		t.Errorf("a set of songs offered %d resolutions", len(m.Quality.Tiers))
	}
	first := m.Entries[0]
	if first.Title != "Crown the invisible the" || !first.TitleGuessed {
		t.Errorf("first entry = %q (guessed %v), want a name read off its link", first.Title, first.TitleGuessed)
	}
}

func TestAVideoPlaylistKeepsTheLadder(t *testing.T) {
	raw := `{"_type":"playlist","extractor_key":"YoutubeTab","title":"Blender films","entries":[` +
		`{"ie_key":"Youtube","id":"aqz-KE-bpKQ","url":"https://www.youtube.com/watch?v=aqz-KE-bpKQ","title":"Big Buck Bunny"}]}`
	m, err := ParseMetadata([]byte(raw), Playback{})
	if err != nil {
		t.Fatal(err)
	}
	if !m.Quality.HasVideo || len(m.Quality.Tiers) == 0 {
		t.Error("a YouTube playlist lost its video ladder")
	}
	if m.Entries[0].Title != "Big Buck Bunny" || m.Entries[0].TitleGuessed {
		t.Errorf("a listed title was replaced: %+v", m.Entries[0])
	}
}

func TestTitleFromLink(t *testing.T) {
	cases := map[string]string{
		"https://soundcloud.com/freemusicarchive/world-on-fire-1": "World on fire 1",
		"https://soundcloud.com/a/b_c-d/":                         "B c d",
		"https://example.bandcamp.com/track/caf%C3%A9-del-mar":    "Café del mar",
		"https://soundcloud.com/":                                 "",
		"https://api-v2.soundcloud.com/tracks/90306537":           "",
		"": "",
	}
	for in, want := range cases {
		if got := TitleFromLink(in); got != want {
			t.Errorf("TitleFromLink(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLookUpEntriesReportsEachAsItArrives(t *testing.T) {
	var args []string
	runner := &funcRunner{run: func(_ context.Context, a []string, stdout, _ func(string)) error {
		args = a
		// Real lines, as yt-dlp printed them for this set.
		stdout(`{"id":"90306532","title":"Crown the Invisible - The Spaniard That Blighted My Life [CONTEST WINNER]","duration":245.466,"uploader":"freemusicarchive"}`)
		stdout(`not json`)
		stdout(`{"id":"90306533","title":null,"duration":0,"uploader":null}`)
		stdout(`{"id":"90306534","title":"The Procedure Club - Let Us Leave The Town Henry Purcell","duration":147.779,"uploader":"freemusicarchive"}`)
		return nil
	}}
	var got []EntryInfo
	err := LookUpEntries(context.Background(), runner, Options{URL: "https://soundcloud.com/freemusicarchive/sets/revitalize-music-contest"},
		func(e EntryInfo) { got = append(got, e) })
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != "90306532" || got[1].Duration != 147.779 {
		t.Errorf("found %+v; want the two lines that named a track", got)
	}
	joined := strings.Join(args, " ")
	for _, want := range []string{"--ignore-config --no-plugin-dirs", "--skip-download", "--playlist-end 100"} {
		if !strings.Contains(joined, want) {
			t.Errorf("lookup lacks %q: %s", want, joined)
		}
	}
	if args[len(args)-1] != "https://soundcloud.com/freemusicarchive/sets/revitalize-music-contest" || args[len(args)-2] != "--" {
		t.Error("the link is not last, after --")
	}
}
