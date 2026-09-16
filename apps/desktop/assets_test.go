package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validThumbName = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef.jpg"

func TestThumbnailHandlerServesCachedImage(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, validThumbName), []byte("jpeg-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, thumbURLPrefix+validThumbName, nil)
	thumbnailHandler{dir: dir}.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if rec.Body.String() != "jpeg-bytes" {
		t.Errorf("body = %q", rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "image/jpeg" {
		t.Errorf("Content-Type = %q", got)
	}
	// Content is addressed by a hash of source and size, so it can be cached
	// forever.
	if !strings.Contains(rec.Header().Get("Cache-Control"), "immutable") {
		t.Errorf("Cache-Control = %q", rec.Header().Get("Cache-Control"))
	}
}

// TestThumbnailHandlerRefusesEscapes is the reason the handler matches a strict
// name pattern rather than joining whatever it is given.
func TestThumbnailHandlerRefusesEscapes(t *testing.T) {
	dir := t.TempDir()
	secret := filepath.Join(filepath.Dir(dir), "secret.txt")
	if err := os.WriteFile(secret, []byte("private"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Remove(secret) })

	paths := []string{
		thumbURLPrefix + "../secret.txt",
		thumbURLPrefix + "..%2Fsecret.txt",
		thumbURLPrefix + "../../etc/passwd",
		thumbURLPrefix + "subdir/" + validThumbName,
		thumbURLPrefix + "notahash.jpg",
		thumbURLPrefix + strings.ToUpper(validThumbName),
		thumbURLPrefix + "0123456789abcdef.jpg",
		thumbURLPrefix + validThumbName + ".sh",
		thumbURLPrefix,
	}

	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, path, nil)
			thumbnailHandler{dir: dir}.ServeHTTP(rec, req)

			if rec.Code != http.StatusNotFound {
				t.Errorf("status = %d, want 404 for %q", rec.Code, path)
			}
			if strings.Contains(rec.Body.String(), "private") {
				t.Errorf("handler served a file outside the cache for %q", path)
			}
		})
	}
}

func TestThumbnailHandlerWithoutCache(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, thumbURLPrefix+validThumbName, nil)
	thumbnailHandler{dir: ""}.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 before the cache exists", rec.Code)
	}
}

func TestThumbnailURL(t *testing.T) {
	got := thumbnailURL("/Users/x/Library/Application Support/Lasso/thumbnails/" + validThumbName)
	if got != thumbURLPrefix+validThumbName {
		t.Errorf("thumbnailURL = %q", got)
	}
}

func TestAssetHandlerReturns404BeforeStartup(t *testing.T) {
	// Before startup finishes there is no cache, and the handler must answer
	// rather than panic on a nil dependency.
	app := NewApp()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, thumbURLPrefix+validThumbName, nil)
	app.assetHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}
