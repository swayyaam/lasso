package core

import (
	"fmt"
	"strconv"
	"strings"
)

// BuildArgs turns Options into the yt-dlp arguments that determine the result.
//
// It is a pure function: no I/O, no environment, no clock, and a stable
// argument order, so the same Options always produce the same slice. This is
// also exactly what the "show command" panel displays — pasting it into a
// terminal reproduces the same file. Flags that only shape how progress is
// reported live in ExecArgs instead.
func BuildArgs(o Options) []string {
	var args []string

	// First, so nothing outside Lasso changes the result: --ignore-config
	// skips the user's own ~/.config/yt-dlp/config, and --no-plugin-dirs skips
	// yt-dlp's default plugin folders, which --ignore-config leaves live. A
	// plugin installed for command-line use would otherwise run inside every
	// Lasso download.
	args = append(args, "--ignore-config", "--no-plugin-dirs")

	// Stated rather than assumed. Resuming a paused download is exactly this
	// flag continuing a .part file, and yt-dlp's default being the same today
	// is not a reason to leave the behaviour Lasso depends on unwritten.
	args = append(args, "--continue")

	args = append(args, formatArgs(o)...)
	args = append(args, sortArgs(o)...)
	args = append(args, containerArgs(o)...)
	args = append(args, subtitleArgs(o)...)
	args = append(args, enhancementArgs(o)...)
	args = append(args, musicArgs(o)...)
	args = append(args, playlistArgs(o)...)
	args = append(args, networkArgs(o)...)
	args = append(args, outputArgs(o)...)

	if o.FFmpegLocation != "" {
		args = append(args, "--ffmpeg-location", o.FFmpegLocation)
	}
	// Named explicitly rather than left to PATH. Lasso's own subprocess has the
	// bundled deno on PATH, but the point of the shown command is that it can
	// be pasted into a terminal — and there, without this, yt-dlp reports "no
	// supported JavaScript runtime" and quietly returns fewer formats.
	if o.DenoPath != "" {
		args = append(args, "--js-runtimes", "deno:"+o.DenoPath)
	}

	return append(args, "--", strings.TrimSpace(o.URL))
}

// progressTemplate asks yt-dlp for machine-readable progress: one JSON object
// per line, with raw numbers rather than the pre-formatted human strings, so
// the parser never has to undo formatting.
//
// Every field carries a |0 default. Without one, a field yt-dlp cannot supply
// renders as a bare NA, which would make the line invalid JSON — total_bytes is
// unavailable on fragmented downloads, and speed and eta are unavailable on the
// final line of every download. Zero therefore means "not known yet".
const progressTemplate = `download:{"stage":"downloading",` +
	`"downloaded":%(progress.downloaded_bytes|0)j,` +
	`"total":%(progress.total_bytes|0)j,` +
	`"estimate":%(progress.total_bytes_estimate|0)j,` +
	`"speed":%(progress.speed|0)j,` +
	`"eta":%(progress.eta|0)j,` +
	`"fragment":%(progress.fragment_index|0)j,` +
	`"fragments":%(progress.fragment_count|0)j}`

// completedTemplate asks yt-dlp to name the file it actually produced.
//
// "[download] Destination:" cannot answer this. It names the file being
// written, which any post-processor then replaces and deletes: extracting
// audio leaves a .mp3 where the .webm was, merging separate video and audio
// streams writes a third file and removes both, and a remux changes the
// extension. Reporting that path back means "Show in Finder" points at a file
// that no longer exists.
//
// after_move runs once per finished file, after every post-processor and after
// the final move into place, so %(filepath)s is the file on disk.
const completedTemplate = `after_move:{"stage":"complete","path":%(filepath)j,"width":%(width|0)j,"height":%(height|0)j}`

// ExecArgs is BuildArgs plus the flags that make progress machine-readable.
//
// These are deliberately excluded from the shown command: they change nothing
// about the resulting file, and pasting a JSON progress template into a
// terminal produces unreadable output.
func ExecArgs(o Options) []string {
	args := BuildArgs(o)
	// Insert before the trailing "--" and URL so the URL stays last. Build a
	// fresh slice rather than appending to a prefix of args: the prefix shares
	// its backing array with the tail, so appending would overwrite it.
	split := len(args) - 2

	out := make([]string, 0, len(args)+6)
	out = append(out, args[:split]...)
	out = append(out,
		"--newline",
		"--no-colors",
		"--progress-template", progressTemplate,
		"--print", completedTemplate,
		// --print implies --quiet, which silences the progress template along
		// with everything else. --no-quiet undoes that; it must come after.
		// after_move is a late stage, so --print does not also imply
		// --simulate here and the download still happens.
		"--no-quiet",
	)
	return append(out, args[split:]...)
}

// formatArgs builds the -f selector.
func formatArgs(o Options) []string {
	if format := o.Pick.audioFormat(); format != "" {
		args := []string{"-f", audioSelector(format), "-x", "--audio-format", format}
		// 0 is "best" for the lossy encoders. FLAC ignores it, and "best"
		// re-encodes nothing at all, so for both it would imply a quality knob
		// that does nothing.
		if format != "flac" && format != "best" {
			args = append(args, "--audio-quality", "0")
		}
		return args
	}

	if height := o.Pick.maxHeight(); height > 0 {
		h := strconv.Itoa(height)
		capped := "[height<=" + h + "]"
		// Fall back to the best available if nothing meets the cap, so a
		// 720p-max video still downloads when 1080p was requested.
		fallback := fmt.Sprintf("bv*%s+ba/b%s/bv*+ba/b", capped, capped)
		if o.wantsPlayable() {
			// A resolution asked for by name is honoured first: at that rung,
			// an encode this Mac plays if there is one, and otherwise
			// whatever the rung comes in. Someone who picked 4K on a Mac that
			// cannot play VP9 still gets 4K; the interface says what it needs.
			rung := capped + "[height>=" + strconv.Itoa(int(float64(height)*tierTolerance)) + "]"
			return []string{"-f", playableFirst(rung, o.Playback) + "/" + fallback}
		}
		return []string{"-f", fallback}
	}

	if o.wantsPlayable() {
		// "Best" is the best this Mac plays, not the best that exists. A
		// file QuickTime cannot open is not the best of anything to someone
		// who double-clicks it; the larger encode stays one named pick away.
		return []string{"-f", playableFirst("", o.Playback) + "/bv*+ba/b"}
	}
	return []string{"-f", "bv*+ba/b"}
}

// playableFirst prefers, within filter, video this Mac plays joined to AAC
// audio, then that video with any audio, then a single file that plays.
//
// AAC is asked for in the selector rather than through -S, because anything
// prepended to -S outranks yt-dlp's language preference — and that would
// trade a video's original-language track for a dubbed one to get a codec.
func playableFirst(filter string, p Playback) string {
	v := filter + p.selector()
	return "bv*" + v + "+ba[acodec^=mp4a]/bv*" + v + "+ba/b" + v
}

// wantsPlayable reports whether Lasso is choosing the codec and container,
// and so should choose ones that play on this Mac.
//
// A named container or codec is the user's decision and is left alone. So is
// asking for HDR on a Mac without AV1: YouTube's HDR is VP9 or AV1 only, so
// insisting on something QuickTime plays would quietly hand back SDR.
func (o Options) wantsPlayable() bool {
	if o.Pick.IsAudioOnly() || o.Container != ContainerAuto || o.VideoCodec != VideoCodecAuto {
		return false
	}
	return !o.PreferHDR || o.Playback.AV1
}

// audioSelector prefers a source stream that already matches the target format,
// so yt-dlp can remux instead of re-encoding and losing quality.
//
// FLAC deliberately has no such preference. There is no lossless source to
// match on the sites this is used with, so the only thing worth asking for is
// the best audio available — which -S abr already orders — and letting ffmpeg
// encode that. Matching on extension here would pick a container, not quality.
func audioSelector(format string) string {
	switch format {
	case "m4a":
		return "ba[ext=m4a]/ba/b"
	case "opus":
		return "ba[acodec=opus]/ba[ext=webm]/ba/b"
	default:
		return "ba/b"
	}
}

// sortArgs expresses soft preferences. Fields passed to -S are prepended to
// yt-dlp's default ordering, so an unavailable preference degrades instead of
// failing.
func sortArgs(o Options) []string {
	var fields []string

	// Resolution first, so it outranks codec preference: a user who asked for
	// 1080p wants 1080p in whatever codec, not AV1 at some other size. The -f
	// selector already caps the set; this orders what is left and makes the
	// fallback pick the closest rung rather than the smallest.
	if height := o.Pick.maxHeight(); height > 0 {
		fields = append(fields, "res:"+strconv.Itoa(height))
	}

	// After resolution, before codec. A user asking for HDR wants it at the
	// size they chose, not a smaller HDR encode — but they want it more than
	// they want any particular codec, since HDR is what the codec is carrying.
	//
	// yt-dlp's own ordering is DV > HDR12 > HDR10+ > HDR10 > HLG > SDR, so the
	// bare field is already "prefer the best dynamic range available". It is a
	// preference: a video with no HDR encode still gets the best it has.
	if o.PreferHDR && !o.Pick.IsAudioOnly() {
		fields = append(fields, "hdr")
	}

	if o.VideoCodec != VideoCodecAuto && !o.Pick.IsAudioOnly() {
		fields = append(fields, "vcodec:"+string(o.VideoCodec))
	}
	if o.AudioCodec != AudioCodecAuto {
		fields = append(fields, "acodec:"+string(o.AudioCodec))
	}

	// Stated rather than left to the default ordering. An audio-only download
	// is the whole file, so the highest bitrate the site offers is always the
	// right source — and it matters most for the lossless formats, where the
	// encode can only ever be as good as what it was given.
	if o.Pick.IsAudioOnly() {
		fields = append(fields, "abr")
	}
	// Steer stream selection toward something the container accepts natively.
	// MKV holds essentially anything, so it needs no hint.
	if !o.Pick.IsAudioOnly() {
		switch o.Container {
		case ContainerMP4:
			fields = append(fields, "ext:mp4:m4a")
		case ContainerWebM:
			fields = append(fields, "ext:webm")
		}
	}

	if len(fields) == 0 {
		return nil
	}
	return []string{"-S", strings.Join(fields, ",")}
}

func containerArgs(o Options) []string {
	if o.wantsPlayable() {
		// MP4 when the streams allow it — what QuickTime, Quick Look and
		// Photos open — and MKV when they do not, since it holds anything.
		// Left to itself yt-dlp prefers WebM and MKV, neither of which
		// QuickTime opens at all.
		return []string{"--merge-output-format", "mp4/mkv"}
	}
	if o.Container == ContainerAuto || o.Pick.IsAudioOnly() {
		return nil
	}
	return []string{"--merge-output-format", string(o.Container)}
}

func subtitleArgs(o Options) []string {
	s := o.Subtitles
	if !s.Download && !s.Embed && !s.AutoGenerated {
		return nil
	}

	var args []string
	if s.Download {
		args = append(args, "--write-subs")
	}
	if s.AutoGenerated {
		args = append(args, "--write-auto-subs")
	}
	if len(s.Languages) > 0 {
		args = append(args, "--sub-langs", strings.Join(s.Languages, ","))
	}
	if s.Embed {
		args = append(args, "--embed-subs")
	}
	return args
}

func enhancementArgs(o Options) []string {
	e := o.Enhancements
	var args []string

	if e.SponsorBlock != SponsorBlockOff {
		categories := e.SponsorBlockCategories
		if len(categories) == 0 {
			categories = DefaultSponsorBlockCategories
		}
		flag := "--sponsorblock-remove"
		if e.SponsorBlock == SponsorBlockMark {
			flag = "--sponsorblock-mark"
		}
		args = append(args, flag, strings.Join(categories, ","))
	}
	if e.EmbedChapters {
		args = append(args, "--embed-chapters")
	}
	if e.EmbedThumbnail {
		args = append(args, "--embed-thumbnail")
	}
	if e.EmbedMetadata {
		args = append(args, "--embed-metadata")
	}
	return args
}

// ChapterTemplate names the files --split-chapters writes.
//
// The number comes first so the tracks sort into playing order in any file
// browser, and it is zero-padded so ten does not sort before two. The format is
// also a contract: ChapterFile.Title parses the chapter name back out of it,
// because yt-dlp reports the path and the number but never the name alone.
const ChapterTemplate = "chapter:%(section_number)02d - %(section_title)s.%(ext)s"

// titleSplitPattern pulls an artist and a title out of "Artist - Title".
//
// It reads %(artist,title)s — the artist when the site gave one, the video
// title when it did not. That is what makes it safe to apply always: a site
// that supplies a real artist yields a string with no " - " in it, the pattern
// does not match, nothing is overwritten, and --embed-metadata goes on using
// the site's own fields. Only a bare video title gets split.
const titleSplitPattern = `%(artist,title)s:(?P<meta_artist>.+?) - (?P<meta_title>.+)`

// yearPattern reduces a full upload date to a year.
//
// Music wants a release year; "20260919" in a date tag is a timestamp nobody
// asked for. release_year is preferred where the site states it.
const yearPattern = `%(release_year,upload_date>%Y)s:%(meta_date)s`

// musicArgs adds what a music download needs beyond a video one.
func musicArgs(o Options) []string {
	var args []string

	if o.Music.Tags {
		args = append(args, "--parse-metadata", titleSplitPattern)
		args = append(args, "--parse-metadata", yearPattern)
		// The tags are written by the metadata post-processor, so asking for
		// them without it would parse fields nothing ever records.
		if !o.Enhancements.EmbedMetadata {
			args = append(args, "--embed-metadata")
		}
	}

	if o.Music.SplitChapters {
		args = append(args, "--split-chapters", "-o", ChapterTemplate)
	}
	return args
}

func playlistArgs(o Options) []string {
	var args []string

	if items := playlistItems(o.Playlist); items != "" {
		args = append(args, "--playlist-items", items)
	}
	if o.Playlist.Reverse {
		args = append(args, "--playlist-reverse")
	}
	return args
}

// playlistItems renders a range in yt-dlp's "start:end" form, leaving either
// side blank when unbounded.
func playlistItems(p Playlist) string {
	switch {
	case p.Start > 0 && p.End > 0:
		return strconv.Itoa(p.Start) + ":" + strconv.Itoa(p.End)
	case p.Start > 0:
		return strconv.Itoa(p.Start) + ":"
	case p.End > 0:
		return ":" + strconv.Itoa(p.End)
	default:
		return ""
	}
}

func networkArgs(o Options) []string {
	var args []string

	if o.Network.RateLimit != "" {
		args = append(args, "--limit-rate", o.Network.RateLimit)
	}
	if spec := cookieSpec(o); spec != "" {
		args = append(args, "--cookies-from-browser", spec)
	}
	return args
}

// cookieSpec renders the --cookies-from-browser value.
//
// yt-dlp has no "arc" browser. Arc is Chromium underneath and keeps a standard
// Chromium profile, so it is requested as chrome with an explicit profile path.
func cookieSpec(o Options) string {
	switch o.Network.Cookies {
	case BrowserNone:
		return ""
	case BrowserArc:
		if o.ArcProfileDir == "" {
			return ""
		}
		return "chrome:" + o.ArcProfileDir
	default:
		return string(o.Network.Cookies)
	}
}

func outputArgs(o Options) []string {
	var args []string

	if o.Output.Folder != "" {
		args = append(args, "-P", o.Output.Folder)
	}
	template := o.Output.Template
	if template == "" {
		template = o.Pick.defaultTemplate()
	}
	return append(args, "-o", template)
}

// ShowCommand renders the command for the "show command" panel, quoting only
// the arguments that need it.
//
// The result is for reading and pasting. Lasso itself never builds a shell
// string: it always executes with an argument slice.
func ShowCommand(ytDlpPath string, o Options) string {
	parts := make([]string, 0, len(BuildArgs(o))+1)
	parts = append(parts, shellQuote(ytDlpPath))
	for _, arg := range BuildArgs(o) {
		parts = append(parts, shellQuote(arg))
	}
	return strings.Join(parts, " ")
}

// shellQuote wraps an argument in single quotes when it contains anything a
// shell would interpret.
func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	if !strings.ContainsAny(s, " \t\n\"'\\$`*?[]{}()<>|&;#~!") {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// CookieProbeArgs asks yt-dlp to open a browser's cookie jar and nothing more.
//
// Whether a cookie jar can be read is not answerable by inspection: it depends
// on macOS's permission state, on whether the browser holds a lock on the file,
// and for Chromium on whether the keychain releases the decryption key. The
// only honest test is the thing that will actually do it.
//
// yt-dlp loads cookies before it extracts anything, so a jar it cannot open
// fails here without a single network request. When the jar opens, the request
// that follows is a simulation — nothing is downloaded and nothing is written.
func CookieProbeArgs(o Options) []string {
	args := []string{
		"--ignore-config",
		"--no-plugin-dirs",
		"--simulate",
		"--no-warnings",
		// One item is enough to reach the point where cookies have been used,
		// and stops a link that turns out to be a playlist walking all of it.
		"--playlist-items", "1",
	}

	if o.DenoPath != "" {
		args = append(args, "--js-runtimes", "deno:"+o.DenoPath)
	}
	if spec := cookieSpec(o); spec != "" {
		args = append(args, "--cookies-from-browser", spec)
	}
	return append(args, "--", strings.TrimSpace(o.URL))
}
