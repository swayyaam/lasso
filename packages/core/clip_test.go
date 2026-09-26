package core

import (
	"math"
	"os"
	"strings"
	"testing"
)

func TestClipValidate(t *testing.T) {
	cases := []struct {
		clip Clip
		ok   bool
	}{
		{Clip{}, true},
		{Clip{Start: 60, End: 150}, true},
		{Clip{Start: 60}, true}, // to the end
		{Clip{End: 30}, true},   // from the start
		{Clip{Start: 150, End: 60}, false},
		{Clip{Start: 60, End: 60}, false},
		{Clip{Start: -5, End: 10}, false},
		{Clip{Start: math.NaN(), End: 10}, false},
		{Clip{Start: 1, End: math.Inf(1)}, false},
	}
	for _, tc := range cases {
		if err := tc.clip.Validate(); (err == nil) != tc.ok {
			t.Errorf("%+v: Validate() = %v, want ok=%v", tc.clip, err, tc.ok)
		}
	}
}

func TestClipLabels(t *testing.T) {
	cases := []struct {
		clip        Clip
		label, file string
		section     string
	}{
		{Clip{Start: 60, End: 150}, "1:00–2:30", "clip 1m00s-2m30s", "*60-150"},
		{Clip{Start: 3723, End: 3730.5}, "1:02:03–1:02:10.5", "clip 1h02m03s-1h02m10.5s", "*3723-3730.5"},
		{Clip{Start: 90}, "1:30 to the end", "clip 1m30s-end", "*90-inf"},
		{Clip{End: 45}, "0:00–0:45", "clip 0m00s-0m45s", "*0-45"},
	}
	for _, tc := range cases {
		if got := tc.clip.Label(); got != tc.label {
			t.Errorf("%+v Label = %q, want %q", tc.clip, got, tc.label)
		}
		if got := tc.clip.fileLabel(); got != tc.file {
			t.Errorf("%+v fileLabel = %q, want %q", tc.clip, got, tc.file)
		}
		if got := tc.clip.section(); got != tc.section {
			t.Errorf("%+v section = %q, want %q", tc.clip, got, tc.section)
		}
		if strings.Contains(tc.clip.fileLabel(), ":") {
			t.Errorf("%q has a colon, which Finder shows as a slash", tc.clip.fileLabel())
		}
	}
}

func TestAClipIsCutWhereAsked(t *testing.T) {
	o := baseOptions()
	o.Clip = Clip{Start: 60, End: 150}
	args := BuildArgs(o)

	section, ok := argValue(args, "--download-sections")
	if !ok || section != "*60-150" {
		t.Errorf("--download-sections = %q, %v; want *60-150", section, ok)
	}
	// Without it the cut lands on the nearest keyframe, which can be seconds out.
	if !hasFlag(args, "--force-keyframes-at-cuts") {
		t.Error("a clip without --force-keyframes-at-cuts starts and ends in the wrong place")
	}
}

func TestTheWholeVideoIsNotCut(t *testing.T) {
	args := BuildArgs(baseOptions())
	for _, flag := range []string{"--download-sections", "--force-keyframes-at-cuts"} {
		if hasFlag(args, flag) {
			t.Errorf("%s without a clip", flag)
		}
	}
}

func TestAClipNeverSharesAName(t *testing.T) {
	// yt-dlp skips a download whose file exists: a clip named like the whole
	// video, or like another clip, would finish at once and point at the wrong
	// file.
	name := func(c Clip, pick QuickPick) string {
		o := baseOptions()
		o.Pick, o.Clip = pick, c
		template, _ := argValue(BuildArgs(o), "-o")
		return template
	}
	whole := name(Clip{}, Pick1080p)
	first := name(Clip{Start: 60, End: 150}, Pick1080p)
	second := name(Clip{Start: 60, End: 151}, Pick1080p)
	if whole == first || first == second {
		t.Errorf("names collide: %q, %q, %q", whole, first, second)
	}
	if !strings.HasSuffix(first, " 1080p clip 1m00s-2m30s.%(ext)s") {
		t.Errorf("clip template = %q, want the rung and then the clip", first)
	}
	// A template the person set is theirs; Lasso does not rewrite it.
	o := baseOptions()
	o.Clip = Clip{Start: 60, End: 150}
	o.Output.Template = "%(title)s.%(ext)s"
	if got, _ := argValue(BuildArgs(o), "-o"); got != "%(title)s.%(ext)s" {
		t.Errorf("custom template became %q", got)
	}
}

func TestAPlaylistEntryDropsTheClip(t *testing.T) {
	o := baseOptions()
	o.Clip = Clip{Start: 60, End: 150}
	entry, err := o.ForEntry(Entry{URL: "https://www.youtube.com/watch?v=jNQXAC9IVRw", Title: "Me at the zoo"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if !entry.Clip.IsWhole() {
		t.Errorf("entry kept clip %+v; its times belong to another video", entry.Clip)
	}
}

func TestAClipsProgressIsTheClipsNotTheVideos(t *testing.T) {
	// A real 15 s clip of Big Buck Bunny at 480p with Lasso's templates. The
	// plan names the full streams (28 MB and 10 MB); ffmpeg cuts both at once
	// and reports them as one, "135+140", at the clip's own 1 MB.
	raw, err := os.ReadFile("testdata/progress-clip.txt")
	if err != nil {
		t.Fatal(err)
	}
	p := NewProgressParser()
	var last Progress
	for _, line := range strings.Split(string(raw), "\n") {
		if got, ok := p.Line(line); ok && got.Stage == StageDownloading && got.Total > 0 {
			last = got
		}
	}
	if last.Total != 1063663 || last.Percent != 100 {
		t.Errorf("finished at %d of %d (%.1f%%), want the clip's 1063663 bytes at 100%%", last.Downloaded, last.Total, last.Percent)
	}
}
