package core

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"syscall"
	"time"

	"golang.org/x/image/draw"
	// YouTube serves most of its sized thumbnails as WebP, which the standard
	// library cannot decode. Without this they fail as "unknown format" and the
	// preview silently falls back to a blank frame.
	_ "golang.org/x/image/webp"
)

// Thumbnail fetching is the only outbound network Lasso performs besides
// yt-dlp's own traffic and the binary updater. It is kept in the backend rather
// than letting the webview load remote images so that there is exactly one
// place to audit, so images are decoded once at the size they are shown, and so
// a cached image costs nothing to redisplay.

const (
	// DefaultThumbnailCacheBytes caps the cache. Thumbnails are a few KB each
	// once downscaled, so this holds thousands.
	DefaultThumbnailCacheBytes = 64 << 20

	// maxThumbnailBytes bounds what will be downloaded before decoding.
	maxThumbnailBytes = 10 << 20

	thumbnailTimeout = 20 * time.Second
	thumbnailQuality = 82
)

// ErrUnsafeThumbnailURL is returned for a thumbnail URL that is not a plain
// https address, or that resolves to a private network.
var ErrUnsafeThumbnailURL = errors.New("unsafe thumbnail address")

// ThumbnailCache fetches, downscales and stores preview images on disk.
//
// It is safe for concurrent use, and concurrent requests for the same image
// collapse into one download.
type ThumbnailCache struct {
	dir      string
	maxBytes int64
	client   *http.Client

	mu       sync.Mutex
	inFlight map[string]*sync.WaitGroup
}

// NewThumbnailCache returns a cache storing images under dir.
func NewThumbnailCache(dir string, maxBytes int64) *ThumbnailCache {
	if maxBytes <= 0 {
		maxBytes = DefaultThumbnailCacheBytes
	}
	return &ThumbnailCache{
		dir:      dir,
		maxBytes: maxBytes,
		client:   newRestrictedClient(),
		inFlight: map[string]*sync.WaitGroup{},
	}
}

// newRestrictedClient builds an HTTP client that refuses to talk to anything on
// a private network.
//
// Thumbnail URLs come out of yt-dlp's metadata, which is ultimately controlled
// by the site being downloaded from. Checking the address at dial time — after
// DNS has resolved — is what makes a hostname that points at localhost or a
// LAN address fail rather than succeed.
func newRestrictedClient() *http.Client {
	dialer := &net.Dialer{
		Timeout: 10 * time.Second,
		Control: func(_, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return ErrUnsafeThumbnailURL
			}
			ip := net.ParseIP(host)
			if ip == nil {
				return ErrUnsafeThumbnailURL
			}
			if !isPublicIP(ip) {
				return fmt.Errorf("%w: %s is not a public address", ErrUnsafeThumbnailURL, ip)
			}
			return nil
		},
	}

	return &http.Client{
		Timeout:   thumbnailTimeout,
		Transport: &http.Transport{DialContext: dialer.DialContext},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("too many redirects")
			}
			// A redirect must not be able to downgrade to http or another scheme.
			if req.URL.Scheme != "https" {
				return ErrUnsafeThumbnailURL
			}
			return nil
		},
	}
}

func isPublicIP(ip net.IP) bool {
	return !(ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() ||
		ip.IsInterfaceLocalMulticast())
}

// Get returns the path of a cached thumbnail scaled to width, fetching it first
// if it is not already stored.
func (c *ThumbnailCache) Get(ctx context.Context, rawURL string, width int) (string, error) {
	if err := checkThumbnailURL(rawURL); err != nil {
		return "", err
	}
	return c.get(ctx, rawURL, width)
}

// get does the work once the address has been vetted.
func (c *ThumbnailCache) get(ctx context.Context, rawURL string, width int) (string, error) {
	if width <= 0 {
		return "", fmt.Errorf("thumbnail width must be positive")
	}

	path := c.pathFor(rawURL, width)
	if touchIfExists(path) {
		return path, nil
	}

	// Collapse concurrent requests for the same image — a playlist of the same
	// uploader will ask for one avatar many times at once.
	c.mu.Lock()
	if wg, ok := c.inFlight[path]; ok {
		c.mu.Unlock()
		wg.Wait()
		if touchIfExists(path) {
			return path, nil
		}
		return "", fmt.Errorf("could not load the preview image")
	}
	wg := &sync.WaitGroup{}
	wg.Add(1)
	c.inFlight[path] = wg
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		delete(c.inFlight, path)
		c.mu.Unlock()
		wg.Done()
	}()

	if err := c.fetch(ctx, rawURL, width, path); err != nil {
		return "", err
	}
	c.evict()
	return path, nil
}

// checkThumbnailURL rejects anything that is not a plain https web address
// before a connection is attempted.
func checkThumbnailURL(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnsafeThumbnailURL, err)
	}
	if parsed.Scheme != "https" {
		return fmt.Errorf("%w: %q is not https", ErrUnsafeThumbnailURL, parsed.Scheme)
	}
	if parsed.Host == "" {
		return fmt.Errorf("%w: no host", ErrUnsafeThumbnailURL)
	}
	return nil
}

func (c *ThumbnailCache) pathFor(rawURL string, width int) string {
	sum := sha256.Sum256([]byte(rawURL + "@" + strconv.Itoa(width)))
	return filepath.Join(c.dir, hex.EncodeToString(sum[:])+".jpg")
}

func (c *ThumbnailCache) fetch(ctx context.Context, rawURL string, width int, path string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return fmt.Errorf("could not load the preview image: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("could not load the preview image: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("could not load the preview image: the site returned %s", resp.Status)
	}

	// Bound the read so an enormous or endless response cannot exhaust memory.
	src, _, err := image.Decode(io.LimitReader(resp.Body, maxThumbnailBytes))
	if err != nil {
		return fmt.Errorf("could not read the preview image: %w", err)
	}

	return writeJPEG(path, scaleToWidth(src, width))
}

// scaleToWidth downscales an image to the requested width, preserving aspect
// ratio. An image already narrower is returned untouched: upscaling a preview
// only makes it blurrier.
func scaleToWidth(src image.Image, width int) image.Image {
	bounds := src.Bounds()
	if bounds.Dx() <= width || bounds.Dx() == 0 {
		return src
	}

	height := int(float64(bounds.Dy()) * float64(width) / float64(bounds.Dx()))
	if height < 1 {
		height = 1
	}

	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, bounds, draw.Over, nil)
	return dst
}

// writeJPEG writes an image to a temporary file and renames it into place, so a
// reader never sees a half-written image.
func writeJPEG(path string, img image.Image) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), ".thumb-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())

	if err := jpeg.Encode(tmp, img, &jpeg.Options{Quality: thumbnailQuality}); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// touchIfExists reports whether path exists, updating its modification time so
// eviction treats it as recently used.
func touchIfExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() || info.Size() == 0 {
		return false
	}
	now := time.Now()
	_ = os.Chtimes(path, now, now)
	return true
}

// evict deletes the least recently used images until the cache fits its budget.
func (c *ThumbnailCache) evict() {
	c.mu.Lock()
	defer c.mu.Unlock()

	entries, err := os.ReadDir(c.dir)
	if err != nil {
		return
	}

	type cached struct {
		path string
		size int64
		used time.Time
	}
	var files []cached
	var total int64

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, cached{filepath.Join(c.dir, e.Name()), info.Size(), info.ModTime()})
		total += info.Size()
	}
	if total <= c.maxBytes {
		return
	}

	sort.Slice(files, func(i, j int) bool { return files[i].used.Before(files[j].used) })
	for _, f := range files {
		if total <= c.maxBytes {
			return
		}
		if os.Remove(f.path) == nil {
			total -= f.size
		}
	}
}

// Dir is where cached images are stored, so the app can serve them.
func (c *ThumbnailCache) Dir() string { return c.dir }

// Clear empties the cache.
func (c *ThumbnailCache) Clear() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return os.RemoveAll(c.dir)
}

// Size reports how many bytes the cache is currently using.
func (c *ThumbnailCache) Size() int64 {
	entries, err := os.ReadDir(c.dir)
	if err != nil {
		return 0
	}
	var total int64
	for _, e := range entries {
		if info, err := e.Info(); err == nil && !e.IsDir() {
			total += info.Size()
		}
	}
	return total
}
