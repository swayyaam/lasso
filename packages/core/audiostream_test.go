package core

import (
	"strings"
	"testing"
)

func TestCodecName(t *testing.T) {
	cases := map[string]string{
		"opus": "Opus", "mp4a.40.2": "AAC", "mp4a.40.5": "AAC", "mp3": "MP3",
		"vorbis": "Vorbis", "flac": "FLAC", "alac": "ALAC", "ac-3": "AC-3",
		"ec-3": "E-AC-3", "none": "", "": "", "weird": "WEIRD",
	}
	for in, want := range cases {
		if got := CodecName(in); got != want {
			t.Errorf("CodecName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAudioStreamLabel(t *testing.T) {
	if got := (AudioStream{Codec: "Opus", Kbps: 129}).Label(); got != "Opus 129 kbps" {
		t.Errorf("Label = %q", got)
	}
	if got := (AudioStream{Codec: "MP3"}).Label(); got != "MP3" {
		t.Errorf("Label without a bitrate = %q, want the codec alone", got)
	}
	if got := (AudioStream{}).Label(); got != "" {
		t.Errorf("empty stream labelled %q", got)
	}
}

func TestOriginalIsTheHighestBitrateStream(t *testing.T) {
	// Big Buck Bunny's real audio formats. Original runs with -S abr, so it
	// saves AAC 140 at 129.5 kbps — not Opus 251, which yt-dlp's default
	// ranking prefers, and which a label read from that default had named.
	raw := `{"id":"aqz-KE-bpKQ","title":"Big Buck Bunny","duration":635,"formats":[` +
		`{"format_id":"139","vcodec":"none","acodec":"mp4a.40.5","abr":48.792,"filesize":3871021},` +
		`{"format_id":"249","vcodec":"none","acodec":"opus","abr":49.561,"filesize":3931453},` +
		`{"format_id":"140-drc","vcodec":"none","acodec":"mp4a.40.2","abr":129.481,"filesize":10271496},` +
		`{"format_id":"251-drc","vcodec":"none","acodec":"opus","abr":129.327,"filesize":10258925},` +
		`{"format_id":"140","vcodec":"none","acodec":"mp4a.40.2","abr":129.481,"filesize":10271496},` +
		`{"format_id":"251","vcodec":"none","acodec":"opus","abr":128.612,"filesize":10202210},` +
		`{"format_id":"137","vcodec":"avc1.640028","acodec":"none","height":1080,"filesize":90000000}]}`
	m, err := ParseMetadata([]byte(raw), Playback{})
	if err != nil {
		t.Fatal(err)
	}
	want := AudioStream{Codec: "AAC", Kbps: 129, Bytes: 10271496}
	if m.Quality.OriginalAudio != want {
		t.Errorf("OriginalAudio = %+v, want %+v", m.Quality.OriginalAudio, want)
	}
	if got := m.Quality.AudioSizes[PickAudioOriginal]; got.Bytes != 10271496 || got.Estimate {
		t.Errorf("Original's size = %+v, want that stream's 10271496", got)
	}
}

func TestNoBitratesNoOriginalLabel(t *testing.T) {
	// archive.org states no bitrates; a label guessed from something else
	// would be worse than none.
	raw := `{"id":"x","title":"Tape","formats":[{"format_id":"a","vcodec":"none","acodec":"mp3","filesize":900}]}`
	m, err := ParseMetadata([]byte(raw), Playback{})
	if err != nil {
		t.Fatal(err)
	}
	if !m.Quality.OriginalAudio.IsZero() {
		t.Errorf("OriginalAudio = %+v with no bitrate stated", m.Quality.OriginalAudio)
	}
}

func TestOriginalOfASingleFormatSite(t *testing.T) {
	// SoundCloud and the like list audio-only formats and nothing else.
	raw := `{"id":"x","title":"Track","formats":[{"format_id":"http_mp3_128","vcodec":"none","acodec":"mp3","abr":128,"filesize":4000000}]}`
	m, err := ParseMetadata([]byte(raw), Playback{})
	if err != nil {
		t.Fatal(err)
	}
	if got := m.Quality.OriginalAudio.Label(); got != "MP3 128 kbps" {
		t.Errorf("Original = %q, want MP3 128 kbps", got)
	}
}

func TestTheFinishedFileNamesItsAudio(t *testing.T) {
	p := NewProgressParser()
	p.Line(`{"stage":"complete","path":"/x/a.mp3","width":0,"height":0,"acodec":"opus","abr":106.064}`)
	if got := p.OutputAudio().Label(); got != "Opus 106 kbps" {
		t.Errorf("OutputAudio = %q, want the stream it came from", got)
	}
	// No audio at all, or yt-dlp not saying, is nothing rather than a guess.
	q := NewProgressParser()
	q.Line(`{"stage":"complete","path":"/x/v.mp4","width":1920,"height":1080,"acodec":null,"abr":0}`)
	if !q.OutputAudio().IsZero() {
		t.Errorf("OutputAudio = %+v for a file with no audio stated", q.OutputAudio())
	}
}

func TestExecArgsAskWhichAudioArrived(t *testing.T) {
	template, _ := argValue(ExecArgs(baseOptions()), "--print")
	for _, field := range []string{`"acodec":%(acodec|null)j`, `"abr":%(abr|0)j`} {
		if !strings.Contains(template, field) {
			t.Errorf("completion template lacks %s: %s", field, template)
		}
	}
}
