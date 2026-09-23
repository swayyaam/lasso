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
