package core

import (
	"sort"
	"strconv"
	"strings"
)

// ResolutionTier is one rung of the quality ladder offered to the user.
type ResolutionTier struct {
	// Height is the nominal short-side resolution, e.g. 1080.
	Height int `json:"height"`
	// Label is what the chip reads, e.g. "4K" or "1080p".
	Label string `json:"label"`
	// Detail is the secondary line for the marketing names, e.g. "2160p".
	Detail string `json:"detail"`
	// HasHighFrameRate is true when some format at this tier exceeds 50fps.
	HasHighFrameRate bool `json:"hasHighFrameRate"`
	// HasHDR is true when some format at this tier is HDR.
	HasHDR bool `json:"hasHDR"`
	// HDRFormat names the kind, e.g. "HDR10" or "DV", for the tooltip.
	HDRFormat string `json:"hdrFormat"`
	// Bytes estimates the finished file at this tier: the best video stream
	// here, plus the best audio when that stream carries none. Zero when the
	// source does not say, which is common for fragmented and live streams.
	Bytes int64 `json:"bytes"`
	// Playable says whether this Mac plays the tier's encode natively — in
	// QuickTime, Quick Look and Photos — rather than needing IINA or VLC.
	Playable bool `json:"playable"`
}

// QualityOptions describes what a resolved link can actually be downloaded as.
type QualityOptions struct {
	// Tiers are the offerable resolutions, largest first. Empty for a source
	// with no video.
	Tiers []ResolutionTier `json:"tiers"`
	// HasVideo and HasAudio say which sections to show at all.
	HasVideo bool `json:"hasVideo"`
	HasAudio bool `json:"hasAudio"`
	// BestHeight is what "Best" downloads: the top resolution this Mac plays
	// natively, or the top resolution outright when none does.
	BestHeight int `json:"bestHeight"`
	// BestPlayable says whether "Best" plays natively, so the interface can
	// say when it would need another player.
	BestPlayable bool `json:"bestPlayable"`
	// BestLabel is the label for that resolution, e.g. "8K".
	BestLabel string `json:"bestLabel"`
	// Approximate is true when the tiers are a generic ladder rather than
	// something measured — a flat playlist has no per-item formats.
	Approximate bool `json:"approximate"`
	// CountedFormats is the number of real downloadable formats, excluding
	// storyboards.
	CountedFormats int `json:"countedFormats"`
	// AudioBytes estimates an audio-only download: the best audio stream.
	AudioBytes int64 `json:"audioBytes"`
	// BestBytes estimates what "Best" would produce.
	BestBytes int64 `json:"bestBytes"`
	// LosslessAudio is true when some audio stream is itself lossless.
	//
	// It is almost never true on the video sites this is used with, and that is
	// the point: asking for FLAC from a source that only serves Opus or AAC
	// produces a genuine FLAC file of audio that has already lost what it lost.
	// The interface can only say so if the backend has looked.
	LosslessAudio bool `json:"losslessAudio"`
	// Limited is true when the format list has the shape a site returns to a
	// client it will not serve properly: one low muxed stream and no separate
	// video track at all. See looksLimited.
	Limited bool `json:"limited"`
}

// limitedCeiling is the tallest a format list can be and still look withheld.
//
// It is YouTube's fallback stream (itag 18, 360p muxed), which is what a
// request without working authentication is left with.
const limitedCeiling = 360

// tierLadder is the set of rungs Lasso offers, largest first.
var tierLadder = []struct {
	height int
	label  string
	detail string
}{
	{4320, "8K", "4320p"},
	{2160, "4K", "2160p"},
	{1440, "1440p", ""},
	{1080, "1080p", ""},
	{720, "720p", ""},
	{480, "480p", ""},
	{360, "360p", ""},
}

// tierTolerance allows an encode that lands just under a rung to still count.
//
// Real files are not always exactly 1080 tall: a 2026x1036 master is a 1080p
// video to everyone except a strict comparison. 1036/1080 is 0.959, so the
// threshold sits just below that. It stays high enough that a genuine 768-tall
// source still lands on 720 rather than being promoted.
const tierTolerance = 0.95

// losslessCodecs are the audio codecs that discard nothing.
var losslessCodecs = []string{"flac", "alac", "pcm", "wav", "ape", "tta", "wv"}

// IsLosslessAudio reports whether a format's audio is itself lossless.
//
// This is what separates "FLAC" the file format from "lossless" the property.
// Encoding an Opus stream to FLAC produces a real FLAC file that is larger than
// the original and sounds exactly like it — nothing is recovered, because
// nothing that was thrown away is still there to recover.
func (f Format) IsLosslessAudio() bool {
	codec := strings.ToLower(f.ACodec)
	if codec == "" || codec == "none" {
		return false
	}
	for _, lossless := range losslessCodecs {
		if strings.HasPrefix(codec, lossless) {
			return true
		}
	}
	return false
}

// IsStoryboard reports whether a format is one of yt-dlp's preview mosaics
// rather than something worth downloading.
func (f Format) IsStoryboard() bool {
	return f.Ext == "mhtml" || containsFold(f.Note, "storyboard")
}

// ShortSide is the smaller of the two dimensions.
//
// Classifying on the short side is what makes a vertical video correct: a
// 1080x1920 Short is 1080p, not 1920p.
func (f Format) ShortSide() int {
	if f.Height <= 0 {
		return 0
	}
	if f.Width > 0 && f.Width < f.Height {
		return f.Width
	}
	return f.Height
}

// IsHDR reports whether a format carries high dynamic range.
//
// DynamicRange is the field that actually answers this. yt-dlp fills it for
// every format it knows about — "SDR" when there is nothing special — and it is
// the only place the answer is stated plainly: a format note may say "2160p60"
// and nothing more, which is why HDR never used to surface.
//
// The note and codec are still consulted, for extractors that fill neither the
// field nor it correctly.
func (f Format) IsHDR() bool {
	switch strings.ToUpper(strings.TrimSpace(f.DynamicRange)) {
	case "", "SDR":
		// Fall through to the older hints rather than concluding SDR.
	default:
		return true
	}
	return containsFold(f.Note, "hdr") || containsFold(f.VCodec, "dvh") ||
		containsFold(f.Note, "dolby vision")
}

// HDRName is the dynamic range to show, e.g. "HDR10" or "DV".
func (f Format) HDRName() string {
	if r := strings.ToUpper(strings.TrimSpace(f.DynamicRange)); r != "" && r != "SDR" {
		return r
	}
	if f.IsHDR() {
		return "HDR"
	}
	return ""
}

// tierState accumulates what the formats at one rung have in common.
type tierState struct {
	highFrameRate bool
	hdr           bool
	hdrFormat     string
	// bestVideo is the largest video stream at this rung, which is the one
	// yt-dlp's default ordering picks. muxed says whether it already
	// carries audio, and so whether an audio stream has to be added.
	bestVideo int64
	muxed     bool
	// The same for the encodes this Mac plays, which the download
	// prefers at a rung when there are any — see formatArgs — and which
	// the size estimate therefore has to describe.
	plays         bool
	bestPlayable  int64
	playableMuxed bool
}

// AnalyseFormats works out which quality options a resolved link supports.
//
// Only formats that carry video and a usable height contribute a tier, so
// storyboards, audio streams and sizeless entries cannot invent a resolution
// the video does not have.
func AnalyseFormats(formats []Format, playback Playback) QualityOptions {
	options := QualityOptions{}

	seen := map[int]*tierState{}

	// The best audio stream, added to any tier whose video has none.
	var bestAudio int64

	// A site serving a link properly offers separate video and audio streams to
	// combine. Their total absence is the signal that something was withheld.
	adaptive := false

	// tallest is the real top resolution, whether or not it plays here.
	tallest := 0

	for _, f := range formats {
		if f.IsStoryboard() {
			continue
		}
		options.CountedFormats++

		if f.HasAudio() {
			options.HasAudio = true
			if f.IsLosslessAudio() {
				options.LosslessAudio = true
			}
			if !f.HasVideo() && f.Size() > bestAudio {
				bestAudio = f.Size()
			}
		}
		if !f.HasVideo() {
			continue
		}

		short := f.ShortSide()
		if short <= 0 {
			// A video format with no dimensions tells us nothing about which
			// rung it belongs to, so it must not create one.
			continue
		}
		options.HasVideo = true
		if !f.HasAudio() {
			adaptive = true
		}
		if short > tallest {
			tallest = short
		}

		height, ok := tierFor(short)
		if !ok {
			continue
		}
		state, exists := seen[height]
		if !exists {
			state = &tierState{}
			seen[height] = state
		}
		if f.FPS > 50 {
			state.highFrameRate = true
		}
		if f.IsHDR() {
			state.hdr = true
			if state.hdrFormat == "" {
				state.hdrFormat = f.HDRName()
			}
		}
		if size := f.Size(); size > state.bestVideo {
			state.bestVideo = size
			state.muxed = f.HasAudio()
		}
		if playback.Plays(f.VCodec) {
			state.plays = true
			if size := f.Size(); size > state.bestPlayable {
				state.bestPlayable = size
				state.playableMuxed = f.HasAudio()
			}
		}
	}

	for _, rung := range tierLadder {
		state, ok := seen[rung.height]
		if !ok {
			continue
		}
		options.Tiers = append(options.Tiers, ResolutionTier{
			Height:           rung.height,
			Label:            rung.label,
			Detail:           rung.detail,
			HasHighFrameRate: state.highFrameRate,
			HasHDR:           state.hdr,
			HDRFormat:        state.hdrFormat,
			Bytes:            state.estimate(bestAudio),
			Playable:         state.plays,
		})
	}

	sort.SliceStable(options.Tiers, func(i, j int) bool {
		return options.Tiers[i].Height > options.Tiers[j].Height
	})

	options.Limited = looksLimited(options.HasVideo, tallest, adaptive)
	options.AudioBytes = bestAudio

	// "Best" is the top rung this Mac plays, falling back to the top rung
	// outright when none does — the same choice formatArgs makes.
	options.BestHeight = tallest
	options.BestLabel = labelFor(tallest)
	if len(options.Tiers) > 0 {
		best := options.Tiers[0]
		for _, tier := range options.Tiers {
			if tier.Playable {
				best = tier
				break
			}
		}
		options.BestHeight = best.Height
		options.BestLabel = best.Label
		options.BestBytes = best.Bytes
		options.BestPlayable = best.Playable
	}
	return options
}

// estimate sizes what the download will choose at this rung: an encode this
// Mac plays when there is one, otherwise the largest.
func (t *tierState) estimate(audio int64) int64 {
	if t.plays {
		return estimate(t.bestPlayable, t.playableMuxed, audio)
	}
	return estimate(t.bestVideo, t.muxed, audio)
}

// estimate adds the audio stream to a video stream that has none.
//
// Zero in means zero out: a size nobody stated is not worth guessing at, and a
// number that is quietly wrong is worse than no number at all.
func estimate(video int64, muxed bool, audio int64) int64 {
	if video <= 0 {
		return 0
	}
	if muxed {
		return video
	}
	return video + audio
}

// looksLimited reports whether a format list looks withheld rather than simply
// small.
//
// The signature is a video whose best is no better than the fallback stream,
// offered only as a single muxed file. A site that genuinely has nothing better
// still lists its low resolutions as separate video and audio tracks, so the
// absence of those is what separates "this is all there is" from "this is all
// you are being given". The usual cause is a request the site would not
// authenticate.
func looksLimited(hasVideo bool, bestHeight int, adaptive bool) bool {
	return hasVideo && !adaptive && bestHeight > 0 && bestHeight <= limitedCeiling
}

// GenericQualityOptions is the ladder offered when the real formats are not
// known — a flat playlist has not visited its videos, so the UI offers the
// standard rungs and yt-dlp falls back per item.
func GenericQualityOptions() QualityOptions {
	options := QualityOptions{HasVideo: true, HasAudio: true, Approximate: true}
	for _, rung := range tierLadder {
		options.Tiers = append(options.Tiers, ResolutionTier{
			Height: rung.height,
			Label:  rung.label,
			Detail: rung.detail,
		})
	}
	return options
}

// tierFor maps a measured short side onto a rung, allowing an encode that lands
// slightly under it to still qualify.
func tierFor(short int) (int, bool) {
	for _, rung := range tierLadder {
		if float64(short) >= float64(rung.height)*tierTolerance {
			return rung.height, true
		}
	}
	return 0, false
}

// labelFor names a measured resolution.
//
// Sources below the lowest rung are named by their actual size rather than
// left blank: a 240p upload has no tier to offer, but "Best · 240p" is still
// the truth and is more useful than saying nothing.
func labelFor(short int) string {
	if short <= 0 {
		return ""
	}
	height, ok := tierFor(short)
	if !ok {
		return strconv.Itoa(short) + "p"
	}
	for _, rung := range tierLadder {
		if rung.height == height {
			return rung.label
		}
	}
	return strconv.Itoa(short) + "p"
}

// containsFold is a case-insensitive substring test.
func containsFold(haystack, needle string) bool {
	return strings.Contains(strings.ToLower(haystack), strings.ToLower(needle))
}
