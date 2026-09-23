package core

import "strings"

// AudioSize is the estimated file an audio pick produces.
type AudioSize struct {
	Bytes int64 `json:"bytes"`
	// Estimate is true when the file is re-encoded, so its size comes from
	// the encoder's bitrate and the length rather than from a stream the site
	// stated. The interface marks those with "about".
	Estimate bool `json:"estimate"`
}

// Nominal bitrates of the re-encodes Lasso asks for, in kbit/s.
//
// Variable-bitrate encoders land wherever the audio takes them, so these are
// what each settles on for typical music at the quality Lasso requests, not
// promises: LAME's V0 (--audio-quality 0) averages about 245; FLAC keeps
// every sample of the decoded audio, so it costs roughly 60% of 48 kHz 16-bit
// stereo PCM — which is why it is several times the size of the source it was
// made from while sounding identical.
const (
	mp3Kbps  = 245
	aacKbps  = 256
	opusKbps = 160
	flacKbps = 920
)

// AudioSizes estimates what each audio pick would produce.
//
// Before this every audio option showed the source stream's size, so FLAC
// claimed to be as small as the lossy stream it re-encodes. Each pick is a
// different file and gets its own number: the original is the source stream;
// M4A and Opus are exact when the site has a matching stream to remux;
// everything else is re-encoded and estimated. Without a duration nothing is
// estimated — a number that is quietly wrong is worse than none.
func AudioSizes(formats []Format, duration float64) map[QuickPick]AudioSize {
	var best, m4a, opus int64
	for _, f := range formats {
		if f.IsStoryboard() || f.HasVideo() || !f.HasAudio() {
			continue
		}
		size := f.Size()
		if size > best {
			best = size
		}
		codec := strings.ToLower(f.ACodec)
		if (f.Ext == "m4a" || strings.HasPrefix(codec, "mp4a")) && size > m4a {
			m4a = size
		}
		if strings.HasPrefix(codec, "opus") && size > opus {
			opus = size
		}
	}

	encoded := func(kbps int) AudioSize {
		if duration <= 0 {
			return AudioSize{}
		}
		return AudioSize{Bytes: int64(duration * float64(kbps) * 1000 / 8), Estimate: true}
	}
	exactOr := func(size int64, kbps int) AudioSize {
		if size > 0 {
			return AudioSize{Bytes: size}
		}
		return encoded(kbps)
	}

	return map[QuickPick]AudioSize{
		PickAudioOriginal: {Bytes: best},
		PickAudioM4A:      exactOr(m4a, aacKbps),
		PickAudioOpus:     exactOr(opus, opusKbps),
		PickAudioMP3:      encoded(mp3Kbps),
		PickAudioFLAC:     encoded(flacKbps),
	}
}
