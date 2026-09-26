package core

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Clip is part of a video, by time. The zero Clip is the whole thing.
//
// yt-dlp downloads only that part (--download-sections), and
// --force-keyframes-at-cuts re-encodes around each edge so the cut lands where
// asked rather than at the nearest keyframe, which can be seconds away. That
// is what makes a clip slower than its size suggests. The download itself is
// ffmpeg's: it fetches the section over https, which is why the helper
// environment carries SSL_CERT_FILE (binaries.Manager.Environ).
type Clip struct {
	// Start and End are seconds from the beginning. End 0 means to the end.
	Start float64 `json:"start"`
	End   float64 `json:"end"`
}

// IsWhole reports whether this is no clip at all.
func (c Clip) IsWhole() bool { return c.Start <= 0 && c.End <= 0 }

// Validate refuses a clip that cannot be cut.
func (c Clip) Validate() error {
	switch {
	case c.IsWhole():
		return nil
	case c.Start < 0 || c.End < 0 || math.IsNaN(c.Start) || math.IsNaN(c.End) || math.IsInf(c.Start, 0) || math.IsInf(c.End, 0):
		return errors.New("a clip's times cannot be negative")
	case c.End > 0 && c.End <= c.Start:
		return errors.New("the clip ends before it starts")
	}
	return nil
}

// section is the --download-sections value: "*60-150", or "*60-inf" to the end.
func (c Clip) section() string {
	end := "inf"
	if c.End > 0 {
		end = seconds(c.End)
	}
	return "*" + seconds(c.Start) + "-" + end
}

// fileLabel names the clip in a filename: "clip 1m00s-2m30s". Without it a
// clip had the full video's name, and yt-dlp, finding that file, would skip
// the clip as already downloaded. No colons: Finder shows them as slashes.
func (c Clip) fileLabel() string {
	end := "end"
	if c.End > 0 {
		end = compactTime(c.End)
	}
	return "clip " + compactTime(c.Start) + "-" + end
}

// Label is the clip as a person reads it: "1:00–2:30", or "1:00 to the end".
func (c Clip) Label() string {
	if c.End <= 0 {
		return ClockTime(c.Start) + " to the end"
	}
	return ClockTime(c.Start) + "–" + ClockTime(c.End)
}

func seconds(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }

// ClockTime is "1:05", "1:02:03", or "1:05.5" when the time is not whole.
func ClockTime(v float64) string {
	whole := int(v)
	h, m, s := whole/3600, whole%3600/60, whole%60
	text := fmt.Sprintf("%d:%02d", m, s)
	if h > 0 {
		text = fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	if frac := v - float64(whole); frac >= 0.05 {
		text += strings.TrimPrefix(strconv.FormatFloat(frac, 'f', 1, 64), "0")
	}
	return text
}

// compactTime is "1m05s", "1h02m03s", or "1m05.5s".
func compactTime(v float64) string {
	whole := int(v)
	h, m, s := whole/3600, whole%3600/60, whole%60
	sec := fmt.Sprintf("%02d", s)
	if frac := v - float64(whole); frac >= 0.05 {
		sec += strings.TrimPrefix(strconv.FormatFloat(frac, 'f', 1, 64), "0")
	}
	if h > 0 {
		return fmt.Sprintf("%dh%02dm%ss", h, m, sec)
	}
	return fmt.Sprintf("%dm%ss", m, sec)
}

// clipArgs cuts the download down to the clip.
func clipArgs(o Options) []string {
	if o.Clip.IsWhole() {
		return nil
	}
	return []string{"--download-sections", o.Clip.section(), "--force-keyframes-at-cuts"}
}
