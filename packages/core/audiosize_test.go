package core

import "testing"

func TestEachAudioPickGetsItsOwnSize(t *testing.T) {
	// "Me at the zoo" as YouTube lists its audio: 19 seconds, AAC and Opus.
	formats := []Format{
		{ID: "140", Ext: "m4a", VCodec: "none", ACodec: "mp4a.40.2", Filesize: 309_000},
		{ID: "251", Ext: "webm", VCodec: "none", ACodec: "opus", Filesize: 290_000},
		{ID: "18", Ext: "mp4", Width: 320, Height: 240, VCodec: "avc1", ACodec: "mp4a", Filesize: 800_000},
	}
	sizes := AudioSizes(formats, 19)

	exact := map[QuickPick]int64{PickAudioOriginal: 309_000, PickAudioM4A: 309_000, PickAudioOpus: 290_000}
	for pick, want := range exact {
		if got := sizes[pick]; got.Bytes != want || got.Estimate {
			t.Errorf("%s = %+v, want exactly %d — it is a stream the site stated", pick, got, want)
		}
	}
	// The re-encodes: not the source's size, and marked as estimates.
	if got := sizes[PickAudioFLAC]; !got.Estimate || got.Bytes <= 309_000 {
		t.Errorf("FLAC = %+v, want an estimate larger than the lossy source it re-encodes", got)
	}
	if got := sizes[PickAudioMP3]; !got.Estimate || got.Bytes != 19*mp3Kbps*1000/8 {
		t.Errorf("MP3 = %+v, want %d estimated from the bitrate", got, 19*mp3Kbps*1000/8)
	}
	if sizes[PickAudioFLAC].Bytes <= sizes[PickAudioMP3].Bytes {
		t.Error("FLAC came out smaller than MP3, which it never is")
	}
}

func TestNoLengthMeansNoGuess(t *testing.T) {
	sizes := AudioSizes([]Format{{Ext: "m4a", VCodec: "none", ACodec: "mp4a", Filesize: 5}}, 0)
	if sizes[PickAudioFLAC].Bytes != 0 || sizes[PickAudioMP3].Bytes != 0 {
		t.Error("a size was invented for a length nobody stated")
	}
	if sizes[PickAudioM4A].Bytes != 5 {
		t.Error("an exact size was dropped along with the guesses")
	}
}

func TestMissingStreamsFallBackToEstimates(t *testing.T) {
	// Only Opus on offer: M4A has to re-encode, so it becomes an estimate.
	sizes := AudioSizes([]Format{{Ext: "webm", VCodec: "none", ACodec: "opus", Filesize: 1000}}, 60)
	if got := sizes[PickAudioM4A]; !got.Estimate || got.Bytes == 0 {
		t.Errorf("M4A = %+v, want an estimate when there is no AAC stream to remux", got)
	}
}
