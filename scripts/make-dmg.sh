#!/usr/bin/env bash
#
# Packages the built Lasso.app into an unsigned, compressed DMG with the
# customary drag-to-Applications layout.
#
# The result is unsigned and un-notarised, so macOS will quarantine it on a
# machine that did not build it. That is expected at this stage.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
APP="${1:-$REPO_ROOT/apps/desktop/build/bin/Lasso.app}"
OUT="${2:-$REPO_ROOT/apps/desktop/build/bin/Lasso.dmg}"

die() { printf '\nerror: %s\n' "$*" >&2; exit 1; }

[ -d "$APP" ] || die "no app bundle at $APP — run \`make build\` first"
[ -x "$APP/Contents/MacOS/Lasso" ] || die "$APP has no executable; the build did not finish"

VERSION="$(/usr/libexec/PlistBuddy -c 'Print :CFBundleShortVersionString' "$APP/Contents/Info.plist" 2>/dev/null || echo 0.0.0)"

staging="$(mktemp -d "${TMPDIR:-/tmp}/lasso-dmg.XXXXXX")"
trap 'rm -rf "$staging"' EXIT

cp -R "$APP" "$staging/"
ln -s /Applications "$staging/Applications"

rm -f "$OUT"
hdiutil create \
	-volname "Lasso $VERSION" \
	-srcfolder "$staging" \
	-fs HFS+ \
	-format UDZO \
	-imagekey zlib-level=9 \
	-quiet \
	"$OUT"

printf 'Built %s\n' "$OUT"
du -h "$OUT" | awk '{printf "  %s\n", $1}'
