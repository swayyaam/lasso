package core

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Kind distinguishes a single video from a playlist.
type Kind string

const (
	KindVideo    Kind = "video"
	KindPlaylist Kind = "playlist"
)

// Thumbnail is one available preview image.
type Thumbnail struct {
	URL    string `json:"url"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	// Preference is yt-dlp's own ranking; higher is better. Many thumbnails
	// carry a preference but no dimensions.
	Preference int `json:"preference"`
}

// Format is one downloadable stream.
type Format struct {
	ID             string  `json:"id"`
	Ext            string  `json:"ext"`
	Note           string  `json:"note"`
	Width          int     `json:"width"`
	Height         int     `json:"height"`
	FPS            float64 `json:"fps"`
	VCodec         string  `json:"vcodec"`
	ACodec         string  `json:"acodec"`
	Filesize       int64   `json:"filesize"`
	FilesizeApprox int64   `json:"filesizeApprox"`
	TBR            float64 `json:"tbr"`
	// DynamicRange is yt-dlp's own word for the format's dynamic range:
	// "SDR", "HDR10", "HDR10+", "HLG", "DV". It is the only reliable place
	// this is stated — the format note usually does not mention it.
	DynamicRange string `json:"dynamicRange"`
}

// HasVideo reports whether the format carries a video track.
//
// yt-dlp says "none" when it knows there is no video, and says nothing at all
// when it does not know — which is how archive.org lists every format. Treating
// "not stated" as "none" made the whole quality picker vanish for those links,
// so a format with dimensions counts as video whatever its codec is called.
func (f Format) HasVideo() bool {
	switch f.VCodec {
	case "none":
		return false
	case "":
		return f.Width > 0 || f.Height > 0
	default:
		return true
	}
}

// HasAudio reports whether the format carries an audio track.
//
// An unstated codec counts as audio. Extractors that know a stream is silent
// say "none" — every video-only DASH stream does — so a whole file from a site
// that names no codecs is, in practice, a file with sound.
func (f Format) HasAudio() bool { return f.ACodec != "none" }

// Size is the best known size in bytes, exact if available and estimated
// otherwise, or zero when unknown.
func (f Format) Size() int64 {
	if f.Filesize > 0 {
		return f.Filesize
	}
	return f.FilesizeApprox
}

// Entry is one item of a playlist, as returned by --flat-playlist. Entries are
// deliberately shallow: yt-dlp has not visited the individual videos.
type Entry struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	// TitleGuessed is true when the site listed no title and Title was read
	// off the link; the real one arrives later from LookUpEntries.
	TitleGuessed bool        `json:"titleGuessed"`
	URL          string      `json:"url"`
	Duration     float64     `json:"duration"`
	Uploader     string      `json:"uploader"`
	Thumbnails   []Thumbnail `json:"thumbnails"`
}

// Metadata is what Lasso shows after resolving a link.
type Metadata struct {
	Kind       Kind        `json:"kind"`
	ID         string      `json:"id"`
	Title      string      `json:"title"`
	Uploader   string      `json:"uploader"`
	WebpageURL string      `json:"webpageUrl"`
	Duration   float64     `json:"duration"`
	Thumbnails []Thumbnail `json:"thumbnails"`
	Formats    []Format    `json:"formats"`
	Entries    []Entry     `json:"entries"`

	// Quality is what this link can actually be downloaded as, derived from
	// Formats. The UI offers exactly these rungs, so it can never present a
	// resolution the source does not have.
	Quality QualityOptions `json:"quality"`

	// Live is where the video is in a broadcast's life; see Unavailable.
	Live LiveStatus `json:"live"`
	// Blocked, when set, is why this cannot be downloaded yet — the sentence
	// the interface shows in place of a Download button.
	Blocked string `json:"blocked"`
}

// IsPlaylist reports whether this resolved to more than one item.
func (m Metadata) IsPlaylist() bool { return m.Kind == KindPlaylist }

// Count is the number of downloadable items.
func (m Metadata) Count() int {
	if m.IsPlaylist() {
		return len(m.Entries)
	}
	return 1
}

// rawMetadata mirrors yt-dlp's -J document. It exists so the exported types can
// use Lasso's own names and stay stable if yt-dlp's field names shift.
type rawMetadata struct {
	Type         string   `json:"_type"`
	ExtractorKey string   `json:"extractor_key"`
	ID           string   `json:"id"`
	Title        string   `json:"title"`
	Uploader     string   `json:"uploader"`
	Channel      string   `json:"channel"`
	UploaderID   string   `json:"uploader_id"`
	WebpageURL   string   `json:"webpage_url"`
	Duration     *float64 `json:"duration"`
	LiveStatus   string   `json:"live_status"`
	IsLive       *bool    `json:"is_live"`

	Thumbnails []rawThumbnail `json:"thumbnails"`
	Formats    []rawFormat    `json:"formats"`
	Entries    []rawEntry     `json:"entries"`
}

type rawThumbnail struct {
	URL        string `json:"url"`
	Width      int    `json:"width"`
	Height     int    `json:"height"`
	Preference int    `json:"preference"`
}

type rawFormat struct {
	FormatID       string   `json:"format_id"`
	Ext            string   `json:"ext"`
	FormatNote     string   `json:"format_note"`
	Width          int      `json:"width"`
	Height         int      `json:"height"`
	FPS            *float64 `json:"fps"`
	VCodec         string   `json:"vcodec"`
	ACodec         string   `json:"acodec"`
	Filesize       *int64   `json:"filesize"`
	FilesizeApprox *int64   `json:"filesize_approx"`
	TBR            *float64 `json:"tbr"`
	ABR            *float64 `json:"abr"`
	DynamicRange   string   `json:"dynamic_range"`
}

type rawEntry struct {
	IEKey      string         `json:"ie_key"`
	ID         string         `json:"id"`
	Title      string         `json:"title"`
	URL        string         `json:"url"`
	Duration   *float64       `json:"duration"`
	Uploader   string         `json:"uploader"`
	Channel    string         `json:"channel"`
	Thumbnails []rawThumbnail `json:"thumbnails"`
}

// ParseMetadata converts a yt-dlp -J document into Metadata.
func ParseMetadata(data []byte, playback Playback) (*Metadata, error) {
	var raw rawMetadata
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("could not read the video details: %w", err)
	}
	if raw.ID == "" && raw.Title == "" && len(raw.Entries) == 0 {
		return nil, fmt.Errorf("could not read the video details: the response was empty")
	}

	m := &Metadata{
		Kind:       KindVideo,
		ID:         raw.ID,
		Title:      raw.Title,
		Uploader:   firstNonEmpty(raw.Uploader, raw.Channel, raw.UploaderID),
		WebpageURL: raw.WebpageURL,
		Duration:   deref(raw.Duration),
		Thumbnails: convertThumbnails(raw.Thumbnails),
		Live:       LiveStatus(raw.LiveStatus),
	}
	// Older extractors only say is_live.
	if m.Live == LiveNone && raw.IsLive != nil && *raw.IsLive {
		m.Live = LiveNow
	}
	m.Blocked = m.Unavailable()

	if raw.Type == string(KindPlaylist) {
		m.Kind = KindPlaylist
		m.Entries = make([]Entry, 0, len(raw.Entries))
		for _, e := range raw.Entries {
			title, guessed := e.Title, false
			if strings.TrimSpace(title) == "" {
				// SoundCloud's flat set lists links and nothing else; a name
				// read off the link beats a list of URLs until the real one
				// is looked up (LookUpEntries).
				title, guessed = TitleFromLink(e.URL), true
			}
			m.Entries = append(m.Entries, Entry{
				ID:           e.ID,
				Title:        title,
				TitleGuessed: guessed,
				URL:          e.URL,
				Duration:     deref(e.Duration),
				Uploader:     firstNonEmpty(e.Uploader, e.Channel),
				Thumbnails:   convertThumbnails(e.Thumbnails),
			})
		}
		// A flat playlist has not visited its videos, so there are no formats
		// to measure: offer the standard ladder and let yt-dlp fall back per
		// item — unless the site has no video at all, when a set of songs
		// opening on the video ladder was just wrong.
		m.Quality = GenericQualityOptions()
		if audioOnlySite(raw) {
			// These sites serve MP3, AAC or Opus, every one of which takes a
			// cover; a set has not looked at its tracks to say so itself.
			m.Quality = QualityOptions{HasAudio: true, Approximate: true, OriginalTakesCover: true}
		}
		return m, nil
	}

	m.Formats = make([]Format, 0, len(raw.Formats))
	for _, f := range raw.Formats {
		m.Formats = append(m.Formats, Format{
			ID:             f.FormatID,
			Ext:            f.Ext,
			Note:           f.FormatNote,
			Width:          f.Width,
			Height:         f.Height,
			FPS:            deref(f.FPS),
			VCodec:         f.VCodec,
			ACodec:         f.ACodec,
			Filesize:       deref(f.Filesize),
			FilesizeApprox: deref(f.FilesizeApprox),
			TBR:            deref(f.TBR),
			DynamicRange:   f.DynamicRange,
		})
	}
	m.Quality = AnalyseFormats(m.Formats, playback)
	m.Quality.AudioSizes = AudioSizes(m.Formats, m.Duration)
	if original := originalAudio(raw.Formats); !original.IsZero() {
		m.Quality.OriginalAudio = original
		m.Quality.OriginalTakesCover = original.TakesCover()
		// Original is that stream, so it weighs what that stream weighs, not
		// what the largest one does.
		if original.Bytes > 0 {
			if m.Quality.AudioSizes == nil {
				m.Quality.AudioSizes = map[QuickPick]AudioSize{}
			}
			m.Quality.AudioSizes[PickAudioOriginal] = AudioSize{Bytes: original.Bytes}
			m.Quality.AudioBytes = original.Bytes
		}
	}
	return m, nil
}

func convertThumbnails(raw []rawThumbnail) []Thumbnail {
	out := make([]Thumbnail, 0, len(raw))
	for _, t := range raw {
		out = append(out, Thumbnail{URL: t.URL, Width: t.Width, Height: t.Height, Preference: t.Preference})
	}
	return out
}

// BestThumbnail picks the thumbnail to display at a given width.
//
// It prefers the smallest image at least as wide as needed, so the result is
// downscaled rather than blown up, and falls back to the largest available when
// nothing is big enough. Most thumbnails in a real response carry no dimensions
// at all — for those, yt-dlp's own preference ordering is the only signal, and
// the last entry is its best.
func BestThumbnail(thumbnails []Thumbnail, width int) (Thumbnail, bool) {
	var best Thumbnail
	var largest Thumbnail
	found := false

	for _, t := range thumbnails {
		if t.URL == "" || t.Width <= 0 {
			continue
		}
		if t.Width > largest.Width {
			largest = t
		}
		if t.Width >= width && (!found || t.Width < best.Width) {
			best = t
			found = true
		}
	}
	if found {
		return best, true
	}
	if largest.URL != "" {
		return largest, true
	}

	// No dimensions anywhere: trust yt-dlp's ordering, which runs worst to best.
	for i := len(thumbnails) - 1; i >= 0; i-- {
		if thumbnails[i].URL != "" {
			return thumbnails[i], true
		}
	}
	return Thumbnail{}, false
}

// MetadataArgs builds the arguments for resolving a link.
//
// --flat-playlist is always passed: for a playlist it avoids visiting every
// video, and for a single video it changes nothing — formats and thumbnails
// still come back in full.
func MetadataArgs(o Options) []string {
	args := []string{"--ignore-config", "--no-plugin-dirs", "-J", "--flat-playlist", "--no-warnings"}

	// Resolving a YouTube link needs the JavaScript runtime just as
	// downloading does; without it yt-dlp returns a reduced format list.
	if o.DenoPath != "" {
		args = append(args, "--js-runtimes", "deno:"+o.DenoPath)
	}

	// Cookies matter here too: age-restricted videos will not resolve without
	// them. Rate limits and output options have no bearing on metadata.
	if spec := cookieSpec(o); spec != "" {
		args = append(args, "--cookies-from-browser", spec)
	}
	return append(args, "--", strings.TrimSpace(o.URL))
}

// metadataTimeout bounds a resolve. Measured against YouTube, -J takes about
// two seconds warm and three cold, so this is slack for a slow network rather
// than a target.
const metadataTimeout = 90 * time.Second

// FetchMetadata resolves a link through yt-dlp.
func FetchMetadata(ctx context.Context, runner Runner, o Options) (*Metadata, error) {
	if err := o.Validate(); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, metadataTimeout)
	defer cancel()

	var out strings.Builder
	var errLines []string

	err := runner.Run(ctx, MetadataArgs(o),
		func(line string) { out.WriteString(line) },
		func(line string) { errLines = append(errLines, line) },
	)
	if err != nil {
		return nil, ClassifyError(strings.Join(errLines, "\n"), err)
	}
	if out.Len() == 0 {
		return nil, ClassifyError(strings.Join(errLines, "\n"), fmt.Errorf("yt-dlp returned nothing"))
	}
	return ParseMetadata([]byte(out.String()), o.Playback)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func deref[T any](p *T) T {
	if p == nil {
		var zero T
		return zero
	}
	return *p
}
