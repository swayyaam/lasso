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
}

// QualityOptions describes what a resolved link can actually be downloaded as.
type QualityOptions struct {
	// Tiers are the offerable resolutions, largest first. Empty for a source
	// with no video.
	Tiers []ResolutionTier `json:"tiers"`
	// HasVideo and HasAudio say which sections to show at all.
	HasVideo bool `json:"hasVideo"`
	HasAudio bool `json:"hasAudio"`
	// BestHeight is the real top resolution, so "Best" can name it.
	BestHeight int `json:"bestHeight"`
	// BestLabel is the label for that resolution, e.g. "8K".
	BestLabel string `json:"bestLabel"`
	// Approximate is true when the tiers are a generic ladder rather than
	// something measured — a flat playlist has no per-item formats.
	Approximate bool `json:"approximate"`
	// CountedFormats is the number of real downloadable formats, excluding
	// storyboards.
	CountedFormats int `json:"countedFormats"`
}

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

// isHDR reads yt-dlp's dynamic-range hints out of the format note.
func (f Format) isHDR() bool {
	return containsFold(f.Note, "hdr") || containsFold(f.VCodec, "hdr")
}

// AnalyseFormats works out which quality options a resolved link supports.
//
// Only formats that carry video and a usable height contribute a tier, so
// storyboards, audio streams and sizeless entries cannot invent a resolution
// the video does not have.
func AnalyseFormats(formats []Format) QualityOptions {
	options := QualityOptions{}

	type tierState struct {
		highFrameRate bool
		hdr           bool
	}
	seen := map[int]*tierState{}

	for _, f := range formats {
		if f.IsStoryboard() {
			continue
		}
		options.CountedFormats++

		if f.HasAudio() {
			options.HasAudio = true
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
		if short > options.BestHeight {
			options.BestHeight = short
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
		if f.isHDR() {
			state.hdr = true
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
		})
	}

	sort.SliceStable(options.Tiers, func(i, j int) bool {
		return options.Tiers[i].Height > options.Tiers[j].Height
	})

	options.BestLabel = labelFor(options.BestHeight)
	return options
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
