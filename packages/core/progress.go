package core

import (
	"encoding/json"
	"strconv"
	"strings"
)

// Stage is where a download has got to.
type Stage string

const (
	StageFetching       Stage = "fetching"
	StageDownloading    Stage = "downloading"
	StagePostProcessing Stage = "post-processing"
)

// PercentUnknown is reported when the total size is not yet known, which is
// normal for fragmented and live streams.
const PercentUnknown = -1

// Progress is one update about a running download.
type Progress struct {
	Stage      Stage   `json:"stage"`
	Percent    float64 `json:"percent"`
	Downloaded int64   `json:"downloaded"`
	Total      int64   `json:"total"`
	Speed      float64 `json:"speed"`
	ETA        int     `json:"eta"`
	Fragment   int     `json:"fragment"`
	Fragments  int     `json:"fragments"`
	// Item and Items track position within a playlist, both zero otherwise.
	Item  int `json:"item"`
	Items int `json:"items"`
	// Detail is a short human label, mainly for post-processing steps.
	Detail string `json:"detail"`
	// Filename is the file currently being written, when yt-dlp has said.
	Filename string `json:"filename"`
}

// progressLine mirrors the JSON emitted by progressTemplate. Fields default to
// zero rather than null, so zero means "not known".
type progressLine struct {
	Stage      string  `json:"stage"`
	Downloaded int64   `json:"downloaded"`
	Total      int64   `json:"total"`
	Estimate   int64   `json:"estimate"`
	Speed      float64 `json:"speed"`
	ETA        int     `json:"eta"`
	Fragment   int     `json:"fragment"`
	Fragments  int     `json:"fragments"`
}

// postProcessorLabels maps yt-dlp's bracketed post-processor tags to something
// worth showing. Matching on the tag rather than the full sentence keeps this
// working when yt-dlp rewords a message.
var postProcessorLabels = map[string]string{
	"Merger":              "Merging video and audio",
	"ExtractAudio":        "Extracting audio",
	"EmbedSubtitle":       "Embedding subtitles",
	"EmbedThumbnail":      "Embedding thumbnail",
	"Metadata":            "Adding metadata",
	"ModifyChapters":      "Applying chapter edits",
	"SponsorBlock":        "Checking SponsorBlock",
	"ThumbnailsConvertor": "Converting thumbnail",
	"VideoConvertor":      "Converting video",
	"VideoRemuxer":        "Repackaging",
	"SplitChapters":       "Splitting chapters",
	"FixupM3u8":           "Finishing up",
	"FixupM4a":            "Finishing up",
	"FixupMp4":            "Finishing up",
	"Fixup":               "Finishing up",
}

// ProgressParser turns a stream of yt-dlp output lines into Progress updates.
//
// It is stateful because some facts persist across lines: the playlist position
// is announced once and applies to everything after it, and post-processing
// steps do not repeat the position.
//
// A ProgressParser is not safe for concurrent use; each download owns one.
type ProgressParser struct {
	item     int
	items    int
	filename string
	stage    Stage
}

// NewProgressParser returns a parser positioned at the fetching stage.
func NewProgressParser() *ProgressParser {
	return &ProgressParser{stage: StageFetching}
}

// Line consumes one line of yt-dlp output. It reports false for lines that say
// nothing about progress, which is most of them.
func (p *ProgressParser) Line(line string) (Progress, bool) {
	line = strings.TrimSpace(line)
	if line == "" {
		return Progress{}, false
	}

	if strings.HasPrefix(line, "{") {
		return p.parseJSON(line)
	}
	return p.parseLogLine(line)
}

func (p *ProgressParser) parseJSON(line string) (Progress, bool) {
	var raw progressLine
	if err := json.Unmarshal([]byte(line), &raw); err != nil {
		// A truncated or interleaved line is not worth failing a download over.
		return Progress{}, false
	}
	if raw.Stage == "" {
		return Progress{}, false
	}

	total := raw.Total
	if total == 0 {
		// Fragmented downloads report an estimate instead of an exact size.
		total = raw.Estimate
	}

	p.stage = StageDownloading
	return p.decorate(Progress{
		Stage:      StageDownloading,
		Percent:    percentOf(raw.Downloaded, total),
		Downloaded: raw.Downloaded,
		Total:      total,
		Speed:      raw.Speed,
		ETA:        raw.ETA,
		Fragment:   raw.Fragment,
		Fragments:  raw.Fragments,
	}), true
}

func (p *ProgressParser) parseLogLine(line string) (Progress, bool) {
	tag, rest, ok := bracketTag(line)
	if !ok {
		return Progress{}, false
	}

	if tag == "download" {
		if item, items, ok := parseItemPosition(rest); ok {
			p.item, p.items = item, items
			return p.decorate(Progress{Stage: p.stage}), true
		}
		if name, ok := strings.CutPrefix(rest, "Destination: "); ok {
			p.filename = strings.TrimSpace(name)
			p.stage = StageDownloading
			return p.decorate(Progress{Stage: StageDownloading, Percent: PercentUnknown}), true
		}
		return Progress{}, false
	}

	if label, ok := postProcessorLabels[tag]; ok {
		p.stage = StagePostProcessing
		return p.decorate(Progress{
			Stage:   StagePostProcessing,
			Percent: PercentUnknown,
			Detail:  label,
		}), true
	}

	return Progress{}, false
}

// decorate attaches the facts the parser is carrying across lines.
func (p *ProgressParser) decorate(progress Progress) Progress {
	progress.Item = p.item
	progress.Items = p.items
	if progress.Filename == "" {
		progress.Filename = p.filename
	}
	return progress
}

// bracketTag splits a line of the form "[tag] rest".
func bracketTag(line string) (tag, rest string, ok bool) {
	if !strings.HasPrefix(line, "[") {
		return "", "", false
	}
	end := strings.Index(line, "]")
	if end <= 1 {
		return "", "", false
	}
	tag = line[1:end]
	rest = strings.TrimSpace(line[end+1:])
	// A tag is a single bare word; anything else is prose that happens to
	// start with a bracket.
	if strings.ContainsAny(tag, " \t") {
		return "", "", false
	}
	return tag, rest, true
}

// parseItemPosition reads "Downloading item 3 of 12".
func parseItemPosition(rest string) (item, items int, ok bool) {
	const prefix = "Downloading item "
	if !strings.HasPrefix(rest, prefix) {
		return 0, 0, false
	}
	first, second, found := strings.Cut(strings.TrimPrefix(rest, prefix), " of ")
	if !found {
		return 0, 0, false
	}
	item, err := strconv.Atoi(strings.TrimSpace(first))
	if err != nil {
		return 0, 0, false
	}
	items, err = strconv.Atoi(strings.TrimSpace(second))
	if err != nil {
		return 0, 0, false
	}
	return item, items, true
}

func percentOf(downloaded, total int64) float64 {
	if total <= 0 || downloaded < 0 {
		return PercentUnknown
	}
	percent := float64(downloaded) / float64(total) * 100
	if percent > 100 {
		return 100
	}
	return percent
}
