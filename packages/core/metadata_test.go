package core

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
)

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	return data
}

func TestParseSingleVideo(t *testing.T) {
	m, err := ParseMetadata(readFixture(t, "single.json"))
	if err != nil {
		t.Fatalf("ParseMetadata: %v", err)
	}

	if m.Kind != KindVideo {
		t.Errorf("Kind = %q, want video", m.Kind)
	}
	if m.IsPlaylist() {
		t.Error("IsPlaylist = true for a single video")
	}
	if m.Count() != 1 {
		t.Errorf("Count = %d, want 1", m.Count())
	}
	if m.ID != "jNQXAC9IVRw" {
		t.Errorf("ID = %q", m.ID)
	}
	if m.Title != "Me at the zoo" {
		t.Errorf("Title = %q", m.Title)
	}
	if m.Uploader != "jawed" {
		t.Errorf("Uploader = %q", m.Uploader)
	}
	if m.Duration != 19 {
		t.Errorf("Duration = %v, want 19", m.Duration)
	}
	if len(m.Formats) == 0 {
		t.Fatal("no formats parsed")
	}
	if len(m.Thumbnails) == 0 {
		t.Fatal("no thumbnails parsed")
	}
}

func TestParseFormatTracks(t *testing.T) {
	m, err := ParseMetadata(readFixture(t, "single.json"))
	if err != nil {
		t.Fatal(err)
	}

	var video, audio int
	for _, f := range m.Formats {
		if f.HasVideo() {
			video++
		}
		if f.HasAudio() {
			audio++
		}
		// "none" is yt-dlp's way of saying the track is absent; it must never
		// be reported as a real codec.
		if f.VCodec == "none" && f.HasVideo() {
			t.Errorf("format %s reports video despite vcodec=none", f.ID)
		}
	}
	if video == 0 {
		t.Error("no video-bearing formats found")
	}
}

func TestParsePlaylist(t *testing.T) {
	m, err := ParseMetadata(readFixture(t, "playlist.json"))
	if err != nil {
		t.Fatalf("ParseMetadata: %v", err)
	}

	if m.Kind != KindPlaylist {
		t.Errorf("Kind = %q, want playlist", m.Kind)
	}
	if !m.IsPlaylist() {
		t.Error("IsPlaylist = false for a playlist")
	}
	if m.Count() != len(m.Entries) || m.Count() == 0 {
		t.Errorf("Count = %d, entries = %d", m.Count(), len(m.Entries))
	}
	for i, e := range m.Entries {
		if e.ID == "" || e.Title == "" || e.URL == "" {
			t.Errorf("entry %d is incomplete: %+v", i, e)
		}
	}
	// --flat-playlist does not visit the videos, so there are no formats.
	if len(m.Formats) != 0 {
		t.Errorf("flat playlist carried %d formats", len(m.Formats))
	}
}

func TestParseMetadataRejectsGarbage(t *testing.T) {
	cases := []struct{ name, input string }{
		{"not json", "this is not json"},
		{"empty object", "{}"},
		{"empty input", ""},
		{"json array", "[1,2,3]"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ParseMetadata([]byte(tc.input)); err == nil {
				t.Error("ParseMetadata accepted invalid input")
			}
		})
	}
}

func TestParseMetadataToleratesNulls(t *testing.T) {
	// yt-dlp emits null for unknown numbers on live streams and some sites.
	input := `{"_type":"video","id":"x","title":"T","duration":null,
	           "formats":[{"format_id":"1","ext":"mp4","fps":null,"filesize":null,"tbr":null}]}`

	m, err := ParseMetadata([]byte(input))
	if err != nil {
		t.Fatalf("ParseMetadata: %v", err)
	}
	if m.Duration != 0 {
		t.Errorf("Duration = %v, want 0 for null", m.Duration)
	}
	if len(m.Formats) != 1 || m.Formats[0].FPS != 0 || m.Formats[0].Size() != 0 {
		t.Errorf("null numbers did not become zero: %+v", m.Formats)
	}
}

func TestFormatSizePrefersExact(t *testing.T) {
	exact := Format{Filesize: 100, FilesizeApprox: 999}
	if got := exact.Size(); got != 100 {
		t.Errorf("Size = %d, want the exact size", got)
	}
	approx := Format{FilesizeApprox: 999}
	if got := approx.Size(); got != 999 {
		t.Errorf("Size = %d, want the approximate size", got)
	}
	if got := (Format{}).Size(); got != 0 {
		t.Errorf("Size = %d, want 0 when unknown", got)
	}
}

func TestBestThumbnailPicksSmallestThatFits(t *testing.T) {
	thumbs := []Thumbnail{
		{URL: "a", Width: 120},
		{URL: "b", Width: 320},
		{URL: "c", Width: 480},
		{URL: "d", Width: 1280},
	}

	// Downscaling is fine; upscaling is not, so never pick something narrower.
	got, ok := BestThumbnail(thumbs, 320)
	if !ok || got.URL != "b" {
		t.Errorf("BestThumbnail(320) = %+v, want b", got)
	}
	got, _ = BestThumbnail(thumbs, 400)
	if got.URL != "c" {
		t.Errorf("BestThumbnail(400) = %+v, want c", got)
	}
	got, _ = BestThumbnail(thumbs, 100)
	if got.URL != "a" {
		t.Errorf("BestThumbnail(100) = %+v, want a", got)
	}
}

func TestBestThumbnailFallsBackToLargest(t *testing.T) {
	thumbs := []Thumbnail{{URL: "a", Width: 120}, {URL: "b", Width: 320}}

	got, ok := BestThumbnail(thumbs, 4000)
	if !ok || got.URL != "b" {
		t.Errorf("BestThumbnail = %+v, want the largest available", got)
	}
}

func TestBestThumbnailHandlesMissingDimensions(t *testing.T) {
	// The common real-world case: most entries carry a preference but no size.
	thumbs := []Thumbnail{
		{URL: "worst", Preference: -37},
		{URL: "middle", Preference: -35},
		{URL: "best", Preference: -33},
	}
	got, ok := BestThumbnail(thumbs, 320)
	if !ok || got.URL != "best" {
		t.Errorf("BestThumbnail = %+v, want yt-dlp's last (best) entry", got)
	}
}

func TestBestThumbnailOnRealFixture(t *testing.T) {
	m, err := ParseMetadata(readFixture(t, "single.json"))
	if err != nil {
		t.Fatal(err)
	}
	// The fixture is a real response: 44 thumbnails, only a handful sized.
	got, ok := BestThumbnail(m.Thumbnails, 320)
	if !ok {
		t.Fatal("no thumbnail chosen from a real response")
	}
	if got.URL == "" {
		t.Error("chosen thumbnail has no URL")
	}
	if got.Width > 0 && got.Width < 320 {
		t.Errorf("chose a %dpx thumbnail for a 320px slot", got.Width)
	}
}

func TestBestThumbnailEmpty(t *testing.T) {
	if _, ok := BestThumbnail(nil, 320); ok {
		t.Error("BestThumbnail found something in an empty list")
	}
	if _, ok := BestThumbnail([]Thumbnail{{URL: ""}}, 320); ok {
		t.Error("BestThumbnail returned an entry with no URL")
	}
}

func TestMetadataArgs(t *testing.T) {
	o := baseOptions()
	args := MetadataArgs(o)

	for _, want := range []string{"--ignore-config", "-J", "--flat-playlist"} {
		if !hasFlag(args, want) {
			t.Errorf("missing %s", want)
		}
	}
	if args[len(args)-1] != o.URL || args[len(args)-2] != "--" {
		t.Errorf("URL is not last behind a separator: %v", args[len(args)-3:])
	}
	// Download-shaping options have no business in a metadata call.
	if hasFlag(args, "-f") || hasFlag(args, "-o") || hasFlag(args, "--limit-rate") {
		t.Errorf("metadata args carry download options: %v", args)
	}
}

func TestMetadataArgsCarryCookies(t *testing.T) {
	// Age-restricted videos will not resolve without cookies.
	o := baseOptions()
	o.Network.Cookies = BrowserSafari

	if got, _ := argValue(MetadataArgs(o), "--cookies-from-browser"); got != "safari" {
		t.Errorf("--cookies-from-browser = %q", got)
	}
}

// fakeRunner replays canned output instead of running yt-dlp.
type fakeRunner struct {
	stdout  []string
	stderr  []string
	err     error
	gotArgs []string
}

func (f *fakeRunner) Run(_ context.Context, args []string, stdout, stderr func(string)) error {
	f.gotArgs = args
	for _, l := range f.stdout {
		stdout(l)
	}
	for _, l := range f.stderr {
		stderr(l)
	}
	return f.err
}

func TestFetchMetadataParsesRunnerOutput(t *testing.T) {
	runner := &fakeRunner{stdout: []string{string(readFixture(t, "single.json"))}}

	m, err := FetchMetadata(context.Background(), runner, baseOptions())
	if err != nil {
		t.Fatalf("FetchMetadata: %v", err)
	}
	if m.Title != "Me at the zoo" {
		t.Errorf("Title = %q", m.Title)
	}
}

func TestFetchMetadataValidatesFirst(t *testing.T) {
	runner := &fakeRunner{}
	o := baseOptions()
	o.URL = "file:///etc/passwd"

	if _, err := FetchMetadata(context.Background(), runner, o); err == nil {
		t.Fatal("FetchMetadata accepted a file:// URL")
	}
	if runner.gotArgs != nil {
		t.Error("FetchMetadata ran yt-dlp despite invalid options")
	}
}

func TestFetchMetadataClassifiesFailure(t *testing.T) {
	runner := &fakeRunner{
		stderr: []string{"ERROR: [youtube] abc: Private video. Sign in if you've been granted access"},
		err:    errors.New("exit status 1"),
	}

	_, err := FetchMetadata(context.Background(), runner, baseOptions())
	if err == nil {
		t.Fatal("FetchMetadata returned no error")
	}
	var d *DownloadError
	if !errors.As(err, &d) {
		t.Fatalf("error is %T, want *DownloadError", err)
	}
	if d.Kind != ErrPrivate {
		t.Errorf("Kind = %q, want private", d.Kind)
	}
	if !strings.Contains(d.Raw, "Private video") {
		t.Error("raw output was not kept for the details toggle")
	}
}

func TestFetchMetadataHandlesEmptyOutput(t *testing.T) {
	runner := &fakeRunner{}
	if _, err := FetchMetadata(context.Background(), runner, baseOptions()); err == nil {
		t.Fatal("FetchMetadata accepted empty output")
	}
}
