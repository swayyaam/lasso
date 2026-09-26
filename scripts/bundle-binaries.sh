#!/usr/bin/env bash
#
# Copies the fetched sidecar binaries into a built Lasso.app.
#
# They go into Contents/Resources/bin, which is where packages/binaries looks
# for them. At first launch the app copies them out to Application Support and
# runs them from there — the bundle is read-only, so yt-dlp could not update
# itself in place.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
APP="${1:-$REPO_ROOT/apps/desktop/build/bin/Lasso.app}"

die() { printf '\nerror: %s\n' "$*" >&2; exit 1; }

case "$(uname -m)" in
	arm64) PLATFORM="darwin-arm64" ;;
	x86_64) PLATFORM="darwin-amd64" ;;
	*) die "unsupported architecture: $(uname -m)" ;;
esac

SOURCE="$REPO_ROOT/apps/desktop/build/bin/$PLATFORM"
[ -d "$APP" ] || die "no app bundle at $APP — run \`wails build\` first"
[ -f "$SOURCE/manifest.json" ] || die "no sidecar binaries at $SOURCE — run \`make setup\`"

DEST="$APP/Contents/Resources/bin"
rm -rf "$DEST"
mkdir -p "$DEST"

# -R preserves the symlinks inside yt-dlp's onedir payload.
cp -R "$SOURCE"/. "$DEST"/
# The fetch stamps are build-time bookkeeping and have no place in the bundle.
rm -rf "$DEST/.stamps"

# Thin to arm64. yt-dlp ships universal2: 107 of its files carry an x86_64
# slice as well, 55 MB that an Apple Silicon build never runs (ffmpeg, ffprobe
# and deno are arm64 already). lipo strips each file's signature, so every
# thinned image is re-signed ad hoc — yt-dlp's own are ad hoc too, so nothing
# stronger is lost — the way the app's own install repairs one, and checked the
# way it checks one. Nested images first and the launcher last: signing a
# component invalidates a signature that covers it.
#
# Only the bundled copy is thinned. yt-dlp's own updates bring the universal
# build back to the installed copy, so the saving is in the download.
#
# Each image is signed and checked away from its folder, then copied back over
# the original, keeping its mode. In place, codesign reads the three copies of
# libpython in yt-dlp's Python.framework as the framework's own — whose links
# arrive flattened into copies, so it cannot tell app from framework — and
# refuses to sign them ("bundle format is ambiguous") or to verify them, even
# untouched. The signature is the image's own either way, and yt-dlp's are the
# standalone kind (Python-<id>); what decides whether it works is yt-dlp
# starting, which is checked below.
thin() {
	local f="$1" archs image
	archs="$(lipo -archs "$f" 2>/dev/null)" || return 0 # not Mach-O
	case " $archs " in *" arm64 "*) ;; *) return 0 ;; esac
	[ "$archs" = "arm64" ] && return 0
	image="$work/$(basename "$f")"
	lipo "$f" -thin arm64 -output "$image" || die "could not thin $f"
	codesign --force --sign - "$image" >/dev/null 2>&1 || die "could not re-sign $f"
	codesign --verify --no-strict "$image" 2>/dev/null || die "$f does not verify after thinning"
	cp "$image" "$f"
	rm -f "$image"
	thinned=$((thinned + 1))
}
if [ "$PLATFORM" = "darwin-arm64" ]; then
	work="$(mktemp -d "${TMPDIR:-/tmp}/lasso-thin.XXXXXX")"
	trap 'rm -rf "$work"' EXIT
	before="$(du -sk "$DEST" | cut -f1)"
	thinned=0
	while IFS= read -r -d '' f; do thin "$f"; done < <(find "$DEST" -type f ! -name 'yt-dlp_macos' -print0)
	while IFS= read -r -d '' f; do thin "$f"; done < <(find "$DEST" -type f -name 'yt-dlp_macos' -print0)
	# Proof, not faith: the thinned yt-dlp has to start.
	"$DEST/yt-dlp/yt-dlp_macos" --version >/dev/null 2>&1 || die "yt-dlp does not run after thinning"
	after="$(du -sk "$DEST" | cut -f1)"
	printf 'Thinned %d universal files to arm64, %d MB smaller\n' "$thinned" "$(((before - after) / 1024))"
fi

# The licences travel with every copy: most of what Lasso is built from may be
# redistributed only with its notice, and the GPL programs only with directions
# to their source. Outside bin/, so the update zip, which empties bin/, keeps
# them too.
LEGAL="$APP/Contents/Resources"
for doc in LICENSE THIRD_PARTY_NOTICES.md PRIVACY.md TERMS.md; do
	[ -f "$REPO_ROOT/$doc" ] || die "missing $doc — run \`make notices\` for THIRD_PARTY_NOTICES.md"
	cp "$REPO_ROOT/$doc" "$LEGAL/$doc"
done

printf 'Bundled sidecar binaries (%s) into %s\n' "$PLATFORM" "$(basename "$APP")"
du -sh "$DEST" | awk '{printf "  %s of helper programs\n", $1}'
