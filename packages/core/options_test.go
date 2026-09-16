package core

import (
	"strings"
	"testing"
)

func TestValidateAcceptsReasonableOptions(t *testing.T) {
	o := baseOptions()
	o.Output = Output{Folder: "/Users/x/Movies", Template: "%(uploader)s/%(title)s.%(ext)s"}
	o.Network = Network{RateLimit: "2.5M"}
	o.Playlist = Playlist{Start: 1, End: 5}

	if err := o.Validate(); err != nil {
		t.Errorf("Validate: %v", err)
	}
}

func TestValidateRejectsBadInput(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*Options)
		wantSub string
	}{
		{"empty url", func(o *Options) { o.URL = "" }, "no URL"},
		{"whitespace url", func(o *Options) { o.URL = "   " }, "no URL"},
		// A downloader must not be talked into reading local files.
		{"file scheme", func(o *Options) { o.URL = "file:///etc/passwd" }, "http and https"},
		{"javascript scheme", func(o *Options) { o.URL = "javascript:alert(1)" }, "http and https"},
		{"no host", func(o *Options) { o.URL = "https://" }, "website address"},
		// A template must not be able to walk out of the download folder.
		{"template escapes folder", func(o *Options) { o.Output.Template = "../../%(title)s.%(ext)s" }, ".."},
		{"template absolute", func(o *Options) { o.Output.Template = "/etc/%(title)s" }, "absolute path"},
		{"bad rate limit", func(o *Options) { o.Network.RateLimit = "fast" }, "500K or 2M"},
		{"negative playlist", func(o *Options) { o.Playlist = Playlist{Start: -1} }, "negative"},
		{"inverted range", func(o *Options) { o.Playlist = Playlist{Start: 9, End: 2} }, "ends before it starts"},
		{"arc without profile", func(o *Options) { o.Network.Cookies = BrowserArc }, "Arc's profile"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			o := baseOptions()
			tc.mutate(&o)

			err := o.Validate()
			if err == nil {
				t.Fatal("Validate accepted invalid options")
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Errorf("error = %q, want it to mention %q", err, tc.wantSub)
			}
		})
	}
}

func TestValidateAcceptsRateLimitForms(t *testing.T) {
	for _, rate := range []string{"", "500K", "2M", "1.5M", "1G", "1000"} {
		o := baseOptions()
		o.Network.RateLimit = rate
		if err := o.Validate(); err != nil {
			t.Errorf("rate %q rejected: %v", rate, err)
		}
	}
}

func TestQuickPickClassification(t *testing.T) {
	audio := []QuickPick{PickAudioMP3, PickAudioM4A, PickAudioFLAC, PickAudioOpus}
	video := []QuickPick{PickBest, Pick2160p, Pick1440p, Pick1080p, Pick720p}

	for _, p := range audio {
		if !p.IsAudioOnly() {
			t.Errorf("%s should be audio-only", p)
		}
		if p.maxHeight() != 0 {
			t.Errorf("%s should not cap height", p)
		}
	}
	for _, p := range video {
		if p.IsAudioOnly() {
			t.Errorf("%s should not be audio-only", p)
		}
	}

	heights := map[QuickPick]int{Pick2160p: 2160, Pick1440p: 1440, Pick1080p: 1080, Pick720p: 720, PickBest: 0}
	for pick, want := range heights {
		if got := pick.maxHeight(); got != want {
			t.Errorf("%s maxHeight = %d, want %d", pick, got, want)
		}
	}
}
