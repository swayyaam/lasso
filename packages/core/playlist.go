package core

import (
	"fmt"
	"strings"
	"unicode"
)

// PlaylistEntries picks which videos of a playlist to download, the way
// yt-dlp's --playlist-items and --playlist-reverse would have: Start and End
// are 1-based and inclusive, either may be 0 for "from the first" or "to the
// last", and Reverse applies to what the range selected.
//
// A playlist used to download as one yt-dlp run — one row, one progress bar
// resetting for every video, one history entry, no retrying a single video.
// Selecting here is what lets each video be its own download.
func PlaylistEntries(entries []Entry, p Playlist) []Entry {
	start, end := 1, len(entries)
	if p.Start > 0 {
		start = p.Start
	}
	if p.End > 0 && p.End < end {
		end = p.End
	}
	if start > end || start > len(entries) {
		return []Entry{}
	}
	picked := append([]Entry(nil), entries[start-1:end]...)
	if p.Reverse {
		for i, j := 0, len(picked)-1; i < j; i, j = i+1, j-1 {
			picked[i], picked[j] = picked[j], picked[i]
		}
	}
	return picked
}

// ForEntry turns the options chosen for a playlist into the options for one
// of its videos: that video's own link, no playlist range — it is a single
// video now — and a folder named after the playlist, so a hundred files do not
// land loose in the download folder.
func (o Options) ForEntry(e Entry, playlistTitle string) (Options, error) {
	out := o
	out.URL = strings.TrimSpace(e.URL)
	out.Playlist = Playlist{}

	template := o.Output.Template
	if template == "" {
		template = DefaultTemplate
	}
	if folder := PlaylistFolder(playlistTitle); folder != "" {
		template = folder + "/" + template
	}
	out.Output.Template = template

	// The entry's link came from the site. It is checked like any other.
	if err := out.Validate(); err != nil {
		return Options{}, fmt.Errorf("%q: %w", e.Title, err)
	}
	return out, nil
}

// PlaylistFolder makes a playlist's title safe as one folder name inside a
// yt-dlp output template.
//
// The title is whatever the uploader typed, so it is treated like any other
// untrusted string: no separators to climb out of the folder with, no leading
// dots to hide it or make "..", no control characters, and "%" doubled —
// otherwise a title like "100% Hits" would be read as template syntax.
func PlaylistFolder(title string) string {
	var b strings.Builder
	for _, r := range title {
		switch {
		case r == '/' || r == '\\' || r == ':':
			b.WriteRune('-')
		case unicode.IsControl(r):
			// dropped
		default:
			b.WriteRune(r)
		}
	}
	folder := strings.Trim(strings.TrimSpace(b.String()), ".")
	folder = strings.TrimSpace(folder)
	if r := []rune(folder); len(r) > 100 {
		folder = strings.TrimSpace(string(r[:100]))
	}
	return strings.ReplaceAll(folder, "%", "%%")
}
