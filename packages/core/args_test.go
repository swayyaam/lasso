package core

import (
	"slices"
	"strings"
	"testing"
)

// baseOptions is a minimal valid request. Tests vary one thing at a time from
// here so a failure names exactly which option broke.
func baseOptions() Options {
	return Options{URL: "https://example.com/watch?v=abc", Pick: PickBest}
}

// argValue returns the value following flag, and whether flag was present.
func argValue(args []string, flag string) (string, bool) {
	for i, a := range args {
		if a == flag && i+1 < len(args) {
			return args[i+1], true
		}
	}
	return "", false
}

func hasFlag(args []string, flag string) bool {
	return slices.Contains(args, flag)
}

func TestBuildArgsIgnoresUserConfigFirst(t *testing.T) {
	args := BuildArgs(baseOptions())
	// Must come first, or a user's own config file can still take effect.
	if args[0] != "--ignore-config" {
		t.Errorf("args[0] = %q, want --ignore-config", args[0])
	}
}

func TestBuildArgsPutsURLLastBehindSeparator(t *testing.T) {
	o := baseOptions()
	o.URL = "https://example.com/-weird-looking-url"

	args := BuildArgs(o)
	if got := args[len(args)-1]; got != o.URL {
		t.Errorf("last arg = %q, want the URL", got)
	}
	// The separator keeps a URL that starts with a dash from being read as a flag.
	if got := args[len(args)-2]; got != "--" {
		t.Errorf("second-to-last arg = %q, want --", got)
	}
}

func TestBuildArgsIsPure(t *testing.T) {
	o := baseOptions()
	o.Subtitles = Subtitles{Download: true, Languages: []string{"en", "de"}}

	first := BuildArgs(o)
	second := BuildArgs(o)
	if !slices.Equal(first, second) {
		t.Errorf("BuildArgs is not deterministic:\n %v\n %v", first, second)
	}
	// Building must not mutate the caller's slices.
	if !slices.Equal(o.Subtitles.Languages, []string{"en", "de"}) {
		t.Errorf("BuildArgs mutated Options: %v", o.Subtitles.Languages)
	}
}

func TestQuickPickFormatSelectors(t *testing.T) {
	cases := []struct {
		pick QuickPick
		want string
	}{
		{PickBest, "bv*+ba/b"},
		{Pick2160p, "bv*[height<=2160]+ba/b[height<=2160]/bv*+ba/b"},
		{Pick1440p, "bv*[height<=1440]+ba/b[height<=1440]/bv*+ba/b"},
		{Pick1080p, "bv*[height<=1080]+ba/b[height<=1080]/bv*+ba/b"},
		{Pick720p, "bv*[height<=720]+ba/b[height<=720]/bv*+ba/b"},
		{PickAudioMP3, "ba/b"},
		{PickAudioFLAC, "ba/b"},
		{PickAudioM4A, "ba[ext=m4a]/ba/b"},
		{PickAudioOpus, "ba[acodec=opus]/ba[ext=webm]/ba/b"},
	}

	for _, tc := range cases {
		t.Run(string(tc.pick), func(t *testing.T) {
			o := baseOptions()
			o.Pick = tc.pick

			got, ok := argValue(BuildArgs(o), "-f")
			if !ok {
				t.Fatal("no -f selector emitted")
			}
			if got != tc.want {
				t.Errorf("-f = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCappedPicksFallBackToBest(t *testing.T) {
	// A 1080p request against a 720p-max video must still download something.
	for _, pick := range []QuickPick{Pick2160p, Pick1440p, Pick1080p, Pick720p} {
		o := baseOptions()
		o.Pick = pick

		selector, _ := argValue(BuildArgs(o), "-f")
		if !strings.HasSuffix(selector, "/bv*+ba/b") {
			t.Errorf("%s selector %q has no unconstrained fallback", pick, selector)
		}
	}
}

func TestAudioPicksExtractAndConvert(t *testing.T) {
	cases := []struct {
		pick        QuickPick
		format      string
		wantQuality bool
	}{
		{PickAudioMP3, "mp3", true},
		{PickAudioM4A, "m4a", true},
		{PickAudioOpus, "opus", true},
		// FLAC is lossless; --audio-quality would be a knob that does nothing.
		{PickAudioFLAC, "flac", false},
	}

	for _, tc := range cases {
		t.Run(string(tc.pick), func(t *testing.T) {
			o := baseOptions()
			o.Pick = tc.pick
			args := BuildArgs(o)

			if !hasFlag(args, "-x") {
				t.Error("missing -x")
			}
			if got, _ := argValue(args, "--audio-format"); got != tc.format {
				t.Errorf("--audio-format = %q, want %q", got, tc.format)
			}
			if _, ok := argValue(args, "--audio-quality"); ok != tc.wantQuality {
				t.Errorf("--audio-quality present = %v, want %v", ok, tc.wantQuality)
			}
		})
	}
}

func TestAudioPicksSkipVideoOnlyOptions(t *testing.T) {
	o := baseOptions()
	o.Pick = PickAudioFLAC
	o.Container = ContainerMKV
	o.VideoCodec = VideoCodecAV1

	args := BuildArgs(o)
	if hasFlag(args, "--merge-output-format") {
		t.Error("audio-only download asked for a video container")
	}
	if sort, ok := argValue(args, "-S"); ok && strings.Contains(sort, "vcodec") {
		t.Errorf("audio-only download sorted on vcodec: %q", sort)
	}
}

func TestContainerSelection(t *testing.T) {
	cases := []struct {
		container Container
		wantMerge string
		wantSort  string
	}{
		{ContainerAuto, "", ""},
		{ContainerMP4, "mp4", "ext:mp4:m4a"},
		{ContainerWebM, "webm", "ext:webm"},
		// MKV holds anything, so it needs no stream-selection hint.
		{ContainerMKV, "mkv", ""},
	}

	for _, tc := range cases {
		name := string(tc.container)
		if name == "" {
			name = "auto"
		}
		t.Run(name, func(t *testing.T) {
			o := baseOptions()
			o.Container = tc.container
			args := BuildArgs(o)

			got, ok := argValue(args, "--merge-output-format")
			if tc.wantMerge == "" && ok {
				t.Errorf("--merge-output-format = %q, want none", got)
			}
			if tc.wantMerge != "" && got != tc.wantMerge {
				t.Errorf("--merge-output-format = %q, want %q", got, tc.wantMerge)
			}

			sort, _ := argValue(args, "-S")
			if tc.wantSort == "" && strings.Contains(sort, "ext:") {
				t.Errorf("-S = %q, want no ext hint", sort)
			}
			if tc.wantSort != "" && !strings.Contains(sort, tc.wantSort) {
				t.Errorf("-S = %q, want it to contain %q", sort, tc.wantSort)
			}
		})
	}
}

func TestCodecPreferences(t *testing.T) {
	o := baseOptions()
	o.VideoCodec = VideoCodecAV1
	o.AudioCodec = AudioCodecOpus
	o.Container = ContainerMP4

	sort, ok := argValue(BuildArgs(o), "-S")
	if !ok {
		t.Fatal("no -S emitted for codec preferences")
	}
	// Codec preferences outrank the container hint.
	want := "vcodec:av01,acodec:opus,ext:mp4:m4a"
	if sort != want {
		t.Errorf("-S = %q, want %q", sort, want)
	}
}

func TestNoSortWithoutPreferences(t *testing.T) {
	// yt-dlp's default ordering is good; only override it when asked.
	if _, ok := argValue(BuildArgs(baseOptions()), "-S"); ok {
		t.Error("-S emitted with no codec or container preference")
	}
}

func TestSubtitleOptions(t *testing.T) {
	cases := []struct {
		name  string
		subs  Subtitles
		want  []string
		avoid []string
	}{
		{
			name:  "off",
			subs:  Subtitles{},
			avoid: []string{"--write-subs", "--embed-subs", "--write-auto-subs", "--sub-langs"},
		},
		{
			name: "download only",
			subs: Subtitles{Download: true},
			want: []string{"--write-subs"},
		},
		{
			name: "embed with languages",
			subs: Subtitles{Download: true, Embed: true, Languages: []string{"en", "ja"}},
			want: []string{"--write-subs", "--embed-subs"},
		},
		{
			name: "auto-generated",
			subs: Subtitles{Download: true, AutoGenerated: true},
			want: []string{"--write-subs", "--write-auto-subs"},
		},
		{
			name: "embed without writing",
			subs: Subtitles{Embed: true},
			want: []string{"--embed-subs"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			o := baseOptions()
			o.Subtitles = tc.subs
			args := BuildArgs(o)

			for _, flag := range tc.want {
				if !hasFlag(args, flag) {
					t.Errorf("missing %s", flag)
				}
			}
			for _, flag := range tc.avoid {
				if hasFlag(args, flag) {
					t.Errorf("unexpected %s", flag)
				}
			}
			if len(tc.subs.Languages) > 0 {
				if got, _ := argValue(args, "--sub-langs"); got != strings.Join(tc.subs.Languages, ",") {
					t.Errorf("--sub-langs = %q", got)
				}
			}
		})
	}
}

func TestEnhancementOptions(t *testing.T) {
	o := baseOptions()
	o.Enhancements = Enhancements{
		SponsorBlock:   SponsorBlockRemove,
		EmbedChapters:  true,
		EmbedThumbnail: true,
		EmbedMetadata:  true,
	}
	args := BuildArgs(o)

	for _, flag := range []string{"--embed-chapters", "--embed-thumbnail", "--embed-metadata"} {
		if !hasFlag(args, flag) {
			t.Errorf("missing %s", flag)
		}
	}
	if got, _ := argValue(args, "--sponsorblock-remove"); got != "sponsor" {
		t.Errorf("--sponsorblock-remove = %q, want the default category", got)
	}
}

func TestSponsorBlockModes(t *testing.T) {
	cases := []struct {
		mode       SponsorBlockMode
		categories []string
		wantFlag   string
		wantValue  string
	}{
		{SponsorBlockRemove, nil, "--sponsorblock-remove", "sponsor"},
		{SponsorBlockMark, nil, "--sponsorblock-mark", "sponsor"},
		{SponsorBlockRemove, []string{"sponsor", "intro", "outro"}, "--sponsorblock-remove", "sponsor,intro,outro"},
	}

	for _, tc := range cases {
		o := baseOptions()
		o.Enhancements = Enhancements{SponsorBlock: tc.mode, SponsorBlockCategories: tc.categories}
		args := BuildArgs(o)

		got, ok := argValue(args, tc.wantFlag)
		if !ok {
			t.Fatalf("missing %s", tc.wantFlag)
		}
		if got != tc.wantValue {
			t.Errorf("%s = %q, want %q", tc.wantFlag, got, tc.wantValue)
		}
	}

	off := BuildArgs(baseOptions())
	if hasFlag(off, "--sponsorblock-remove") || hasFlag(off, "--sponsorblock-mark") {
		t.Error("SponsorBlock flags emitted while it is off")
	}
}

func TestPlaylistRanges(t *testing.T) {
	cases := []struct {
		name string
		p    Playlist
		want string
	}{
		{"unset", Playlist{}, ""},
		{"both ends", Playlist{Start: 1, End: 5}, "1:5"},
		{"open ended", Playlist{Start: 3}, "3:"},
		{"open start", Playlist{End: 7}, ":7"},
		{"single item", Playlist{Start: 4, End: 4}, "4:4"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			o := baseOptions()
			o.Playlist = tc.p
			args := BuildArgs(o)

			got, ok := argValue(args, "--playlist-items")
			if tc.want == "" {
				if ok {
					t.Errorf("--playlist-items = %q, want none", got)
				}
				return
			}
			if got != tc.want {
				t.Errorf("--playlist-items = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestPlaylistReverse(t *testing.T) {
	o := baseOptions()
	o.Playlist = Playlist{Reverse: true}
	if !hasFlag(BuildArgs(o), "--playlist-reverse") {
		t.Error("missing --playlist-reverse")
	}
	if hasFlag(BuildArgs(baseOptions()), "--playlist-reverse") {
		t.Error("--playlist-reverse emitted when not asked for")
	}
}

func TestNetworkOptions(t *testing.T) {
	o := baseOptions()
	o.Network = Network{RateLimit: "2M", Cookies: BrowserFirefox}
	args := BuildArgs(o)

	if got, _ := argValue(args, "--limit-rate"); got != "2M" {
		t.Errorf("--limit-rate = %q", got)
	}
	if got, _ := argValue(args, "--cookies-from-browser"); got != "firefox" {
		t.Errorf("--cookies-from-browser = %q", got)
	}

	bare := BuildArgs(baseOptions())
	if hasFlag(bare, "--limit-rate") || hasFlag(bare, "--cookies-from-browser") {
		t.Error("network flags emitted with no network options set")
	}
}

func TestArcCookiesUseChromiumProfilePath(t *testing.T) {
	// yt-dlp has no "arc" browser; Arc is Chromium with its own profile folder.
	o := baseOptions()
	o.Network = Network{Cookies: BrowserArc}
	o.ArcProfileDir = "/Users/x/Library/Application Support/Arc/User Data"

	got, ok := argValue(BuildArgs(o), "--cookies-from-browser")
	if !ok {
		t.Fatal("missing --cookies-from-browser")
	}
	if want := "chrome:/Users/x/Library/Application Support/Arc/User Data"; got != want {
		t.Errorf("--cookies-from-browser = %q, want %q", got, want)
	}
}

func TestOutputOptions(t *testing.T) {
	o := baseOptions()
	o.Output = Output{Folder: "/Users/x/Movies", Template: "%(uploader)s/%(title)s.%(ext)s"}
	args := BuildArgs(o)

	if got, _ := argValue(args, "-P"); got != "/Users/x/Movies" {
		t.Errorf("-P = %q", got)
	}
	if got, _ := argValue(args, "-o"); got != "%(uploader)s/%(title)s.%(ext)s" {
		t.Errorf("-o = %q", got)
	}
}

func TestOutputTemplateDefaults(t *testing.T) {
	got, ok := argValue(BuildArgs(baseOptions()), "-o")
	if !ok {
		t.Fatal("no -o emitted")
	}
	if got != DefaultTemplate {
		t.Errorf("-o = %q, want the default template", got)
	}
}

func TestFFmpegLocationIsPassedExplicitly(t *testing.T) {
	o := baseOptions()
	o.FFmpegLocation = "/Users/x/Library/Application Support/Lasso/bin"

	got, ok := argValue(BuildArgs(o), "--ffmpeg-location")
	if !ok {
		t.Fatal("missing --ffmpeg-location: yt-dlp would fall back to the system PATH")
	}
	if got != o.FFmpegLocation {
		t.Errorf("--ffmpeg-location = %q", got)
	}
}

func TestExecArgsAddProgressReportingOnly(t *testing.T) {
	o := baseOptions()
	shown := BuildArgs(o)
	executed := ExecArgs(o)

	for _, flag := range []string{"--newline", "--no-colors", "--progress-template"} {
		if hasFlag(shown, flag) {
			t.Errorf("%s leaked into the shown command", flag)
		}
		if !hasFlag(executed, flag) {
			t.Errorf("%s missing from the executed command", flag)
		}
	}

	// Everything the shown command contains must also be executed, in order:
	// the panel must never describe a download different from the real one.
	if !isSubsequence(shown, executed) {
		t.Errorf("shown command is not a subsequence of what runs:\n shown: %v\n exec:  %v", shown, executed)
	}
	if executed[len(executed)-1] != o.URL {
		t.Error("ExecArgs did not keep the URL last")
	}
	if executed[len(executed)-2] != "--" {
		t.Error("ExecArgs did not keep the -- separator before the URL")
	}
}

func isSubsequence(want, got []string) bool {
	i := 0
	for _, g := range got {
		if i < len(want) && g == want[i] {
			i++
		}
	}
	return i == len(want)
}

func TestExecArgsProgressTemplateIsValidJSONShape(t *testing.T) {
	template, ok := argValue(ExecArgs(baseOptions()), "--progress-template")
	if !ok {
		t.Fatal("no progress template")
	}
	if !strings.HasPrefix(template, "download:{") {
		t.Errorf("template %q does not target the download stage", template)
	}
	// Every field needs a default, or yt-dlp emits a bare NA and breaks the JSON.
	for _, field := range []string{"downloaded_bytes", "total_bytes", "speed", "eta"} {
		if !strings.Contains(template, field+"|0") {
			t.Errorf("field %s has no |0 default; an unavailable value would emit NA", field)
		}
	}
}

func TestShowCommandQuotesOnlyWhatNeedsIt(t *testing.T) {
	o := baseOptions()
	o.Output = Output{Folder: "/Users/x/My Movies"}

	cmd := ShowCommand("/Users/x/Library/Application Support/Lasso/bin/yt-dlp/yt-dlp_macos", o)

	if !strings.Contains(cmd, "'/Users/x/My Movies'") {
		t.Errorf("a path with a space was not quoted:\n%s", cmd)
	}
	if !strings.Contains(cmd, "--ignore-config") {
		t.Errorf("plain flags should not be quoted:\n%s", cmd)
	}
	if strings.Contains(cmd, "--progress-template") {
		t.Errorf("progress flags leaked into the shown command:\n%s", cmd)
	}
}

func TestShellQuote(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", "''"},
		{"plain", "plain"},
		{"--ignore-config", "--ignore-config"},
		{"has space", "'has space'"},
		{"bv*+ba/b", "'bv*+ba/b'"},
		{"it's", `'it'\''s'`},
		{"$(rm -rf /)", `'$(rm -rf /)'`},
	}
	for _, tc := range cases {
		if got := shellQuote(tc.in); got != tc.want {
			t.Errorf("shellQuote(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestArcWithoutProfileEmitsNoCookieFlag(t *testing.T) {
	// Validate rejects this combination, but the builder must not emit a
	// half-formed "chrome:" spec if it is ever reached another way.
	o := baseOptions()
	o.Network = Network{Cookies: BrowserArc}
	o.ArcProfileDir = ""

	if got, ok := argValue(BuildArgs(o), "--cookies-from-browser"); ok {
		t.Errorf("--cookies-from-browser = %q, want none without a profile path", got)
	}
}

func TestCappedPicksSortByResolutionFirst(t *testing.T) {
	// Resolution must outrank codec preference: asking for 1080p means 1080p
	// in whatever codec, not the preferred codec at some other size.
	o := baseOptions()
	o.Pick = Pick1080p
	o.VideoCodec = VideoCodecAV1

	sort, ok := argValue(BuildArgs(o), "-S")
	if !ok {
		t.Fatal("no -S emitted for a capped pick")
	}
	if want := "res:1080,vcodec:av01"; sort != want {
		t.Errorf("-S = %q, want %q", sort, want)
	}
}

func TestBestPickHasNoResolutionSort(t *testing.T) {
	// "Best" means whatever the source has; pinning a resolution would cap it.
	o := baseOptions()
	o.Pick = PickBest

	if sort, ok := argValue(BuildArgs(o), "-S"); ok && strings.Contains(sort, "res:") {
		t.Errorf("-S = %q, want no resolution pin for Best", sort)
	}
}

func TestEveryCappedPickPinsItsResolution(t *testing.T) {
	for pick, want := range map[QuickPick]string{
		Pick2160p: "res:2160",
		Pick1440p: "res:1440",
		Pick1080p: "res:1080",
		Pick720p:  "res:720",
	} {
		o := baseOptions()
		o.Pick = pick

		sort, _ := argValue(BuildArgs(o), "-S")
		if !strings.Contains(sort, want) {
			t.Errorf("%s: -S = %q, want it to contain %q", pick, sort, want)
		}
	}
}
