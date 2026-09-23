package core

import "strings"

// Playback is what this Mac plays natively — in QuickTime, Quick Look, Photos
// and iMovie, which all decode through AVFoundation.
//
// It is a fact about the machine, so the backend fills it in (apps/desktop
// asks VideoToolbox) and core only reasons with it. A request from the
// interface never sets it.
type Playback struct {
	// AV1 is true where VideoToolbox decodes AV1 in hardware: Apple M3 and
	// later. Earlier Macs cannot play AV1 through AVFoundation at all.
	AV1 bool `json:"av1"`
}

// playableVideo lists video codec prefixes AVFoundation plays on every Mac
// Lasso runs on.
//
// Measured, not assumed: H.264 and HEVC play; MPEG-4 Part 2 plays once it is
// in an MP4 rather than an AVI (see Remuxer); VP9 plays in no container at
// all, which matters because it is how YouTube serves most 4K.
var playableVideo = []string{"avc1", "avc3", "h264", "hev1", "hvc1", "hevc", "h265", "mp4v"}

// Plays reports whether this Mac decodes a video codec natively. A codec the
// site did not name is not claimed to play.
func (p Playback) Plays(vcodec string) bool {
	c := strings.ToLower(vcodec)
	if c == "" || c == "none" {
		return false
	}
	for _, prefix := range playableVideo {
		if strings.HasPrefix(c, prefix) {
			return true
		}
	}
	return p.AV1 && strings.HasPrefix(c, "av01")
}

// selector is the yt-dlp format filter matching the codecs Plays accepts.
func (p Playback) selector() string {
	pattern := "^(avc|h26[45]|hev|hvc|mp4v"
	if p.AV1 {
		pattern += "|av01"
	}
	return "[vcodec~='" + pattern + ")']"
}
