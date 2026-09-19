package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/swayyaam/lasso/packages/updater"
)

// bundlePath is the .app this process is running out of, or "" in development.
//
// The executable sits at Lasso.app/Contents/MacOS/Lasso, so the bundle is three
// levels up. Symlinks are resolved first: without that, a bundle reached
// through one would be replaced at the wrong path.
func bundlePath() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}

	bundle := filepath.Clean(filepath.Join(filepath.Dir(exe), "..", ".."))
	if !strings.HasSuffix(bundle, ".app") {
		// `go run`, `go test`, and `wails dev` before it packages.
		return ""
	}
	return bundle
}

// appVersion reads CFBundleShortVersionString from the running bundle.
//
// Info.plist rather than a compile-time constant, so the version the updater
// compares against is the one macOS shows in Finder and About — there is no
// second place for it to drift from.
func appVersion(bundle string) string {
	if bundle == "" {
		return ""
	}
	out, err := exec.Command("/usr/libexec/PlistBuddy",
		"-c", "Print :CFBundleShortVersionString",
		filepath.Join(bundle, "Contents", "Info.plist"),
	).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// newUpdater builds an updater for the running app, or explains why it cannot.
func newUpdater() (*updater.Updater, error) {
	bundle := bundlePath()
	if bundle == "" {
		return nil, fmt.Errorf("Lasso is running from a development build, which updates itself by being rebuilt")
	}

	version := appVersion(bundle)
	if version == "" {
		return nil, fmt.Errorf("could not read Lasso's own version, so there is nothing to compare a release against")
	}

	return updater.New(updater.Config{
		BundlePath:     bundle,
		CurrentVersion: version,
		// Empty unless overridden, which leaves the updater on its default.
		ReleaseAPI: os.Getenv(updater.APIEnv),
	})
}

// relaunch starts the newly installed app and asks this one to quit.
//
// `open -n` rather than exec: the replacement is a bundle, and macOS has to
// launch it as an application for it to get a dock icon, a main menu and the
// bundle identity its notifications depend on.
//
// The two overlap for a moment, which is why InstallUpdate refuses while
// anything is downloading — this process quitting would take its downloads
// with it.
func relaunch(ctx context.Context, bundle string) error {
	if bundle == "" {
		return fmt.Errorf("there is no installed app to restart")
	}
	cmd := exec.Command("/usr/bin/open", "-n", bundle)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("could not start the new version: %w", err)
	}
	// Released rather than waited on: `open` returns once LaunchServices has
	// the request, and this process is about to exit anyway.
	_ = cmd.Process.Release()

	// A breath so the new instance is up before this one tears down its
	// window, which otherwise looks like the app simply quit.
	time.Sleep(600 * time.Millisecond)
	return nil
}
