package core

import (
	"math"
	"os"
	"strings"
	"testing"
)

func TestParseProgressJSONLine(t *testing.T) {
	p := NewProgressParser()
	line := `{"stage":"downloading","downloaded":50,"total":200,"estimate":0,"speed":1024.5,"eta":7,"fragment":0,"fragments":0}`

	got, ok := p.Line(line)
	if !ok {
		t.Fatal("progress line was not recognised")
	}
	if got.Stage != StageDownloading {
		t.Errorf("Stage = %q", got.Stage)
	}
	if got.Percent != 25 {
		t.Errorf("Percent = %v, want 25", got.Percent)
	}
	if got.Downloaded != 50 || got.Total != 200 {
		t.Errorf("bytes = %d/%d", got.Downloaded, got.Total)
	}
	if got.Speed != 1024.5 || got.ETA != 7 {
		t.Errorf("speed = %v, eta = %d", got.Speed, got.ETA)
	}
}

func TestProgressUsesEstimateWhenTotalUnknown(t *testing.T) {
	// Fragmented and HLS downloads report an estimate instead of an exact size.
	p := NewProgressParser()
	got, ok := p.Line(`{"stage":"downloading","downloaded":25,"total":0,"estimate":100,"speed":0,"eta":0,"fragment":2,"fragments":8}`)
	if !ok {
		t.Fatal("not recognised")
	}
	if got.Total != 100 {
		t.Errorf("Total = %d, want the estimate", got.Total)
	}
	if got.Percent != 25 {
		t.Errorf("Percent = %v, want 25", got.Percent)
	}
	if got.Fragment != 2 || got.Fragments != 8 {
		t.Errorf("fragments = %d/%d", got.Fragment, got.Fragments)
	}
}

func TestProgressPercentUnknown(t *testing.T) {
	p := NewProgressParser()
	got, ok := p.Line(`{"stage":"downloading","downloaded":25,"total":0,"estimate":0,"speed":0,"eta":0,"fragment":0,"fragments":0}`)
	if !ok {
		t.Fatal("not recognised")
	}
	if got.Percent != PercentUnknown {
		t.Errorf("Percent = %v, want PercentUnknown for an unknown size", got.Percent)
	}
}

func TestProgressPercentNeverExceeds100(t *testing.T) {
	// yt-dlp occasionally reports slightly more bytes than the advertised total.
	p := NewProgressParser()
	got, _ := p.Line(`{"stage":"downloading","downloaded":110,"total":100,"estimate":0,"speed":0,"eta":0,"fragment":0,"fragments":0}`)
	if got.Percent != 100 {
		t.Errorf("Percent = %v, want it clamped to 100", got.Percent)
	}
}

func TestParsePostProcessingStages(t *testing.T) {
	cases := []struct {
		line       string
		wantDetail string
	}{
		{`[Merger] Merging formats into "video.mkv"`, "Merging video and audio"},
		{`[ExtractAudio] Destination: audio.mp3`, "Extracting audio"},
		{`[EmbedSubtitle] Embedding subtitles in "video.mkv"`, "Embedding subtitles"},
		{`[Metadata] Adding metadata to "video.mkv"`, "Adding metadata"},
		{`[EmbedThumbnail] Adding thumbnail to "video.mkv"`, "Embedding thumbnail"},
		{`[SponsorBlock] Found 3 segments`, "Checking SponsorBlock"},
		{`[ModifyChapters] Removing segments`, "Applying chapter edits"},
	}

	for _, tc := range cases {
		t.Run(tc.wantDetail, func(t *testing.T) {
			p := NewProgressParser()
			got, ok := p.Line(tc.line)
			if !ok {
				t.Fatalf("not recognised: %s", tc.line)
			}
			if got.Stage != StagePostProcessing {
				t.Errorf("Stage = %q, want post-processing", got.Stage)
			}
			if got.Detail != tc.wantDetail {
				t.Errorf("Detail = %q, want %q", got.Detail, tc.wantDetail)
			}
		})
	}
}

func TestIgnoresChatterLines(t *testing.T) {
	// Most of yt-dlp's output says nothing about progress and must not produce
	// spurious updates.
	noise := []string{
		"[youtube] Extracting URL: https://example.com",
		"[youtube] abc: Downloading webpage",
		"[info] abc: Downloading 1 format(s): 251",
		"Deleting original file x.webm (pass -k to keep)",
		"WARNING: something happened",
		"",
		"   ",
		"not bracketed at all",
		"[this is prose] not a tag",
	}
	p := NewProgressParser()
	for _, line := range noise {
		if got, ok := p.Line(line); ok {
			t.Errorf("line %q produced an update: %+v", line, got)
		}
	}
}

func TestTracksPlaylistPosition(t *testing.T) {
	p := NewProgressParser()

	if _, ok := p.Line("[download] Downloading item 3 of 12"); !ok {
		t.Fatal("playlist position was not recognised")
	}
	// The position must persist onto later updates, which do not repeat it.
	got, ok := p.Line(`{"stage":"downloading","downloaded":1,"total":2,"estimate":0,"speed":0,"eta":0,"fragment":0,"fragments":0}`)
	if !ok {
		t.Fatal("not recognised")
	}
	if got.Item != 3 || got.Items != 12 {
		t.Errorf("position = %d of %d, want 3 of 12", got.Item, got.Items)
	}

	// And onto post-processing, which never repeats it either.
	got, _ = p.Line(`[Merger] Merging formats into "x.mkv"`)
	if got.Item != 3 || got.Items != 12 {
		t.Errorf("post-processing lost the playlist position: %d of %d", got.Item, got.Items)
	}
}

func TestTracksDestinationFilename(t *testing.T) {
	p := NewProgressParser()
	if _, ok := p.Line("[download] Destination: Me at the zoo.webm"); !ok {
		t.Fatal("destination line was not recognised")
	}
	got, _ := p.Line(`{"stage":"downloading","downloaded":1,"total":2,"estimate":0,"speed":0,"eta":0,"fragment":0,"fragments":0}`)
	if got.Filename != "Me at the zoo.webm" {
		t.Errorf("Filename = %q", got.Filename)
	}
}

func TestSurvivesTruncatedAndInterleavedJSON(t *testing.T) {
	// A partial write or a line that got mixed with other output must be
	// skipped, never crash and never fail the download.
	p := NewProgressParser()
	bad := []string{
		`{"stage":"downloading","downloaded":50`,
		`{"stage":"downloading","downloaded":NA,"total":100}`,
		`{}`,
		`{"stage":""}`,
		`{"stage":"downloading","downloaded":"lots"}`,
		`{`,
		`}`,
	}
	for _, line := range bad {
		if _, ok := p.Line(line); ok {
			t.Errorf("malformed line was accepted: %s", line)
		}
	}
	// A good line after the bad ones must still work.
	if _, ok := p.Line(`{"stage":"downloading","downloaded":1,"total":2,"estimate":0,"speed":0,"eta":0,"fragment":0,"fragments":0}`); !ok {
		t.Error("parser did not recover after malformed input")
	}
}

// TestParsesRealDownloadLog runs the parser over output captured from an actual
// yt-dlp run.
func TestParsesRealDownloadLog(t *testing.T) {
	data, err := os.ReadFile("testdata/download-log.txt")
	if err != nil {
		t.Fatal(err)
	}

	p := NewProgressParser()
	var updates []Progress
	for _, line := range strings.Split(string(data), "\n") {
		if progress, ok := p.Line(line); ok {
			updates = append(updates, progress)
		}
	}

	if len(updates) == 0 {
		t.Fatal("no progress parsed from a real log")
	}

	var sawDownloading, sawPostProcessing bool
	var maxPercent float64
	for _, u := range updates {
		switch u.Stage {
		case StageDownloading:
			sawDownloading = true
			maxPercent = math.Max(maxPercent, u.Percent)
		case StagePostProcessing:
			sawPostProcessing = true
		}
	}
	if !sawDownloading {
		t.Error("never reached the downloading stage")
	}
	if !sawPostProcessing {
		t.Error("never reached post-processing")
	}
	if maxPercent != 100 {
		t.Errorf("peak percent = %v, want the download to reach 100", maxPercent)
	}
	// The stages must arrive in order; post-processing never precedes bytes.
	for i := 1; i < len(updates); i++ {
		if updates[i-1].Stage == StagePostProcessing && updates[i].Stage == StageDownloading {
			t.Error("stages went backwards from post-processing to downloading")
			break
		}
	}
}

// FuzzProgressParser checks that no input can panic the parser. It runs on
// every yt-dlp line, including anything a video title manages to smuggle in.
func FuzzProgressParser(f *testing.F) {
	f.Add(`{"stage":"downloading","downloaded":50,"total":200,"estimate":0,"speed":1.5,"eta":7,"fragment":0,"fragments":0}`)
	f.Add(`[Merger] Merging formats into "x.mkv"`)
	f.Add("[download] Downloading item 3 of 12")
	f.Add("[download] Destination: x.webm")
	f.Add("")
	f.Add("[")
	f.Add("{")

	f.Fuzz(func(t *testing.T, line string) {
		p := NewProgressParser()
		got, ok := p.Line(line)
		if !ok {
			return
		}
		if got.Percent < PercentUnknown || got.Percent > 100 {
			t.Errorf("percent %v out of range for input %q", got.Percent, line)
		}
	})
}
