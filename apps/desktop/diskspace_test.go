package main

import (
	"path/filepath"
	"testing"
)

func TestFreeBytesReportsSomethingReal(t *testing.T) {
	free, err := freeBytes(t.TempDir())
	if err != nil {
		t.Fatalf("freeBytes: %v", err)
	}
	if free <= 0 {
		t.Errorf("freeBytes = %d, want the volume to report space", free)
	}
}

func TestCheckDiskSpacePassesOnANormalVolume(t *testing.T) {
	// The floor is a guard against a full disk, not a gate on ordinary use: a
	// working machine must never be refused.
	if err := checkDiskSpace(t.TempDir()); err != nil {
		t.Errorf("checkDiskSpace on a working volume: %v", err)
	}
}

func TestCheckDiskSpaceIgnoresWhatItCannotMeasure(t *testing.T) {
	// An unreadable destination is the download's problem to report, with a
	// better error than a space check could give.
	missing := filepath.Join(t.TempDir(), "does", "not", "exist")
	if err := checkDiskSpace(missing); err != nil {
		t.Errorf("checkDiskSpace on an unmeasurable path = %v, want it to defer", err)
	}
	if err := checkDiskSpace(""); err != nil {
		t.Errorf("checkDiskSpace on no path = %v, want it to defer", err)
	}
}

func TestFormatBytes(t *testing.T) {
	cases := []struct {
		bytes int64
		want  string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1500, "1.5 KB"},
		{1_500_000, "1.5 MB"},
		{2_000_000_000, "2.0 GB"},
	}
	for _, c := range cases {
		if got := formatBytes(c.bytes); got != c.want {
			t.Errorf("formatBytes(%d) = %q, want %q", c.bytes, got, c.want)
		}
	}
}
