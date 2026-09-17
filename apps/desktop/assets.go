package main

import (
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// thumbURLPrefix is the path the frontend uses to load cached preview images.
const thumbURLPrefix = "/thumbs/"

// thumbNamePattern matches exactly what ThumbnailCache produces: a sha256 hex
// digest with a .jpg suffix. Anything else is refused, which is what stops a
// crafted request from walking out of the cache directory.
var thumbNamePattern = regexp.MustCompile(`^[0-9a-f]{64}\.jpg$`)

// thumbnailHandler serves cached preview images to the webview.
//
// Wails serves the embedded frontend from a custom scheme, and a file:// URL is
// not loadable from there. Handing images to the webview through the asset
// server keeps them as ordinary <img src> targets — so the browser does its own
// lazy loading and caching — without the page ever reaching the network itself.
type thumbnailHandler struct {
	dir string
}

func (h thumbnailHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, thumbURLPrefix)
	if h.dir == "" || !thumbNamePattern.MatchString(name) {
		notFound(w, r)
		return
	}

	// The long-lived cache header must only go on a response that actually
	// carries an image. Setting it before ServeFile put it on 404s too, and a
	// webview then cached "this thumbnail does not exist" for a year — so a
	// preview requested a moment too early stayed blank for good.
	path := filepath.Join(h.dir, name)
	if info, err := os.Stat(path); err != nil || info.IsDir() {
		notFound(w, r)
		return
	}

	// Content is addressed by a hash of its source and size, so it can never
	// change under a given name.
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("Content-Type", "image/jpeg")
	http.ServeFile(w, r, path)
}

// notFound answers a missing thumbnail without letting the client remember it.
func notFound(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	http.NotFound(w, r)
}

// thumbnailURL turns a cached file path into the path the frontend requests.
func thumbnailURL(path string) string {
	return thumbURLPrefix + filepath.Base(path)
}
