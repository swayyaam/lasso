package core

// LiveStatus is yt-dlp's word for where a video is in a broadcast's life.
type LiveStatus string

const (
	LiveNone     LiveStatus = ""
	LiveNow      LiveStatus = "is_live"
	LiveUpcoming LiveStatus = "is_upcoming"
	// LiveJustEnded is a stream that has finished but whose recording the
	// site is still preparing.
	LiveJustEnded LiveStatus = "post_live"
	// LiveWas is a finished stream with its recording ready: an ordinary
	// video, as far as downloading goes.
	LiveWas LiveStatus = "was_live"
)

// Unavailable says why a video cannot be downloaded yet, in words the
// interface shows as they are, or "" when it can.
//
// A live stream is the case this exists for. Handed to yt-dlp it records until
// the stream ends — which for a 24/7 stream is never — so it fills the disk,
// never finishes, and shows a progress bar with no total. And nothing yt-dlp
// reports tells a stream that will end from one that will not: a live stream
// has no duration either way. So Lasso downloads the recording once the
// stream is over, which yt-dlp handles like any other video, and says so.
func (m Metadata) Unavailable() string {
	switch m.Live {
	case LiveNow:
		return "This is live right now. Lasso downloads the recording once the stream has ended — come back then."
	case LiveUpcoming:
		return "This stream hasn't started yet. Once it has been and gone, its recording can be downloaded."
	case LiveJustEnded:
		return "This stream has just ended and the site is still preparing its recording. Try again in a little while."
	default:
		return ""
	}
}
