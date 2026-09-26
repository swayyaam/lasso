package core

import (
	"fmt"
	"math"
	"strings"
)

// AudioStream is an audio stream as a person would name it: "Opus 129 kbps".
type AudioStream struct {
	// Codec is the codec's usual name: Opus, AAC, MP3, FLAC.
	Codec string `json:"codec"`
	// Kbps is the average bitrate, 0 when the site did not say.
	Kbps int `json:"kbps"`
	// Bytes is the stream's size, 0 when unknown.
	Bytes int64 `json:"bytes"`
}

// IsZero reports whether nothing is known about the stream.
func (a AudioStream) IsZero() bool { return a.Codec == "" }

// Label is "Opus 129 kbps", "MP3" without a bitrate, or "".
func (a AudioStream) Label() string {
	if a.Codec == "" {
		return ""
	}
	if a.Kbps > 0 {
		return fmt.Sprintf("%s %d kbps", a.Codec, a.Kbps)
	}
	return a.Codec
}

// CodecName turns yt-dlp's acodec into the name people know it by. yt-dlp
// says "mp4a.40.2" for AAC and "none" for no audio at all.
func CodecName(acodec string) string {
	c := strings.ToLower(strings.TrimSpace(acodec))
	switch {
	case c == "" || c == "none":
		return ""
	case strings.HasPrefix(c, "opus"):
		return "Opus"
	case strings.HasPrefix(c, "mp4a"), c == "aac":
		return "AAC"
	case strings.HasPrefix(c, "mp3"):
		return "MP3"
	case strings.HasPrefix(c, "vorbis"):
		return "Vorbis"
	case strings.HasPrefix(c, "flac"):
		return "FLAC"
	case strings.HasPrefix(c, "alac"):
		return "ALAC"
	case c == "ac-3" || c == "ac3":
		return "AC-3"
	case c == "ec-3" || c == "eac3":
		return "E-AC-3"
	case strings.HasPrefix(c, "pcm") || c == "wav":
		return "PCM"
	}
	return strings.ToUpper(c)
}

func audioStream(acodec string, abr float64, bytes int64) AudioStream {
	name := CodecName(acodec)
	if name == "" {
		return AudioStream{}
	}
	return AudioStream{Codec: name, Kbps: int(math.Round(abr)), Bytes: bytes}
}

// originalAudio is the stream "Original" saves: the audio-only format with the
// highest bitrate, because every audio pick runs with -S abr. Not yt-dlp's
// default choice, which ranks codec before bitrate — on YouTube that is Opus
// 106 kbps, while Original saves AAC 130; a label read from it named a stream
// the download never used (seen in the running app). Ties go to the larger
// file, and a site that states no bitrates gets no label rather than a guess.
func originalAudio(formats []rawFormat) AudioStream {
	var best rawFormat
	found := false
	for _, f := range formats {
		if f.VCodec != "none" || f.ACodec == "" || f.ACodec == "none" {
			continue
		}
		abr, size := deref(f.ABR), max(deref(f.Filesize), deref(f.FilesizeApprox))
		bestABR, bestSize := deref(best.ABR), max(deref(best.Filesize), deref(best.FilesizeApprox))
		if !found || abr > bestABR || (abr == bestABR && size > bestSize) {
			best, found = f, true
		}
	}
	if !found || deref(best.ABR) <= 0 {
		return AudioStream{}
	}
	return audioStream(best.ACodec, deref(best.ABR), max(deref(best.Filesize), deref(best.FilesizeApprox)))
}
