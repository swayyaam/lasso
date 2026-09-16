package binaries

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// AppName is the folder Lasso owns under Application Support.
const AppName = "Lasso"

// SourceEnv overrides where Install copies binaries from. Set it to point at a
// fetched sidecar directory when running outside a built .app.
const SourceEnv = "LASSO_BIN_SOURCE"

// SupportEnv overrides the Application Support directory. Tests use it; so can
// anyone who wants a throwaway install.
const SupportEnv = "LASSO_SUPPORT_DIR"

// Paths are the directories the manager works with.
type Paths struct {
	// Support is ~/Library/Application Support/Lasso.
	Support string
	// Bin is where binaries are executed from: <Support>/bin.
	Bin string
	// Source is the read-only directory the binaries are copied from —
	// Lasso.app/Contents/Resources/bin in a built app.
	Source string
}

// SupportDir returns Lasso's Application Support directory, honouring
// SupportEnv.
func SupportDir() (string, error) {
	if override := os.Getenv(SupportEnv); override != "" {
		return override, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot locate your home directory: %w", err)
	}
	return filepath.Join(home, "Library", "Application Support", AppName), nil
}

// platformDir is the fetch script's per-platform folder name.
func platformDir() string {
	return "darwin-" + runtime.GOARCH
}

// ResolveSource finds the directory holding the bundled sidecar binaries.
//
// It tries, in order: the SourceEnv override, the Resources directory of the
// surrounding .app bundle, and finally the development tree, so `wails dev`
// works straight after `make setup`.
func ResolveSource() (string, error) {
	var tried []string

	if override := os.Getenv(SourceEnv); override != "" {
		if isSourceDir(override) {
			return override, nil
		}
		return "", fmt.Errorf("%s is set to %q but there is no %s there", SourceEnv, override, manifestName)
	}

	if exe, err := os.Executable(); err == nil {
		if exe, err := filepath.EvalSymlinks(exe); err == nil {
			// Lasso.app/Contents/MacOS/Lasso -> Lasso.app/Contents/Resources/bin
			candidate := filepath.Join(filepath.Dir(exe), "..", "Resources", "bin")
			if isSourceDir(candidate) {
				return filepath.Clean(candidate), nil
			}
			tried = append(tried, filepath.Clean(candidate))

			// `wails dev` builds into a throwaway directory; walk up looking
			// for the repo's fetched binaries.
			if dir, ok := findDevSource(filepath.Dir(exe)); ok {
				return dir, nil
			}
		}
	}

	if wd, err := os.Getwd(); err == nil {
		if dir, ok := findDevSource(wd); ok {
			return dir, nil
		}
		tried = append(tried, filepath.Join(wd, "apps/desktop/build/bin", platformDir()))
	}

	return "", &SourceMissingError{Tried: tried}
}

// findDevSource walks up from start looking for apps/desktop/build/bin/<platform>.
func findDevSource(start string) (string, bool) {
	dir := start
	for i := 0; i < 12; i++ {
		candidate := filepath.Join(dir, "apps", "desktop", "build", "bin", platformDir())
		if isSourceDir(candidate) {
			return candidate, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", false
}

func isSourceDir(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, manifestName))
	return err == nil && !info.IsDir()
}

// ResolvePaths assembles every directory the manager needs.
func ResolvePaths() (Paths, error) {
	support, err := SupportDir()
	if err != nil {
		return Paths{}, err
	}
	source, err := ResolveSource()
	if err != nil {
		return Paths{}, err
	}
	return Paths{
		Support: support,
		Bin:     filepath.Join(support, "bin"),
		Source:  source,
	}, nil
}
