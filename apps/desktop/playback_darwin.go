package main

/*
#cgo LDFLAGS: -framework VideoToolbox -framework CoreMedia
#include <VideoToolbox/VideoToolbox.h>

// Hardware decode is what AVFoundation — and so QuickTime, Quick Look and
// Photos — plays AV1 with. There is no software fallback, so a Mac without it
// cannot play AV1 in any of them.
static int lassoDecodesAV1(void) {
	return VTIsHardwareDecodeSupported(kCMVideoCodecType_AV1) ? 1 : 0;
}
*/
import "C"

import "github.com/swayyaam/lasso/packages/core"

// detectPlayback asks this Mac what it plays natively.
//
// Asked rather than inferred from the chip name: VideoToolbox is what
// AVFoundation itself consults, so its answer is the one that decides whether
// a file opens. Measured on an M5 — H.264, HEVC and AV1 in hardware, VP9 not —
// and matches Apple's M3-and-later line for AV1.
func detectPlayback() core.Playback {
	return core.Playback{AV1: C.lassoDecodesAV1() == 1}
}
