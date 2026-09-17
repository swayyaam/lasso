package binaries

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// InstallReport describes what Install did.
type InstallReport struct {
	// Installed lists binaries copied during this call.
	Installed []Name
	// Skipped lists binaries already present at the expected version.
	Skipped []Name
	// Fixups notes non-fatal repairs, keyed by binary.
	Fixups map[Name][]string
}

// Changed reports whether anything was copied.
func (r InstallReport) Changed() bool { return len(r.Installed) > 0 }

// Install copies the bundled binaries into the Application Support directory,
// makes them executable, strips quarantine, and repairs signatures if macOS
// would otherwise refuse to run them.
//
// It is idempotent: a binary already present at the manifest's version is left
// alone. Binaries are always executed from Application Support, never from
// inside the .app bundle, because the bundle is read-only and yt-dlp needs to
// be able to update itself.
func (m *Manager) Install(ctx context.Context) (InstallReport, error) {
	report := InstallReport{Fixups: map[Name][]string{}}

	if err := os.MkdirAll(m.paths.Bin, 0o755); err != nil {
		return report, fmt.Errorf("creating %s: %w", m.paths.Bin, err)
	}

	installed := readStamp(m.paths.Bin)

	for _, name := range requiredBinaries {
		spec := m.manifest.Binaries[name]
		target := filepath.Join(m.paths.Bin, spec.relPath(name))

		if !needsInstall(name, installed.Binaries[name], spec.Version) {
			if _, err := os.Stat(target); err == nil {
				report.Skipped = append(report.Skipped, name)
				continue
			}
		}

		src := filepath.Join(m.paths.Source, spec.rootRel(name))
		dst := filepath.Join(m.paths.Bin, spec.rootRel(name))
		if err := copyTree(src, dst); err != nil {
			return report, fmt.Errorf("installing %s: %w", name, err)
		}

		fixes, err := m.fixup(ctx, target, spec)
		if err != nil {
			return report, fmt.Errorf("preparing %s: %w", name, err)
		}
		if len(fixes) > 0 {
			report.Fixups[name] = fixes
		}

		installed.Binaries[name] = spec.Version
		report.Installed = append(report.Installed, name)
	}

	if err := writeStamp(m.paths.Bin, installed); err != nil {
		return report, fmt.Errorf("recording installed versions: %w", err)
	}
	return report, nil
}

// fixup makes a freshly placed binary runnable: executable bit, no quarantine
// flag, and a signature macOS will accept.
//
// Apple Silicon refuses to exec an unsigned or broken-signature binary, so a
// bad signature is repaired with an ad-hoc one. Only the entrypoint is signed —
// never `--deep` — because yt-dlp's onedir build ships its own signed dylibs
// under _internal and re-signing them would break it.
func (m *Manager) fixup(ctx context.Context, target string, spec Spec) ([]string, error) {
	var fixes []string

	if err := os.Chmod(target, 0o755); err != nil {
		return fixes, fmt.Errorf("chmod: %w", err)
	}

	// Files this process copied do not normally carry com.apple.quarantine, but
	// they inherit it when the .app itself was quarantined. Clearing is
	// best-effort: "no such xattr" is the common, harmless case.
	root := filepath.Dir(target)
	if spec.Layout == LayoutFile {
		root = target
	}
	if out, err := m.run(ctx, 20*time.Second, "xattr", "-r", "-d", "com.apple.quarantine", root); err == nil {
		fixes = append(fixes, "cleared quarantine flag")
	} else if !isNoSuchXattr(out) {
		// Not fatal: verification will catch a binary that genuinely cannot run.
		fixes = append(fixes, "could not clear quarantine flag")
	}

	// codesign only understands Mach-O images.
	machO, err := isMachO(target)
	if err != nil {
		return fixes, err
	}
	if !machO {
		return fixes, nil
	}

	if _, err := m.run(ctx, 60*time.Second, "codesign", "--verify", "--no-strict", target); err != nil {
		if _, signErr := m.run(ctx, 120*time.Second, "codesign", "--force", "--sign", "-", target); signErr != nil {
			// Report it, but let verification decide whether it actually matters.
			fixes = append(fixes, "signature invalid and could not be repaired")
			return fixes, nil
		}
		fixes = append(fixes, "repaired code signature (ad-hoc)")
	}
	return fixes, nil
}

// isMachO reports whether path starts with a Mach-O or universal-binary magic
// number. Anything else (a shell script, say) must not be handed to codesign.
func isMachO(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()

	var magic [4]byte
	if _, err := io.ReadFull(f, magic[:]); err != nil {
		return false, nil // too short to be Mach-O
	}
	switch binary.BigEndian.Uint32(magic[:]) {
	case 0xfeedface, 0xfeedfacf, // 32- and 64-bit, big-endian
		0xcefaedfe, 0xcffaedfe, // 32- and 64-bit, little-endian
		0xcafebabe, 0xbebafeca: // universal
		return true, nil
	}
	return false, nil
}

func isNoSuchXattr(output string) bool {
	return strings.Contains(output, "No such xattr") || strings.Contains(output, "No such file")
}

// run executes a short-lived helper command and returns its combined output.
func (m *Manager) run(ctx context.Context, timeout time.Duration, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	return string(out), err
}

// copyTree copies a file or a directory tree, replacing whatever is at dst.
//
// The copy lands beside the target first and is then renamed into place, so an
// interrupted install cannot leave a half-written binary that looks valid.
func copyTree(src, dst string) error {
	info, err := os.Lstat(src)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%s is missing from the bundled binaries", filepath.Base(src))
		}
		return err
	}

	staging := dst + ".incoming"
	if err := os.RemoveAll(staging); err != nil {
		return err
	}
	if info.IsDir() {
		err = copyDir(src, staging)
	} else {
		err = copyFile(src, staging, info.Mode())
	}
	if err != nil {
		os.RemoveAll(staging)
		return err
	}

	if err := os.RemoveAll(dst); err != nil {
		os.RemoveAll(staging)
		return err
	}
	if err := os.Rename(staging, dst); err != nil {
		os.RemoveAll(staging)
		return err
	}
	return nil
}

func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)

		info, err := d.Info()
		if err != nil {
			return err
		}
		switch {
		case d.IsDir():
			return os.MkdirAll(target, 0o755)
		case info.Mode()&os.ModeSymlink != 0:
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(link, target)
		default:
			return copyFile(path, target, info.Mode())
		}
	})
}

func copyFile(src, dst string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode.Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// fixupTree makes every file in a freshly unpacked directory runnable.
//
// An update replaces the whole folder, not just the launcher, so the repairs
// have to cover all of it: the executable bit on anything that is a Mach-O
// image, the quarantine flag on the entire tree, and a signature macOS will
// accept on every signed component.
//
// Signatures are verified first and only repaired where verification fails.
// Re-signing all 100-odd images unconditionally would be slow and would
// replace yt-dlp's own valid signatures with weaker ad-hoc ones for no gain.
// Nested images are signed before the entrypoint, because signing a component
// invalidates any signature that covers it.
func (m *Manager) fixupTree(ctx context.Context, dir, entrypoint string) ([]string, error) {
	var fixes []string

	// Directories must be traversable and the payload readable once bundled.
	if err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return os.Chmod(path, 0o755)
		}
		return nil
	}); err != nil {
		return fixes, fmt.Errorf("setting directory permissions: %w", err)
	}

	if out, err := m.run(ctx, 60*time.Second, "xattr", "-r", "-d", "com.apple.quarantine", dir); err == nil {
		fixes = append(fixes, "cleared quarantine flag on the whole folder")
	} else if !isNoSuchXattr(out) {
		fixes = append(fixes, "could not clear quarantine flag")
	}

	var images []string
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		macho, err := isMachO(path)
		if err != nil || !macho {
			return nil //nolint:nilerr // an unreadable file is not a signing problem
		}
		images = append(images, path)
		return nil
	})
	if err != nil {
		return fixes, fmt.Errorf("scanning the update: %w", err)
	}

	target := filepath.Join(dir, entrypoint)
	if err := os.Chmod(target, 0o755); err != nil {
		return fixes, fmt.Errorf("chmod: %w", err)
	}

	repaired := 0
	for _, image := range images {
		if image == target {
			continue // signed last, below
		}
		if m.repairSignature(ctx, image) {
			repaired++
		}
	}
	if repaired > 0 {
		fixes = append(fixes, fmt.Sprintf("re-signed %d of %d bundled components", repaired, len(images)))
	}
	if m.repairSignature(ctx, target) {
		fixes = append(fixes, "repaired the launcher's code signature")
	}

	return fixes, nil
}

// repairSignature ad-hoc signs an image whose signature macOS would reject,
// and reports whether it had to. Apple Silicon refuses to exec an image with a
// broken or absent signature, so this is what stands between a replaced file
// and a download that dies on launch.
func (m *Manager) repairSignature(ctx context.Context, path string) bool {
	if _, err := m.run(ctx, 30*time.Second, "codesign", "--verify", "--no-strict", path); err == nil {
		return false
	}
	if _, err := m.run(ctx, 60*time.Second, "codesign", "--force", "--sign", "-", path); err != nil {
		return false
	}
	return true
}

// needsInstall decides whether the bundled copy should replace what is on disk.
//
// yt-dlp updates itself in place between app releases, so a newer installed
// version is the expected state and must not be overwritten by the older copy
// inside the bundle. Its versions are dates (2026.08.19), which order
// correctly as strings.
//
// The other binaries are only ever changed by shipping a new app, so any
// difference there means the bundle is authoritative.
func needsInstall(name Name, installed, bundled string) bool {
	if installed == "" {
		return true
	}
	if name == YtDlp {
		return bundled > installed
	}
	return installed != bundled
}
