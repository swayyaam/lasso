package core

import "testing"

// youtube4K is a trimmed version of what YouTube lists for a 4K upload: 4K in
// VP9 and AV1 only, 1080p in all three, and separate audio.
func youtube4K() []Format {
	return []Format{
		{ID: "313", Ext: "webm", Width: 3840, Height: 2160, VCodec: "vp09.00.51.08", ACodec: "none", Filesize: 900_000_000},
		{ID: "401", Ext: "mp4", Width: 3840, Height: 2160, VCodec: "av01.0.12M.08", ACodec: "none", Filesize: 700_000_000},
		{ID: "137", Ext: "mp4", Width: 1920, Height: 1080, VCodec: "avc1.640028", ACodec: "none", Filesize: 250_000_000},
		{ID: "248", Ext: "webm", Width: 1920, Height: 1080, VCodec: "vp09.00.40.08", ACodec: "none", Filesize: 200_000_000},
		{ID: "140", Ext: "m4a", VCodec: "none", ACodec: "mp4a.40.2", Filesize: 20_000_000},
	}
}

func TestBestIsTheTopTierThisMacPlays(t *testing.T) {
	// No AV1 decode (M1, M2): 4K exists but only as VP9 or AV1, so "Best" is
	// 1080p H.264 — and 4K stays on offer, marked as needing another player.
	q := AnalyseFormats(youtube4K(), Playback{})
	if q.BestLabel != "1080p" || !q.BestPlayable {
		t.Errorf("Best = %s (playable %v), want 1080p that plays", q.BestLabel, q.BestPlayable)
	}
	if q.BestBytes != 250_000_000+20_000_000 {
		t.Errorf("BestBytes = %d, want the H.264 encode plus audio", q.BestBytes)
	}
	if len(q.Tiers) == 0 || q.Tiers[0].Label != "4K" || q.Tiers[0].Playable {
		t.Errorf("tiers = %+v, want 4K first and marked as not playable here", q.Tiers)
	}

	// AV1 in hardware (M3 and later): 4K AV1 plays, so it is "Best".
	q = AnalyseFormats(youtube4K(), Playback{AV1: true})
	if q.BestLabel != "4K" || !q.BestPlayable {
		t.Errorf("Best = %s (playable %v), want 4K that plays", q.BestLabel, q.BestPlayable)
	}
	if q.BestBytes != 700_000_000+20_000_000 {
		t.Errorf("BestBytes = %d, want the AV1 encode the download will choose, not the larger VP9", q.BestBytes)
	}
}

func TestNothingPlayableFallsBackToTheTopTier(t *testing.T) {
	formats := []Format{
		{ID: "a", Width: 1280, Height: 720, VCodec: "vp9", ACodec: "none", Filesize: 10},
		{ID: "b", VCodec: "none", ACodec: "opus", Filesize: 1},
	}
	q := AnalyseFormats(formats, Playback{})
	if q.BestLabel != "720p" || q.BestPlayable {
		t.Errorf("Best = %s (playable %v), want 720p, honestly marked", q.BestLabel, q.BestPlayable)
	}
}

func TestUnstatedCodecsAreNotTreatedAsMissing(t *testing.T) {
	// archive.org's Big Buck Bunny, exactly as yt-dlp reported it: no codec
	// named for any format. Reading that as "no video, no audio" is what made
	// the quality picker vanish for these links.
	formats := []Format{
		{ID: "ogv", Ext: "ogv", Height: 300, Filesize: 46_935_223},
		{ID: "mp4", Ext: "mp4", Height: 360, Filesize: 61_878_609},
		{ID: "avi", Ext: "avi", Height: 720, Filesize: 332_243_668},
	}
	q := AnalyseFormats(formats, Playback{})
	if !q.HasVideo || !q.HasAudio {
		t.Errorf("HasVideo=%v HasAudio=%v, want both: these are whole files with sound", q.HasVideo, q.HasAudio)
	}
	if len(q.Tiers) == 0 || q.Tiers[0].Height != 720 {
		t.Errorf("tiers = %+v, want 720p on offer", q.Tiers)
	}
	// Nothing is claimed to play when the site did not say what it is.
	if q.BestPlayable {
		t.Error("an unnamed codec was claimed to play")
	}

	// And "none" still means none.
	silent := Format{VCodec: "avc1", ACodec: "none", Height: 1080}
	if silent.HasAudio() {
		t.Error(`acodec "none" was treated as audio`)
	}
	audioOnly := Format{VCodec: "none", ACodec: "mp4a"}
	if audioOnly.HasVideo() {
		t.Error(`vcodec "none" was treated as video`)
	}
}

func TestPlaysMatchesWhatWasMeasured(t *testing.T) {
	for codec, want := range map[string]bool{
		"avc1.640028": true, "hvc1.2.4.L153": true, "hev1": true, "mp4v.20.9": true,
		"vp09.00.51.08": false, "vp9": false, "vp8": false, "av01.0.12M.08": false,
		"": false, "none": false,
	} {
		if got := (Playback{}).Plays(codec); got != want {
			t.Errorf("Plays(%q) without AV1 = %v, want %v", codec, got, want)
		}
	}
	if !(Playback{AV1: true}).Plays("av01.0.12M.08") {
		t.Error("AV1 did not play on a Mac that decodes it")
	}
}

func TestAVideoBelowTheLadderStillKnowsItPlays(t *testing.T) {
	// "Me at the zoo", as YouTube lists it: 240p at most, in H.264 among
	// others. With no rung to hang it on, Best used to report that nothing
	// here plays — and the Video card said it needed IINA or VLC.
	formats := []Format{
		{ID: "133", Ext: "mp4", Width: 320, Height: 240, VCodec: "avc1.4d400c", ACodec: "none", Filesize: 300_000},
		{ID: "242", Ext: "webm", Width: 320, Height: 240, VCodec: "vp9", ACodec: "none", Filesize: 250_000},
		{ID: "140", Ext: "m4a", VCodec: "none", ACodec: "mp4a.40.2", Filesize: 50_000},
	}
	q := AnalyseFormats(formats, Playback{})
	if len(q.Tiers) != 0 {
		t.Fatalf("tiers = %+v, want none below the ladder", q.Tiers)
	}
	if q.BestLabel != "240p" || !q.BestPlayable {
		t.Errorf("Best = %s (playable %v), want 240p that plays", q.BestLabel, q.BestPlayable)
	}
	if q.BestBytes != 350_000 {
		t.Errorf("BestBytes = %d, want the H.264 stream plus audio", q.BestBytes)
	}

	// And when nothing down there plays, it still says so.
	q = AnalyseFormats(formats[1:], Playback{})
	if q.BestPlayable {
		t.Error("a VP9-only 240p video does not play natively")
	}
}
