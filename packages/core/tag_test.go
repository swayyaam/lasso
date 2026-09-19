package core

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestChapterTitleStripsThePrefixLassoPutThere(t *testing.T) {
	cases := []struct {
		name   string
		number int
		path   string
		want   string
	}{
		{"ordinary", 1, "/m/01 - Opening Theme.m4a", "Opening Theme"},
		{"double digits", 12, "/m/12 - Finale.opus", "Finale"},
		{
			// The prefix is rebuilt from the number and stripped exactly once,
			// so a chapter genuinely named like one survives.
			name: "a chapter whose own name looks like a prefix", number: 3,
			path: "/m/03 - 01 - Intro.m4a", want: "01 - Intro",
		},
		{
			name: "a title containing a dash", number: 2,
			path: "/m/02 - Death Cab - Transatlanticism.flac", want: "Death Cab - Transatlanticism",
		},
		{
			// Nothing to strip means nothing is stripped, rather than a guess.
			name: "a filename that does not match", number: 9,
			path: "/m/whatever.m4a", want: "whatever",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ChapterFile{Number: c.number, Path: c.path}.Title()
			if got != c.want {
				t.Errorf("Title() = %q, want %q", got, c.want)
			}
		})
	}
}

func TestTrackTagsOmitWhatTheyDoNotKnow(t *testing.T) {
	// A blank tag is worse than the value already on the file, which was copied
	// from the recording and is probably right.
	if !(TrackTags{}).Empty() {
		t.Error("empty tags reported as having something to write")
	}

	args := TrackTags{Title: "Finale", Track: 3, Tracks: 12, Album: "Live"}.args()
	joined := strings.Join(args, " ")
	for _, want := range []string{"title=Finale", "album=Live", "track=3/12"} {
		if !strings.Contains(joined, want) {
			t.Errorf("args %v missing %q", args, want)
		}
	}

	// Without a total there is no "/n" to write.
	if joined := strings.Join(TrackTags{Track: 3}.args(), " "); joined != "-metadata track=3" {
		t.Errorf("args = %q, want a bare track number", joined)
	}
}

func TestParseChapterFileLine(t *testing.T) {
	// Verbatim from a real run.
	line := `[SplitChapters] Chapter 002; Destination: /Users/x/Music/02 - Second Movement.m4a`

	p := NewProgressParser()
	p.Line(line)

	files := p.ChapterFiles()
	if len(files) != 1 {
		t.Fatalf("got %d chapter files, want 1", len(files))
	}
	if files[0].Number != 2 {
		t.Errorf("Number = %d, want 2", files[0].Number)
	}
	if files[0].Path != "/Users/x/Music/02 - Second Movement.m4a" {
		t.Errorf("Path = %q", files[0].Path)
	}
	if files[0].Title() != "Second Movement" {
		t.Errorf("Title() = %q, want the chapter's own name", files[0].Title())
	}
}

func TestChapterLinesAreAlsoProgress(t *testing.T) {
	// Splitting is a post-processing step, and a long album spends real time
	// in it, so it has to reach the UI as well as be recorded.
	p := NewProgressParser()
	progress, ok := p.Line(`[SplitChapters] Chapter 001; Destination: /m/01 - One.m4a`)
	if !ok {
		t.Fatal("a chapter line produced no progress update")
	}
	if progress.Stage != StagePostProcessing {
		t.Errorf("Stage = %q, want %q", progress.Stage, StagePostProcessing)
	}
}

func TestIgnoresMalformedChapterLines(t *testing.T) {
	p := NewProgressParser()
	for _, line := range []string{
		`[SplitChapters] Splitting video by chapters; 3 chapters found`,
		`[SplitChapters] Chapter abc; Destination: /m/x.m4a`,
		`[SplitChapters] Chapter 001; Destination: `,
		`[SplitChapters] Chapter 000; Destination: /m/x.m4a`,
		`[SplitChapters] nonsense`,
	} {
		p.Line(line)
	}
	if got := p.ChapterFiles(); len(got) != 0 {
		t.Errorf("got %v, want malformed lines to produce no chapter files", got)
	}
}

// recordingTagger captures what it was asked to write.
type recordingTagger struct {
	calls []struct {
		path string
		tags TrackTags
	}
	err error
}

func (r *recordingTagger) Tag(_ context.Context, path string, tags TrackTags) error {
	r.calls = append(r.calls, struct {
		path string
		tags TrackTags
	}{path, tags})
	return r.err
}

// chapterRunner scripts a download that splits into three tracks.
func chapterRunner() *funcRunner {
	return &funcRunner{run: func(_ context.Context, args []string, stdout, _ func(string)) error {
		if isMetadataCall(args) {
			stdout(`{"id":"x","title":"Various Artists - Greatest Hits"}`)
			return nil
		}
		stdout(`{"stage":"downloading","downloaded":10,"total":10}`)
		stdout(`[SplitChapters] Splitting video by chapters; 3 chapters found`)
		stdout(`[SplitChapters] Chapter 001; Destination: /m/01 - Opening Theme.m4a`)
		stdout(`[SplitChapters] Chapter 002; Destination: /m/02 - Second Movement.m4a`)
		stdout(`[SplitChapters] Chapter 003; Destination: /m/03 - Finale.m4a`)
		stdout(`{"stage":"complete","path":"/m/Various Artists - Greatest Hits.m4a"}`)
		return nil
	}}
}

func musicOptions() Options {
	return Options{
		URL:   "https://example.com/album",
		Pick:  PickAudioOriginal,
		Music: Music{Tags: true, SplitChapters: true},
	}
}

func TestEveryTrackGetsItsOwnTitle(t *testing.T) {
	// The whole reason this step exists. yt-dlp cuts the tracks with the audio
	// copied, which carries the recording's tags onto all of them — so without
	// retagging, a twelve-track album is twelve files with one name.
	tagger := &recordingTagger{}
	h := newQueueHarnessWith(t, chapterRunner(), 1, tagger)

	item, _ := h.q.Add(musicOptions(), "Various Artists - Greatest Hits")
	h.waitFor(t, item.ID, StateDone)

	if len(tagger.calls) != 3 {
		t.Fatalf("tagged %d files, want 3", len(tagger.calls))
	}

	wantTitles := []string{"Opening Theme", "Second Movement", "Finale"}
	for i, call := range tagger.calls {
		if call.tags.Title != wantTitles[i] {
			t.Errorf("track %d title = %q, want %q", i+1, call.tags.Title, wantTitles[i])
		}
		if call.tags.Track != i+1 {
			t.Errorf("track %d number = %d", i+1, call.tags.Track)
		}
		if call.tags.Tracks != 3 {
			t.Errorf("track %d total = %d, want 3", i+1, call.tags.Tracks)
		}
		if call.tags.Album != "Various Artists - Greatest Hits" {
			t.Errorf("track %d album = %q, want the recording's title", i+1, call.tags.Album)
		}
	}
}

func TestTaggingFailureDoesNotFailTheDownload(t *testing.T) {
	// The tracks exist and play. Losing them over a metadata rewrite would be
	// a bad trade — but the user still has to be told the tags need a look.
	tagger := &recordingTagger{err: errors.New("ffmpeg: permission denied")}
	h := newQueueHarnessWith(t, chapterRunner(), 1, tagger)

	item, _ := h.q.Add(musicOptions(), "Various Artists - Greatest Hits")
	h.waitFor(t, item.ID, StateDone)

	done, _ := h.q.Get(item.ID)
	if done.State != StateDone {
		t.Errorf("State = %q, want the download to have succeeded", done.State)
	}
	if done.Notice == "" {
		t.Error("a tagging failure was not reported at all")
	}
	if !strings.Contains(done.Detail, "permission denied") {
		t.Errorf("Detail = %q, want the reason kept", done.Detail)
	}
}

func TestNoTaggerMeansNoRetagging(t *testing.T) {
	// Splitting still works without one; the tracks simply keep the recording's
	// tags. It must not fail the download.
	h := newQueueHarnessWith(t, chapterRunner(), 1, nil)

	item, _ := h.q.Add(musicOptions(), "Album")
	h.waitFor(t, item.ID, StateDone)

	if done, _ := h.q.Get(item.ID); done.State != StateDone {
		t.Errorf("State = %q, want done", done.State)
	}
}

func TestOrdinaryDownloadsAreNotTagged(t *testing.T) {
	// Nothing was split, so there is nothing to retag and ffmpeg should never
	// be invoked.
	tagger := &recordingTagger{}
	runner := &funcRunner{run: func(_ context.Context, args []string, stdout, _ func(string)) error {
		if isMetadataCall(args) {
			stdout(`{"id":"x","title":"Clip"}`)
			return nil
		}
		stdout(`{"stage":"complete","path":"/m/Clip.mp4"}`)
		return nil
	}}

	h := newQueueHarnessWith(t, runner, 1, tagger)
	item, _ := h.q.Add(Options{URL: "https://example.com/v", Pick: PickBest}, "Clip")
	h.waitFor(t, item.ID, StateDone)

	if len(tagger.calls) != 0 {
		t.Errorf("tagged %d files on a download that was never split", len(tagger.calls))
	}
}

func TestMusicArgs(t *testing.T) {
	o := Options{URL: "https://example.com/a", Pick: PickAudioOriginal}
	o.Music = Music{Tags: true, SplitChapters: true}
	args := BuildArgs(o)

	if !hasFlag(args, "--split-chapters") {
		t.Error("--split-chapters missing")
	}
	// The chapter template is a contract with ChapterFile.Title, so it has to
	// be the one that parses back.
	if !slices.Contains(args, ChapterTemplate) {
		t.Errorf("args %v do not carry the chapter template", args)
	}
	// Tagging without the metadata post-processor would parse fields that
	// nothing ever writes.
	if !hasFlag(args, "--embed-metadata") {
		t.Error("--embed-metadata missing from a tagged download")
	}

	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "meta_artist") {
		t.Error("nothing parses an artist out of the title")
	}
	if !strings.Contains(joined, "%(artist,title)s") {
		// Reading artist-then-title is what makes the split safe: a site that
		// supplies a real artist yields no " - " and nothing is overwritten.
		t.Error("the title split does not prefer a real artist field")
	}
}

func TestMusicOptionsAreOffByDefault(t *testing.T) {
	args := BuildArgs(Options{URL: "https://example.com/v", Pick: PickBest})
	for _, flag := range []string{"--split-chapters", "--parse-metadata"} {
		if hasFlag(args, flag) {
			t.Errorf("%s on an ordinary download", flag)
		}
	}
}

func TestEmbedMetadataIsNotDuplicated(t *testing.T) {
	o := Options{URL: "https://example.com/a", Pick: PickAudioFLAC}
	o.Music = Music{Tags: true}
	o.Enhancements.EmbedMetadata = true

	var count int
	for _, arg := range BuildArgs(o) {
		if arg == "--embed-metadata" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("--embed-metadata appears %d times, want once", count)
	}
}
