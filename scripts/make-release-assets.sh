#!/usr/bin/env bash
#
# Produces everything a release publishes:
#
#   Lasso.dmg       the full installer, for a first install
#   Lasso-app.zip   the app without its helper programs, for in-app updates
#   latest.json     what the release says about itself: version, notes, and
#                   each file's size and digest. This is what the updater
#                   reads, from releases/latest/download/, which is GitHub's
#                   download CDN rather than its rate-limited API.
#   SHA256SUMS      digests for people checking by hand — and for Lasso 0.1.2
#                   to 0.1.5, which verify against it. Keep publishing it.
#
# The zip is the interesting one. Contents/Resources/bin is 328 MB of the
# bundle's 329 MB and changes only when binaries.lock.json does, so it is left
# out and the installed copy is carried across during the update instead —
# about 15 MB downloaded rather than 150.
#
# manifest.json stays behind in the emptied directory. That is what lets the
# updater tell whether carrying the old helpers across is honest: if the
# release wants different versions, the manifests disagree and it refuses
# rather than installing an update that silently keeps the old ffmpeg.
#
# Usage: NOTES=path/to/notes.md scripts/make-release-assets.sh [path/to/Lasso.app] [output-dir]
#
# NOTES is required: the app shows them before installing, and a release
# without them would offer an update with nothing to say about it.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
APP="${1:-$REPO_ROOT/apps/desktop/build/bin/Lasso.app}"
OUTDIR="${2:-$REPO_ROOT/apps/desktop/build/bin}"

die() { printf '\nerror: %s\n' "$*" >&2; exit 1; }

[ -n "${NOTES:-}" ] || die "set NOTES to the release notes file: NOTES=notes.md make release-assets"
[ -s "$NOTES" ] || die "NOTES=$NOTES is missing or empty"
[ -d "$APP" ] || die "no app bundle at $APP — run \`make build\` first"
[ -f "$APP/Contents/Resources/bin/manifest.json" ] || die "$APP has no helper manifest; run \`make build\`"

# The updater re-signs what it installs, but a release asset that is already
# broken would only be found by whoever downloaded it.
codesign --verify --deep --strict "$APP" >/dev/null 2>&1 \
	|| die "$APP is not validly signed; run scripts/sign-app.sh"

DMG="$OUTDIR/Lasso.dmg"
ZIP="$OUTDIR/Lasso-app.zip"
SUMS="$OUTDIR/SHA256SUMS"
MANIFEST="$OUTDIR/latest.json"

[ -f "$DMG" ] || die "no $DMG — run \`make dmg\` first"

staging="$(mktemp -d "${TMPDIR:-/tmp}/lasso-thin.XXXXXX")"
trap 'rm -rf "$staging"' EXIT

# ditto rather than cp: it carries the extended attributes and symlinks a
# signed bundle depends on, and it is what will unpack this on the other side.
ditto "$APP" "$staging/$(basename "$APP")" || die "could not copy the app"

thin_bin="$staging/$(basename "$APP")/Contents/Resources/bin"
manifest="$(cat "$thin_bin/manifest.json")"
rm -rf "$thin_bin"
mkdir -p "$thin_bin"
printf '%s\n' "$manifest" > "$thin_bin/manifest.json"

rm -f "$ZIP"
ditto -c -k --sequesterRsrc --keepParent "$staging/$(basename "$APP")" "$ZIP" \
	|| die "could not build $ZIP"

# Written with bare filenames so the digests are checkable from a directory of
# downloaded assets: `shasum -a 256 -c SHA256SUMS`.
( cd "$OUTDIR" && shasum -a 256 "$(basename "$DMG")" "$(basename "$ZIP")" > "$(basename "$SUMS")" ) \
	|| die "could not write $SUMS"

# The version comes from the bundle itself — the same Info.plist the running
# app reads — so the manifest cannot disagree with what it describes.
VERSION="$(/usr/libexec/PlistBuddy -c 'Print :CFBundleShortVersionString' "$APP/Contents/Info.plist")" \
	|| die "could not read the app's version"

# Written by the same Manifest type the updater parses, and validated by it,
# so a release the app could not install is refused here rather than by users.
( cd "$REPO_ROOT" && go run ./packages/updater/cmd/release-manifest \
	-version "$VERSION" -notes "$NOTES" -out "$MANIFEST" "$ZIP" "$DMG" ) \
	|| die "could not write $MANIFEST"

printf 'Release assets for %s in %s\n' "$VERSION" "$OUTDIR"
for f in "$DMG" "$ZIP" "$MANIFEST" "$SUMS"; do
	printf '  %-16s %s\n' "$(basename "$f")" "$(du -h "$f" | awk '{print $1}')"
done
