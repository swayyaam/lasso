package core

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"
	"strings"
	"unicode"
)

// audioOnlyExtractors are sites with no video at all. yt-dlp names its
// extractor for a playlist and for each flat entry (SoundcloudSet, and
// Soundcloud for its tracks), and a set from one of these used to open on the
// video ladder, because a flat playlist has no formats to say otherwise.
var audioOnlyExtractors = []string{"Soundcloud", "Bandcamp", "Mixcloud", "Audiomack"}

func audioOnlySite(raw rawMetadata) bool {
	keys := []string{raw.ExtractorKey}
	for _, e := range raw.Entries {
		keys = append(keys, e.IEKey)
	}
	for _, key := range keys {
		for _, site := range audioOnlyExtractors {
			if key != "" && strings.HasPrefix(key, site) {
				return true
			}
		}
	}
	return false
}

// TitleFromLink makes a readable name from a link's last part, for an entry
// the site listed without one: ".../world-on-fire-1" is "World on fire 1".
func TitleFromLink(link string) string {
	u, err := url.Parse(strings.TrimSpace(link))
	if err != nil {
		return ""
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	last := parts[len(parts)-1]
	if unescaped, err := url.PathUnescape(last); err == nil {
		last = unescaped
	}
	words := strings.FieldsFunc(last, func(r rune) bool { return r == '-' || r == '_' || r == '+' || unicode.IsSpace(r) })
	if len(words) == 0 {
		return ""
	}
	// Past its first few tracks a SoundCloud set lists API links that end in
	// the track's number, which is no name at all.
	if strings.IndexFunc(last, func(r rune) bool { return !unicode.IsDigit(r) }) < 0 {
		return ""
	}
	title := strings.Join(words, " ")
	runes := []rune(title)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}

// MaxEntryLookup is the largest set whose names are looked up. Each costs a
// request to the site, about two and a half seconds on SoundCloud (11 tracks
// took 29 s), so past this the names read off the links stand.
const MaxEntryLookup = 100

// EntryInfo is one track's real details, from LookUpEntries.
type EntryInfo struct {
	ID       string  `json:"id"`
	Title    string  `json:"title"`
	Duration float64 `json:"duration"`
	Uploader string  `json:"uploader"`
}

// LookUpEntries resolves each track of a set for its name and length, and
// reports each one as it arrives, so a list shown straight away fills in
// rather than waiting half a minute. yt-dlp visits them in order, one request
// each; the context stops it when the person moves on.
func LookUpEntries(ctx context.Context, runner Runner, o Options, found func(EntryInfo)) error {
	args := []string{"--ignore-config", "--no-plugin-dirs", "--no-warnings", "--skip-download",
		"--playlist-end", strconv.Itoa(MaxEntryLookup),
		"--print", `{"id":%(id)j,"title":%(title|null)j,"duration":%(duration|0)j,"uploader":%(uploader|null)j}`}
	if o.DenoPath != "" {
		args = append(args, "--js-runtimes", "deno:"+o.DenoPath)
	}
	if spec := cookieSpec(o); spec != "" {
		args = append(args, "--cookies-from-browser", spec)
	}
	args = append(args, "--", strings.TrimSpace(o.URL))

	line := func(text string) {
		var info EntryInfo
		if json.Unmarshal([]byte(strings.TrimSpace(text)), &info) == nil && info.ID != "" && info.Title != "" {
			found(info)
		}
	}
	return runner.Run(ctx, args, line, func(string) {})
}
