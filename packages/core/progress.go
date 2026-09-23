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

// progressLine mirrors the JSON yt-dlp is asked to emit. Two templates produce
// it: progressTemplate during the transfer, and completedTemplate once per
// finished file. Stage says which. Fields default to zero rather than null, so
// zero means "not known".
type progressLine struct {
	Stage      string  `json:"stage"`
	Downloaded int64   `json:"downloaded"`
	Total      int64   `json:"total"`
	Estimate   int64   `json:"estimate"`
	Speed      float64 `json:"speed"`
	ETA        int     `json:"eta"`
	Fragment   int     `json:"fragment"`
	Fragments  int     `json:"fragments"`
	// Path is the finished file, on a completedTemplate line only.
	Path string `json:"path"`
	// Width and Height are that file's dimensions, zero for audio.
	Width  int `json:"width"`
	Height int `json:"height"`
}

// stageComplete is the stage completedTemplate reports. It is not a Stage: a
// finished file is a result, not a point in the transfer.
const stageComplete = "complete"

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
	"SplitChapters":       "Splitting into tracks",
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
	// output is the last file yt-dlp reported as finished. Unlike filename it
	// survives post-processing, because yt-dlp reports it after the fact.
	output                    string
	outputWidth, outputHeight int
	// existing is yt-dlp finding the file already there and skipping it.
	existing bool
	// chapters are the per-track files --split-chapters wrote. They need
	// retagging afterwards, and this is the only place they are named.
	chapters []ChapterFile
}

// NewProgressParser returns a parser positioned at the fetching stage.
func NewProgressParser() *ProgressParser {
	return &ProgressParser{stage: StageFetching}
}

// OutputPath is the last file yt-dlp reported finishing, or "" if it reported
// none. For a playlist that is the last item downloaded, which is the one a
// user asking to see the result most likely means.
func (p *ProgressParser) OutputPath() string { return p.output }

// Existing reports whether yt-dlp found the file already downloaded and did
// not download it again.
func (p *ProgressParser) Existing() bool { return p.existing }

// OutputResolution labels the file OutputPath names, e.g. "1080p", or "" for
// audio and when yt-dlp did not say.
//
// It is measured on the short side, as the quality picker is: yt-dlp's height
// is the long side of a vertical video, and a 1080x1920 Short is 1080p.
func (p *ProgressParser) OutputResolution() string {
	short := p.outputHeight
	if p.outputWidth > 0 && p.outputWidth < short {
		short = p.outputWidth
	}
	return ResolutionLabel(short)
}

// ChapterFiles are the per-track files --split-chapters wrote, in the order
// yt-dlp wrote them.
func (p *ProgressParser) ChapterFiles() []ChapterFile { return p.chapters }

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

	// A finished file says nothing about progress, so it produces no update.
	// It is recorded and read back with OutputPath once the run is over.
	if raw.Stage == stageComplete {
		if raw.Path != "" {
			p.output = raw.Path
			p.outputWidth, p.outputHeight = raw.Width, raw.Height
			// The in-flight display should stop naming a file that has just
			// been replaced by this one.
			p.filename = raw.Path
		}
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
		if strings.HasSuffix(rest, "has already been downloaded") {
			p.existing = true
			return Progress{}, false
		}
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

	if tag == "SplitChapters" {
		if chapter, ok := parseChapterFile(rest); ok {
			p.chapters = append(p.chapters, chapter)
		}
		// Fall through: it is also a post-processing step worth showing.
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

// parseChapterFile reads "Chapter 001; Destination: /path/01 - Name.m4a".
//
// The number matters as much as the path: it is the track number, and it is
// what ChapterFile.Title uses to strip the prefix back off the filename.
func parseChapterFile(rest string) (ChapterFile, bool) {
	number, after, found := strings.Cut(rest, ";")
	if !found {
		return ChapterFile{}, false
	}
	digits, ok := strings.CutPrefix(strings.TrimSpace(number), "Chapter ")
	if !ok {
		return ChapterFile{}, false
	}
	n, err := strconv.Atoi(strings.TrimSpace(digits))
	if err != nil || n <= 0 {
		return ChapterFile{}, false
	}

	path, ok := strings.CutPrefix(strings.TrimSpace(after), "Destination: ")
	if !ok {
		return ChapterFile{}, false
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return ChapterFile{}, false
	}
	return ChapterFile{Number: n, Path: path}, true
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
