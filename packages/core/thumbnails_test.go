package core

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// testImage builds a solid JPEG of the given size.
func testImage(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x % 256), G: uint8(y % 256), B: 128, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// newTestCache returns a cache pointed at a test server, with the private-network
// guard relaxed so httptest's loopback address is reachable.
func newTestCache(t *testing.T) (*ThumbnailCache, string) {
	t.Helper()
	dir := t.TempDir()
	c := NewThumbnailCache(dir, DefaultThumbnailCacheBytes)
	c.client = &http.Client{Timeout: 5 * time.Second}
	return c, dir
}

// getForTest skips only the https requirement, so tests can use httptest's
// plain-http server while still going through caching, in-flight collapsing,
// fetching, decoding, scaling, storing and eviction.
func (c *ThumbnailCache) getForTest(ctx context.Context, url string, width int) (string, error) {
	return c.get(ctx, url, width)
}

func decodeFile(t *testing.T, path string) image.Image {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	return img
}

func TestThumbnailDownscalesToRequestedWidth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write(testImage(t, 1280, 720))
	}))
	defer server.Close()

	c, _ := newTestCache(t)
	path, err := c.getForTest(context.Background(), server.URL+"/thumb.jpg", 320)
	if err != nil {
		t.Fatalf("get: %v", err)
	}

	img := decodeFile(t, path)
	if got := img.Bounds().Dx(); got != 320 {
		t.Errorf("width = %d, want 320", got)
	}
	// 1280x720 scaled to 320 wide should stay 16:9.
	if got := img.Bounds().Dy(); got != 180 {
		t.Errorf("height = %d, want 180 for a preserved aspect ratio", got)
	}
}

func TestThumbnailNeverUpscales(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write(testImage(t, 120, 90))
	}))
	defer server.Close()

	c, _ := newTestCache(t)
	path, err := c.getForTest(context.Background(), server.URL+"/small.jpg", 640)
	if err != nil {
		t.Fatalf("get: %v", err)
	}

	// Blowing a 120px preview up to 640 only makes it blurrier and bigger.
	if got := decodeFile(t, path).Bounds().Dx(); got != 120 {
		t.Errorf("width = %d, want the original 120", got)
	}
}

func TestThumbnailIsFetchedOnlyOnce(t *testing.T) {
	var requests int
	var mu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		requests++
		mu.Unlock()
		w.Write(testImage(t, 640, 360))
	}))
	defer server.Close()

	c, _ := newTestCache(t)
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		if _, err := c.getForTest(ctx, server.URL+"/x.jpg", 320); err != nil {
			t.Fatalf("get: %v", err)
		}
	}

	mu.Lock()
	defer mu.Unlock()
	if requests != 1 {
		t.Errorf("made %d requests, want 1: the cache is not being reused", requests)
	}
}

func TestThumbnailCachesPerWidth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write(testImage(t, 1280, 720))
	}))
	defer server.Close()

	c, _ := newTestCache(t)
	ctx := context.Background()

	small, _ := c.getForTest(ctx, server.URL+"/x.jpg", 160)
	large, _ := c.getForTest(ctx, server.URL+"/x.jpg", 480)

	if small == large {
		t.Fatal("different widths shared one cache entry")
	}
	if got := decodeFile(t, small).Bounds().Dx(); got != 160 {
		t.Errorf("small width = %d", got)
	}
	if got := decodeFile(t, large).Bounds().Dx(); got != 480 {
		t.Errorf("large width = %d", got)
	}
}

func TestThumbnailAcceptsPNG(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		img := image.NewRGBA(image.Rect(0, 0, 400, 400))
		png.Encode(w, img)
	}))
	defer server.Close()

	c, _ := newTestCache(t)
	if _, err := c.getForTest(context.Background(), server.URL+"/x.png", 200); err != nil {
		t.Fatalf("PNG rejected: %v", err)
	}
}

func TestThumbnailRejectsUnsafeURLs(t *testing.T) {
	c, _ := newTestCache(t)
	ctx := context.Background()

	// Metadata is ultimately controlled by the site being downloaded from, so
	// these must never turn into a request.
	unsafe := []string{
		"http://example.com/x.jpg",
		"file:///etc/passwd",
		"ftp://example.com/x.jpg",
		"https://",
		"javascript:alert(1)",
	}
	for _, raw := range unsafe {
		if _, err := c.Get(ctx, raw, 320); !errors.Is(err, ErrUnsafeThumbnailURL) {
			t.Errorf("Get(%q) error = %v, want ErrUnsafeThumbnailURL", raw, err)
		}
	}
}

func TestThumbnailRefusesPrivateAddresses(t *testing.T) {
	// A real cache keeps its restricted dialer, which rejects the loopback
	// address httptest binds to even though the URL looks ordinary.
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write(testImage(t, 100, 100))
	}))
	defer server.Close()

	c := NewThumbnailCache(t.TempDir(), DefaultThumbnailCacheBytes)
	_, err := c.Get(context.Background(), server.URL+"/x.jpg", 100)
	if err == nil {
		t.Fatal("fetched an image from a loopback address")
	}
	if !strings.Contains(err.Error(), "preview image") && !errors.Is(err, ErrUnsafeThumbnailURL) {
		t.Errorf("error = %v, want it to reflect a blocked address", err)
	}
}

func TestThumbnailRejectsNonImages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("<html>not an image</html>"))
	}))
	defer server.Close()

	c, _ := newTestCache(t)
	if _, err := c.getForTest(context.Background(), server.URL+"/x.jpg", 320); err == nil {
		t.Error("accepted a response that was not an image")
	}
}

func TestThumbnailReportsServerErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "gone", http.StatusNotFound)
	}))
	defer server.Close()

	c, _ := newTestCache(t)
	_, err := c.getForTest(context.Background(), server.URL+"/x.jpg", 320)
	if err == nil {
		t.Fatal("a 404 was treated as success")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("error = %v, want it to name the status", err)
	}
}

func TestThumbnailRejectsZeroWidth(t *testing.T) {
	c, _ := newTestCache(t)
	if _, err := c.Get(context.Background(), "https://example.com/x.jpg", 0); err == nil {
		t.Error("accepted a zero width")
	}
}

func TestThumbnailEvictsLeastRecentlyUsed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(testImage(t, 800, 600))
	}))
	defer server.Close()

	dir := t.TempDir()
	// A budget small enough that a handful of images overflows it.
	c := NewThumbnailCache(dir, 12*1024)
	c.client = &http.Client{Timeout: 5 * time.Second}
	ctx := context.Background()

	var first string
	for i := 0; i < 8; i++ {
		path, err := c.getForTest(ctx, server.URL+"/"+string(rune('a'+i))+".jpg", 400)
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if i == 0 {
			first = path
		}
		// Keep modification times distinct so ordering is unambiguous.
		time.Sleep(10 * time.Millisecond)
	}

	if got := c.Size(); got > 12*1024 {
		t.Errorf("cache is %d bytes, over its %d budget", got, 12*1024)
	}
	if _, err := os.Stat(first); err == nil {
		t.Error("the oldest entry survived eviction")
	}
}

func TestThumbnailClear(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write(testImage(t, 200, 200))
	}))
	defer server.Close()

	c, dir := newTestCache(t)
	if _, err := c.getForTest(context.Background(), server.URL+"/x.jpg", 100); err != nil {
		t.Fatal(err)
	}
	if c.Size() == 0 {
		t.Fatal("nothing was cached")
	}

	if err := c.Clear(); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Error("Clear left the cache directory behind")
	}
}

func TestThumbnailWritesAreAtomic(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write(testImage(t, 400, 300))
	}))
	defer server.Close()

	c, dir := newTestCache(t)
	if _, err := c.getForTest(context.Background(), server.URL+"/x.jpg", 200); err != nil {
		t.Fatal(err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".thumb-") {
			t.Errorf("temporary file %s was left behind", e.Name())
		}
	}
}

func TestThumbnailConcurrentRequestsCollapse(t *testing.T) {
	var mu sync.Mutex
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		requests++
		mu.Unlock()
		time.Sleep(40 * time.Millisecond)
		w.Write(testImage(t, 600, 400))
	}))
	defer server.Close()

	c, _ := newTestCache(t)
	url := server.URL + "/same.jpg"

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.getForTest(context.Background(), url, 300)
		}()
	}
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	// Ten simultaneous requests for one image must become one download, which
	// is what a playlist of videos from the same uploader produces.
	if requests != 1 {
		t.Errorf("made %d requests, want 1: concurrent requests did not collapse", requests)
	}
}

func TestScaleToWidth(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 1000, 500))

	got := scaleToWidth(src, 250)
	if got.Bounds().Dx() != 250 || got.Bounds().Dy() != 125 {
		t.Errorf("scaled to %v, want 250x125", got.Bounds())
	}

	// Very wide images must not round to a zero-height result.
	wide := image.NewRGBA(image.Rect(0, 0, 10000, 3))
	if got := scaleToWidth(wide, 10); got.Bounds().Dy() < 1 {
		t.Errorf("height collapsed to %d", got.Bounds().Dy())
	}
}

func TestPathForIsStableAndDistinct(t *testing.T) {
	c := NewThumbnailCache(filepath.Join("x"), 0)

	a := c.pathFor("https://example.com/a.jpg", 320)
	if a != c.pathFor("https://example.com/a.jpg", 320) {
		t.Error("the same request produced two paths")
	}
	if a == c.pathFor("https://example.com/b.jpg", 320) {
		t.Error("different URLs collided")
	}
	if a == c.pathFor("https://example.com/a.jpg", 640) {
		t.Error("different widths collided")
	}
}
