package core

import (
	"bytes"
	"context"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/png"
	"net"
	"net/http"
	"net/http/httptest"
	"runtime"
	"slices"
	"strings"
	"testing"
)

// pngClaiming returns a valid tiny PNG whose header claims other dimensions.
// Reading the header is all a decoder needs to decide how much to allocate,
// which is exactly the attack: a few hundred bytes asking for gigabytes.
func pngClaiming(t *testing.T, width, height uint32) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewGray(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	b := buf.Bytes()
	// Signature (8) + length (4) + "IHDR" (4), then width and height.
	binary.BigEndian.PutUint32(b[16:], width)
	binary.BigEndian.PutUint32(b[20:], height)
	// The CRC covers the chunk type and data, and sits right after them.
	binary.BigEndian.PutUint32(b[29:], crc32.ChecksumIEEE(b[12:29]))
	return b
}

func TestThumbnailRefusesAnImageBombBeforeAllocating(t *testing.T) {
	bomb := pngClaiming(t, 60000, 60000) // 3.6 gigapixels
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write(bomb)
	}))
	defer server.Close()
	c, _ := newTestCache(t)

	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	_, err := c.getForTest(context.Background(), server.URL+"/bomb.png", 320)
	runtime.ReadMemStats(&after)

	if err == nil {
		t.Fatal("a 60000×60000 image was accepted as a thumbnail")
	}
	if !strings.Contains(err.Error(), "60000×60000") {
		t.Errorf("err = %v, want it to name the dimensions it refused", err)
	}
	if grew := after.TotalAlloc - before.TotalAlloc; grew > 32<<20 {
		t.Errorf("refusing the bomb allocated %d MB; it should be refused from the header alone", grew>>20)
	}
}

func TestThumbnailStillAcceptsARealSizedImage(t *testing.T) {
	// The limit must not catch the images it exists to show: 4K is the
	// largest preview anyone serves.
	var buf bytes.Buffer
	png.Encode(&buf, image.NewGray(image.Rect(0, 0, 3840, 2160)))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.Write(buf.Bytes()) }))
	defer server.Close()
	c, _ := newTestCache(t)
	if _, err := c.getForTest(context.Background(), server.URL+"/4k.png", 320); err != nil {
		t.Errorf("a 4K preview was refused: %v", err)
	}
}

func TestIsPublicIPRefusesEveryNonPublicRange(t *testing.T) {
	for _, s := range []string{
		"127.0.0.1", "10.1.2.3", "172.16.0.1", "192.168.1.1", "169.254.169.254",
		"100.64.0.1", "100.100.100.100", // CGNAT: Tailscale peers and MagicDNS
		"0.0.0.1", "198.18.0.1", "192.0.2.1", "203.0.113.9", "240.0.0.1", "255.255.255.255", "224.0.0.1",
		"::1", "::", "fe80::1", "fc00::1", "fd12:3456::1", "ff02::1",
		"::ffff:127.0.0.1", "::ffff:10.0.0.1", // IPv4 in IPv6 clothing
		"64:ff9b::7f00:1",     // NAT64 wrapping 127.0.0.1
		"2002:7f00:1::1",      // 6to4 wrapping 127.0.0.1
		"2001:0:4136:e378::1", // Teredo
		"2001:db8::1",
	} {
		if isPublicIP(net.ParseIP(s)) {
			t.Errorf("%s was treated as public", s)
		}
	}
	for _, s := range []string{"8.8.8.8", "1.1.1.1", "142.250.72.14", "2606:4700:4700::1111", "2a00:1450:4009:81f::200e"} {
		if !isPublicIP(net.ParseIP(s)) {
			t.Errorf("%s, a public address, was refused", s)
		}
	}
}

func TestEveryInvocationClearsPluginFolders(t *testing.T) {
	// --ignore-config leaves yt-dlp's default plugin folders live, so a plugin
	// installed for command-line use would run inside Lasso. Every way Lasso
	// starts yt-dlp has to switch them off.
	o := Options{URL: "https://example.com/v", Pick: PickBest}
	for name, args := range map[string][]string{
		"download": BuildArgs(o),
		"metadata": MetadataArgs(o),
		"cookies":  CookieProbeArgs(o),
	} {
		i := slices.Index(args, "--no-plugin-dirs")
		if i < 0 {
			t.Errorf("%s args do not pass --no-plugin-dirs: %v", name, args)
			continue
		}
		// It clears earlier --plugin-dirs too, so it must come before any.
		if j := slices.Index(args, "--plugin-dirs"); j >= 0 && j < i {
			t.Errorf("%s args pass --plugin-dirs before --no-plugin-dirs, which would clear it", name)
		}
	}
}
