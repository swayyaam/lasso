package core

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// MusicLibrary hands finished audio to a music app. Adding is a copy: the
// download stays in the download folder, where Lasso's history points.
type MusicLibrary interface {
	Add(path string) error
}

// ErrNoMusicFolder is Music having no "Automatically Add to Music" folder to
// hand files to. Music makes it the first time it opens.
var ErrNoMusicFolder = errors.New(`Music has no "Automatically Add to Music" folder yet. Open Music once, then try again`)

// musicOpens are the extensions Music imports. It cannot open Opus, FLAC,
// Vorbis or WebM: handed one, it sets the file aside in a "Not Added" folder
// and says nothing, so Lasso does not hand it over at all.
var musicOpens = map[string]bool{
	".mp3": true, ".m4a": true, ".m4b": true, ".aac": true,
	".aif": true, ".aiff": true, ".wav": true,
}

// MusicOpens reports whether Music can import the file at path.
func MusicOpens(path string) bool {
	return musicOpens[strings.ToLower(filepath.Ext(path))]
}

// musicFormatName names a file's format for a notice: "Opus", "FLAC".
func musicFormatName(path string) string {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(path), "."))
	switch ext {
	case "opus":
		return "Opus"
	case "ogg":
		return "Vorbis"
	case "":
		return "this"
	}
	return strings.ToUpper(ext)
}

// MusicFolder is Music's "Automatically Add to Music" folder, found under
// Home. Music imports whatever lands there into the library and moves it out.
//
// It is looked for on every add rather than once: Music makes the folder the
// first time it opens, which can be after Lasso has.
type MusicFolder struct {
	Home string
}

var _ MusicLibrary = MusicFolder{}

// autoAddPatterns are where Music keeps the folder, most likely first: a
// library made by Music, then one carried over from iTunes, whose media folder
// Music goes on using.
var autoAddPatterns = []string{
	"Music/Music/Media*/Automatically Add to Music*",
	"Music/iTunes/iTunes Media*/Automatically Add to Music*",
	"Music/*/*/Automatically Add to Music*",
}

// Find returns the folder, or ErrNoMusicFolder.
func (m MusicFolder) Find() (string, error) {
	if m.Home == "" {
		return "", ErrNoMusicFolder
	}
	for _, pattern := range autoAddPatterns {
		matches, _ := filepath.Glob(filepath.Join(m.Home, pattern))
		for _, match := range matches {
			if info, err := os.Stat(match); err == nil && info.IsDir() {
				return match, nil
			}
		}
	}
	return "", ErrNoMusicFolder
}

// Add copies path into the folder under its own name.
//
// Music watches the folder and imports what appears, so a half-written file
// must never appear there. The copy is written beside the folder, in Music's
// media folder on the same volume, and renamed in once complete, which is
// atomic. A name already waiting in the folder gets a number rather than being
// replaced.
func (m MusicFolder) Add(path string) error {
	dir, err := m.Find()
	if err != nil {
		return err
	}
	src, err := os.Open(path)
	if err != nil {
		return err
	}
	defer src.Close()

	tmp, err := os.CreateTemp(filepath.Dir(dir), ".lasso-add-*"+filepath.Ext(path))
	if err != nil {
		return err
	}
	done := false
	defer func() {
		if !done {
			os.Remove(tmp.Name())
		}
	}()
	if _, err := io.Copy(tmp, src); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	dest := freeName(dir, filepath.Base(path))
	if err := os.Rename(tmp.Name(), dest); err != nil {
		return err
	}
	done = true
	return nil
}

// freeName is name in dir, or "name 2.ext" and so on when that is taken.
func freeName(dir, name string) string {
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	candidate := filepath.Join(dir, name)
	for n := 2; ; n++ {
		if _, err := os.Lstat(candidate); os.IsNotExist(err) {
			return candidate
		}
		candidate = filepath.Join(dir, fmt.Sprintf("%s %d%s", stem, n, ext))
	}
}
