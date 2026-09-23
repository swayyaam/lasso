package core

import (
	"strings"
	"testing"
)

func entries(n int) []Entry {
	out := make([]Entry, n)
	for i := range out {
		id := string(rune('a' + i))
		out[i] = Entry{ID: id, Title: "Video " + id, URL: "https://www.youtube.com/watch?v=" + id}
	}
	return out
}

func ids(es []Entry) string {
	var b strings.Builder
	for _, e := range es {
		b.WriteString(e.ID)
	}
	return b.String()
}

func TestPlaylistEntriesMatchYtDlpsRangeRules(t *testing.T) {
	all := entries(6) // a..f
	cases := []struct {
		p    Playlist
		want string
	}{
		{Playlist{}, "abcdef"},
		{Playlist{Start: 2, End: 4}, "bcd"},
		{Playlist{Start: 5}, "ef"},
		{Playlist{End: 2}, "ab"},
		{Playlist{Reverse: true}, "fedcba"},
		// Reverse applies to what the range selected.
		{Playlist{Start: 2, End: 4, Reverse: true}, "dcb"},
		{Playlist{Start: 9}, ""},
		{Playlist{End: 99}, "abcdef"},
	}
	for _, tc := range cases {
		if got := ids(PlaylistEntries(all, tc.p)); got != tc.want {
			t.Errorf("%+v selected %q, want %q", tc.p, got, tc.want)
		}
	}
}

func TestEachVideoGetsItsOwnDownload(t *testing.T) {
	playlist := Options{URL: "https://www.youtube.com/playlist?list=PL1", Pick: Pick1080p,
		Playlist: Playlist{Start: 2, End: 10}}
	e := Entry{ID: "x", Title: "Blender 4.2 LTS", URL: "https://www.youtube.com/watch?v=x"}

	o, err := playlist.ForEntry(e, "Blender Releases")
	if err != nil {
		t.Fatal(err)
	}
	if o.URL != e.URL {
		t.Errorf("URL = %q, want the video's own link", o.URL)
	}
	if o.Playlist != (Playlist{}) {
		t.Errorf("Playlist = %+v; a single video must not carry the playlist's range", o.Playlist)
	}
	if o.Pick != Pick1080p {
		t.Error("the quality chosen for the playlist was not carried to its videos")
	}
	if o.Output.Template != "Blender Releases/"+Pick1080p.defaultTemplate() {
		t.Errorf("template = %q, want the videos gathered in a folder named after the playlist", o.Output.Template)
	}
}

func TestPlaylistFolderIsSafeAsAPathAndATemplate(t *testing.T) {
	cases := map[string]string{
		"Blender Releases": "Blender Releases",
		"../../Library":    "-..-Library",
		"AC/DC: Live":      "AC-DC- Live",
		"...hidden":        "hidden",
		"100% Hits":        "100%% Hits", // or yt-dlp reads it as a field
		"line\nbreak\ttab": "linebreaktab",
		"   ":              "",
	}
	for in, want := range cases {
		if got := PlaylistFolder(in); got != want {
			t.Errorf("PlaylistFolder(%q) = %q, want %q", in, got, want)
		}
	}
	// Whatever the title, the result passes the template rules.
	for in := range cases {
		o := Options{URL: "https://example.com/v", Output: Output{Template: PlaylistFolder(in) + "/" + DefaultTemplate}}
		if err := o.Validate(); err != nil && PlaylistFolder(in) != "" {
			t.Errorf("a template from %q failed validation: %v", in, err)
		}
	}
}

func TestAnEntryWithoutAUsableLinkIsRefused(t *testing.T) {
	// Some extractors give an ID, not a link. It is not guessed at.
	_, err := Options{Pick: PickBest}.ForEntry(Entry{ID: "x", Title: "No link", URL: "x"}, "P")
	if err == nil {
		t.Error("an entry with no http(s) link became a download")
	}
}

func TestPlaylistVideosShareAGroupAndAreSaved(t *testing.T) {
	h := newQueueHarnessWith(t, blockingRunner(), 1, nil)
	group := NewGroupID()
	a, _ := h.q.AddToGroup(uniqueOptions(), Source{Title: "One"}, group, "Mix")
	b, _ := h.q.AddToGroup(uniqueOptions(), Source{Title: "Two"}, group, "Mix")
	if a.Group != group || b.Group != group || a.GroupTitle != "Mix" {
		t.Errorf("group fields = %q/%q, %q; want both in %q", a.Group, a.GroupTitle, b.Group, group)
	}
	if a.ID == b.ID {
		t.Error("two videos of one playlist got the same ID")
	}
}
