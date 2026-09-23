package core

import (
	"strings"
	"testing"
)

func TestLiveStreamsAreRecognisedAndExplained(t *testing.T) {
	cases := []struct {
		json    string
		live    LiveStatus
		blocked string
	}{
		{`{"id":"a","title":"24/7 lofi","live_status":"is_live","is_live":true}`, LiveNow, "live right now"},
		{`{"id":"b","title":"Launch","live_status":"is_upcoming"}`, LiveUpcoming, "hasn't started"},
		{`{"id":"c","title":"Stream","live_status":"post_live"}`, LiveJustEnded, "still preparing"},
		// A finished stream with its recording ready is just a video.
		{`{"id":"d","title":"Yesterday's stream","live_status":"was_live"}`, LiveWas, ""},
		{`{"id":"e","title":"Plain video","live_status":"not_live"}`, "not_live", ""},
		// Extractors that only say is_live.
		{`{"id":"f","title":"Old-style","is_live":true}`, LiveNow, "live right now"},
	}
	for _, tc := range cases {
		m, err := ParseMetadata([]byte(tc.json), Playback{})
		if err != nil {
			t.Fatalf("ParseMetadata: %v", err)
		}
		if m.Live != tc.live {
			t.Errorf("%s: Live = %q, want %q", m.Title, m.Live, tc.live)
		}
		if tc.blocked == "" && m.Blocked != "" {
			t.Errorf("%s: blocked with %q, want it downloadable", m.Title, m.Blocked)
		}
		if tc.blocked != "" && !strings.Contains(m.Blocked, tc.blocked) {
			t.Errorf("%s: Blocked = %q, want it to say %q", m.Title, m.Blocked, tc.blocked)
		}
	}
}
