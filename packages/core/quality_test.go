package core

import (
	"slices"
	"strings"
	"testing"
)

// videoFormat builds a video-bearing format.
func videoFormat(id string, w, h int, fps float64, note string) Format {
	return Format{ID: id, Ext: "mp4", Width: w, Height: h, FPS: fps, VCodec: "avc1", ACodec: "none", Note: note}
}

func audioFormat(id string) Format {
	return Format{ID: id, Ext: "m4a", VCodec: "none", ACodec: "mp4a"}
}

func storyboard(id string) Format {
	return Format{ID: id, Ext: "mhtml", Width: 160, Height: 90, VCodec: "none", ACodec: "none", Note: "storyboard"}
}

func tierHeights(o QualityOptions) []int {
	out := make([]int, 0, len(o.Tiers))
	for _, t := range o.Tiers {
		out = append(out, t.Height)
	}
	return out
}

func TestAnalyseFormatsBuckets(t *testing.T) {
	cases := []struct {
		name      string
		formats   []Format
		wantTiers []int
		wantBest  int
		wantLabel string
		wantVideo bool
		wantAudio bool
	}{
		{
			name: "8K source offers every rung it reaches",
			formats: []Format{
				videoFormat("1", 7680, 4320, 60, ""),
				videoFormat("2", 3840, 2160, 60, ""),
				videoFormat("3", 1920, 1080, 30, ""),
				audioFormat("a"),
			},
			wantTiers: []int{4320, 2160, 1080},
			wantBest:  4320,
			wantLabel: "8K",
			wantVideo: true,
			wantAudio: true,
		},
		{
			name: "1080p-max source never offers 4K",
			formats: []Format{
				videoFormat("1", 1920, 1080, 30, ""),
				videoFormat("2", 1280, 720, 30, ""),
				videoFormat("3", 640, 360, 30, ""),
				audioFormat("a"),
			},
			wantTiers: []int{1080, 720, 360},
			wantBest:  1080,
			wantLabel: "1080p",
			wantVideo: true,
			wantAudio: true,
		},
		{
			// A vertical Short is 1080p, not 1920p: the short side decides.
			name: "vertical short classifies on the short side",
			formats: []Format{
				videoFormat("1", 1080, 1920, 30, ""),
				videoFormat("2", 720, 1280, 30, ""),
			},
			wantTiers: []int{1080, 720},
			wantBest:  1080,
			wantLabel: "1080p",
			wantVideo: true,
		},
		{
			name:      "audio-only source offers no video tiers",
			formats:   []Format{audioFormat("a"), audioFormat("b")},
			wantTiers: []int{},
			wantBest:  0,
			wantLabel: "",
			wantVideo: false,
			wantAudio: true,
		},
		{
			// 2026x1036 is a 1080p master that no exact comparison would match.
			name: "odd encode sizes still land on a rung",
			formats: []Format{
				videoFormat("1", 2026, 1036, 24, ""),
			},
			wantTiers: []int{1080},
			wantBest:  1036,
			wantLabel: "1080p",
			wantVideo: true,
		},
		{
			name:      "storyboards alone produce nothing",
			formats:   []Format{storyboard("sb0"), storyboard("sb1")},
			wantTiers: []int{},
			wantBest:  0,
			wantLabel: "",
			wantVideo: false,
			wantAudio: false,
		},
		{
			name: "video formats without dimensions do not invent a rung",
			formats: []Format{
				{ID: "x", Ext: "mp4", VCodec: "avc1", ACodec: "none"},
				audioFormat("a"),
			},
			wantTiers: []int{},
			wantBest:  0,
			wantVideo: false,
			wantAudio: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := AnalyseFormats(tc.formats)

			if !slices.Equal(tierHeights(got), tc.wantTiers) {
				t.Errorf("tiers = %v, want %v", tierHeights(got), tc.wantTiers)
			}
			if got.BestHeight != tc.wantBest {
				t.Errorf("BestHeight = %d, want %d", got.BestHeight, tc.wantBest)
			}
			if tc.wantLabel != "" && got.BestLabel != tc.wantLabel {
				t.Errorf("BestLabel = %q, want %q", got.BestLabel, tc.wantLabel)
			}
			if got.HasVideo != tc.wantVideo {
				t.Errorf("HasVideo = %v, want %v", got.HasVideo, tc.wantVideo)
			}
			if got.HasAudio != tc.wantAudio {
				t.Errorf("HasAudio = %v, want %v", got.HasAudio, tc.wantAudio)
			}
		})
	}
}

func TestStoryboardsAreNotCounted(t *testing.T) {
	formats := []Format{
		videoFormat("1", 1920, 1080, 30, ""),
		audioFormat("a"),
		storyboard("sb0"),
		storyboard("sb1"),
		{ID: "sb2", Ext: "jpg", Note: "storyboard", VCodec: "none", ACodec: "none"},
	}

	got := AnalyseFormats(formats)
	if got.CountedFormats != 2 {
		t.Errorf("CountedFormats = %d, want 2 real formats", got.CountedFormats)
	}
}

func TestTierBadges(t *testing.T) {
	formats := []Format{
		videoFormat("1", 3840, 2160, 60, "HDR"),
		videoFormat("2", 1920, 1080, 30, ""),
	}

	got := AnalyseFormats(formats)
	byHeight := map[int]ResolutionTier{}
	for _, tier := range got.Tiers {
		byHeight[tier.Height] = tier
	}

	if !byHeight[2160].HasHighFrameRate {
		t.Error("2160 should be marked as high frame rate")
	}
	if !byHeight[2160].HasHDR {
		t.Error("2160 should be marked as HDR")
	}
	// A badge must not leak onto a rung that has no such format.
	if byHeight[1080].HasHighFrameRate || byHeight[1080].HasHDR {
		t.Errorf("1080 picked up badges it should not have: %+v", byHeight[1080])
	}
}

func TestTierLabels(t *testing.T) {
	got := AnalyseFormats([]Format{
		videoFormat("1", 7680, 4320, 30, ""),
		videoFormat("2", 3840, 2160, 30, ""),
		videoFormat("3", 2560, 1440, 30, ""),
	})

	want := map[int][2]string{
		4320: {"8K", "4320p"},
		2160: {"4K", "2160p"},
		1440: {"1440p", ""},
	}
	for _, tier := range got.Tiers {
		if w, ok := want[tier.Height]; ok {
			if tier.Label != w[0] || tier.Detail != w[1] {
				t.Errorf("%d: label/detail = %q/%q, want %q/%q", tier.Height, tier.Label, tier.Detail, w[0], w[1])
			}
		}
	}
}

func TestTiersAreLargestFirst(t *testing.T) {
	got := AnalyseFormats([]Format{
		videoFormat("1", 640, 360, 30, ""),
		videoFormat("2", 3840, 2160, 30, ""),
		videoFormat("3", 1280, 720, 30, ""),
	})

	heights := tierHeights(got)
	for i := 1; i < len(heights); i++ {
		if heights[i-1] <= heights[i] {
			t.Fatalf("tiers are not descending: %v", heights)
		}
	}
}

func TestGenericQualityOptionsForPlaylists(t *testing.T) {
	// A flat playlist has not visited its videos, so the ladder is offered in
	// full and yt-dlp falls back per item.
	got := GenericQualityOptions()

	if !got.Approximate {
		t.Error("Approximate = false for a playlist ladder")
	}
	if len(got.Tiers) != len(tierLadder) {
		t.Errorf("got %d tiers, want the full ladder of %d", len(got.Tiers), len(tierLadder))
	}
	if !got.HasVideo || !got.HasAudio {
		t.Error("a playlist should offer both video and audio")
	}
	for _, tier := range got.Tiers {
		if tier.HasHDR || tier.HasHighFrameRate {
			t.Errorf("%d claims a badge that was never measured", tier.Height)
		}
	}
}

func TestRealFixtureAnalysis(t *testing.T) {
	m, err := ParseMetadata(readFixture(t, "single.json"))
	if err != nil {
		t.Fatal(err)
	}

	got := AnalyseFormats(m.Formats)
	if !got.HasVideo {
		t.Error("the fixture has video formats")
	}
	if got.CountedFormats > len(m.Formats) {
		t.Error("counted more formats than exist")
	}
	// "Me at the zoo" is a 2005 upload; it must not offer 4K.
	for _, tier := range got.Tiers {
		if tier.Height > 720 {
			t.Errorf("fixture offered %dp, which the source does not have", tier.Height)
		}
	}
}

func TestShortSideAndStoryboardHelpers(t *testing.T) {
	if got := videoFormat("1", 1080, 1920, 30, "").ShortSide(); got != 1080 {
		t.Errorf("ShortSide = %d, want the smaller dimension", got)
	}
	if got := videoFormat("1", 1920, 1080, 30, "").ShortSide(); got != 1080 {
		t.Errorf("ShortSide = %d", got)
	}
	if got := (Format{}).ShortSide(); got != 0 {
		t.Errorf("ShortSide = %d for an empty format", got)
	}
	if !storyboard("sb").IsStoryboard() {
		t.Error("mhtml should be a storyboard")
	}
	if (Format{Ext: "jpg", Note: "Storyboard"}).IsStoryboard() != true {
		t.Error("a storyboard note should be recognised regardless of case")
	}
	if videoFormat("1", 1920, 1080, 30, "").IsStoryboard() {
		t.Error("a normal video was treated as a storyboard")
	}
}

// TestTierToleranceBoundaries pins the rounding behaviour, since the tolerance
// is the one number here that is a judgement call rather than a fact.
func TestTierToleranceBoundaries(t *testing.T) {
	cases := []struct {
		short int
		want  int
	}{
		{4320, 4320},
		{2160, 2160},
		{2052, 2160}, // a slightly-under 4K master
		{1440, 1440},
		{1080, 1080},
		{1036, 1080}, // the 2026x1036 case
		{1026, 1080},
		{1024, 720}, // below tolerance, drops a rung
		{768, 720},  // a genuine 1024x768 is not promoted to 1080
		{720, 720},
		{480, 480},
		{360, 360},
		{240, 0}, // below the lowest rung
		{0, 0},
	}

	for _, tc := range cases {
		got, ok := tierFor(tc.short)
		if tc.want == 0 {
			if ok {
				t.Errorf("tierFor(%d) = %d, want no rung", tc.short, got)
			}
			continue
		}
		if !ok || got != tc.want {
			t.Errorf("tierFor(%d) = %d (ok=%v), want %d", tc.short, got, ok, tc.want)
		}
	}
}

// TestSourcesBelowTheLadderAreStillNamed covers old uploads: "Me at the zoo"
// is a real 240p video, which offers no tier but must not be left unlabelled.
func TestSourcesBelowTheLadderAreStillNamed(t *testing.T) {
	got := AnalyseFormats([]Format{videoFormat("1", 320, 240, 30, "")})

	if len(got.Tiers) != 0 {
		t.Errorf("tiers = %v, want none below the lowest rung", tierHeights(got))
	}
	if !got.HasVideo {
		t.Error("HasVideo = false for a video that exists")
	}
	if got.BestHeight != 240 {
		t.Errorf("BestHeight = %d, want 240", got.BestHeight)
	}
	if got.BestLabel != "240p" {
		t.Errorf("BestLabel = %q, want the real resolution", got.BestLabel)
	}
}

// muxedFormat carries video and audio in one stream, the way YouTube's
// fallback (itag 18) does.
func muxedFormat(id string, w, h int) Format {
	return Format{ID: id, Ext: "mp4", Width: w, Height: h, FPS: 30, VCodec: "avc1", ACodec: "mp4a"}
}

func TestDetectsAWithheldFormatList(t *testing.T) {
	cases := []struct {
		name    string
		formats []Format
		want    bool
	}{
		{
			// The signature: YouTube serving a request it would not authenticate
			// is left offering itag 18 and nothing else. This is what makes a 4K
			// video look like a 360p one.
			name:    "only the muxed fallback stream",
			formats: []Format{muxedFormat("18", 640, 360)},
			want:    true,
		},
		{
			name: "a 4K video served properly",
			formats: []Format{
				videoFormat("401", 3840, 2160, 30, ""),
				videoFormat("137", 1920, 1080, 30, ""),
				audioFormat("140"),
				muxedFormat("18", 640, 360),
			},
			want: false,
		},
		{
			// A genuinely small upload still lists separate tracks, so it is not
			// being withheld — it simply has nothing better.
			name: "a low-resolution upload with adaptive streams",
			formats: []Format{
				videoFormat("160", 426, 240, 30, ""),
				audioFormat("140"),
			},
			want: false,
		},
		{
			// A direct file is one muxed stream by nature. Above the fallback
			// ceiling there is no reason to suspect anything was held back.
			name:    "a single muxed stream above the ceiling",
			formats: []Format{muxedFormat("0", 1920, 1080)},
			want:    false,
		},
		{
			name:    "an audio-only source",
			formats: []Format{audioFormat("140")},
			want:    false,
		},
		{
			name:    "nothing at all",
			formats: nil,
			want:    false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := AnalyseFormats(c.formats).Limited; got != c.want {
				t.Errorf("Limited = %v, want %v", got, c.want)
			}
		})
	}
}

func TestWithheldListStillOffersWhatItHas(t *testing.T) {
	// Flagging the list must not empty it: 360p is still downloadable, and a
	// user who wants it anyway should not be blocked.
	got := AnalyseFormats([]Format{muxedFormat("18", 640, 360)})

	if !got.Limited {
		t.Fatal("expected the list to be flagged")
	}
	if !slices.Contains(tierHeights(got), 360) {
		t.Errorf("tiers = %v, want 360p to remain offerable", tierHeights(got))
	}
}

// hdrFormat builds a video format carrying a dynamic range, the way yt-dlp
// reports one.
func hdrFormat(id string, w, h int, fps float64, dynamicRange string) Format {
	f := videoFormat(id, w, h, fps, "")
	f.DynamicRange = dynamicRange
	return f
}

func TestHDRComesFromTheDynamicRangeField(t *testing.T) {
	// The note usually says "2160p60" and nothing more, which is why HDR never
	// used to surface. dynamic_range is where yt-dlp actually states it.
	cases := []struct {
		dynamicRange string
		want         bool
		wantName     string
	}{
		{"HDR10", true, "HDR10"},
		{"HDR10+", true, "HDR10+"},
		{"HLG", true, "HLG"},
		{"DV", true, "DV"},
		{"SDR", false, ""},
		{"", false, ""},
		{"sdr", false, ""},
	}

	for _, c := range cases {
		t.Run(c.dynamicRange, func(t *testing.T) {
			f := hdrFormat("1", 3840, 2160, 60, c.dynamicRange)
			if got := f.IsHDR(); got != c.want {
				t.Errorf("IsHDR() = %v, want %v", got, c.want)
			}
			if got := f.HDRName(); got != c.wantName {
				t.Errorf("HDRName() = %q, want %q", got, c.wantName)
			}
		})
	}
}

func TestHDRSurfacesOnTheTier(t *testing.T) {
	got := AnalyseFormats([]Format{
		hdrFormat("401", 3840, 2160, 60, "HDR10"),
		videoFormat("137", 1920, 1080, 30, ""),
		audioFormat("140"),
	})

	var found bool
	for _, tier := range got.Tiers {
		if tier.Height != 2160 {
			continue
		}
		found = true
		if !tier.HasHDR {
			t.Error("the 4K tier does not report HDR")
		}
		if tier.HDRFormat != "HDR10" {
			t.Errorf("HDRFormat = %q, want the kind named", tier.HDRFormat)
		}
		if !tier.HasHighFrameRate {
			t.Error("60fps did not surface alongside it")
		}
	}
	if !found {
		t.Fatalf("no 4K tier in %v", tierHeights(got))
	}

	// And the SDR rung must not inherit it.
	for _, tier := range got.Tiers {
		if tier.Height == 1080 && tier.HasHDR {
			t.Error("an SDR tier was marked HDR")
		}
	}
}

func TestTierSizeAddsAudioToAVideoOnlyStream(t *testing.T) {
	video := videoFormat("137", 1920, 1080, 30, "")
	video.Filesize = 100_000_000
	audio := audioFormat("140")
	audio.Filesize = 5_000_000

	got := AnalyseFormats([]Format{video, audio})
	for _, tier := range got.Tiers {
		if tier.Height == 1080 && tier.Bytes != 105_000_000 {
			t.Errorf("Bytes = %d, want video plus audio", tier.Bytes)
		}
	}
	if got.AudioBytes != 5_000_000 {
		t.Errorf("AudioBytes = %d, want the best audio stream", got.AudioBytes)
	}
	if got.BestBytes != 105_000_000 {
		t.Errorf("BestBytes = %d, want what the top rung costs", got.BestBytes)
	}
}

func TestMuxedTierSizeDoesNotDoubleCountAudio(t *testing.T) {
	muxed := muxedFormat("18", 640, 360)
	muxed.Filesize = 20_000_000
	audio := audioFormat("140")
	audio.Filesize = 5_000_000

	got := AnalyseFormats([]Format{muxed, audio})
	for _, tier := range got.Tiers {
		if tier.Height == 360 && tier.Bytes != 20_000_000 {
			t.Errorf("Bytes = %d, want the muxed stream's own size", tier.Bytes)
		}
	}
}

func TestUnknownSizeStaysUnknown(t *testing.T) {
	// Fragmented and live streams state no size. A number that is quietly
	// wrong is worse than no number.
	got := AnalyseFormats([]Format{videoFormat("1", 1920, 1080, 30, ""), audioFormat("140")})
	for _, tier := range got.Tiers {
		if tier.Bytes != 0 {
			t.Errorf("Bytes = %d, want 0 when nothing stated a size", tier.Bytes)
		}
	}
}

func TestLosslessAudioIsAboutTheCodecNotTheTarget(t *testing.T) {
	// The distinction the FLAC preset lives or dies on: a real FLAC file made
	// from an Opus stream is not lossless audio, and the interface can only be
	// honest about that if this is right.
	lossy := audioFormat("140")
	lossy.ACodec = "opus"
	if lossy.IsLosslessAudio() {
		t.Error("opus reported as lossless")
	}
	if AnalyseFormats([]Format{lossy}).LosslessAudio {
		t.Error("a source serving only opus was reported as lossless")
	}

	lossless := audioFormat("bandcamp-flac")
	lossless.ACodec = "flac"
	if !lossless.IsLosslessAudio() {
		t.Error("flac not reported as lossless")
	}
	if !AnalyseFormats([]Format{lossless}).LosslessAudio {
		t.Error("a source serving flac was not reported as lossless")
	}

	for _, codec := range []string{"alac", "pcm_s16le", "wav"} {
		f := audioFormat("x")
		f.ACodec = codec
		if !f.IsLosslessAudio() {
			t.Errorf("%q not reported as lossless", codec)
		}
	}
}

func TestAudioPicksAskForTheBestBitrate(t *testing.T) {
	// An audio-only download is the whole file, so the best bitrate the site
	// offers is always the right source.
	o := baseOptions()
	o.Pick = PickAudioFLAC

	sort, ok := argValue(BuildArgs(o), "-S")
	if !ok || !slices.Contains(strings.Split(sort, ","), "abr") {
		t.Errorf("sort = %q, want it to order by audio bitrate", sort)
	}
}
