#!/usr/bin/env bash
#
# Builds the app's .icns from build/appicon.png and installs it into a built
# Lasso.app, replacing the one Wails generates.
#
# Wails derives every size from the single appicon.png. That is fine down to
# about 64px, but a detailed icon turns to mush at 16 and 32. If
# build/appicon-small.png exists it is used for those two sizes instead, so a
# simplified mark can carry the small end without affecting the large one.
#
# Usage: scripts/make-icon.sh [path/to/Lasso.app]

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
APP="${1:-$REPO_ROOT/apps/desktop/build/bin/Lasso.app}"
SOURCE="$REPO_ROOT/apps/desktop/build/appicon.png"
SMALL="$REPO_ROOT/apps/desktop/build/appicon-small.png"

die() { printf '\nerror: %s\n' "$*" >&2; exit 1; }

[ -f "$SOURCE" ] || die "no icon at $SOURCE"
command -v iconutil >/dev/null 2>&1 || die "iconutil not found"

# A non-square source would be stretched by sips; catch it here rather than
# shipping a distorted icon.
W=$(sips -g pixelWidth "$SOURCE" | awk '/pixelWidth/{print $2}')
H=$(sips -g pixelHeight "$SOURCE" | awk '/pixelHeight/{print $2}')
[ "$W" = "$H" ] || die "$SOURCE is ${W}x${H}; it must be square. Run: cd scripts/icon && go run . -in <artwork> -out $SOURCE"

staging="$(mktemp -d "${TMPDIR:-/tmp}/lasso-icon.XXXXXX")"
trap 'rm -rf "$staging"' EXIT
iconset="$staging/icon.iconset"
mkdir -p "$iconset"

# render <source> <pixels> <filename>
render() {
	sips -s format png -z "$2" "$2" "$1" --out "$iconset/$3" >/dev/null 2>&1 \
		|| die "could not render $3"
}

small_source="$SOURCE"
using_small="no"
if [ -f "$SMALL" ]; then
	SW=$(sips -g pixelWidth "$SMALL" | awk '/pixelWidth/{print $2}')
	SH=$(sips -g pixelHeight "$SMALL" | awk '/pixelHeight/{print $2}')
	if [ "$SW" = "$SH" ]; then
		small_source="$SMALL"
		using_small="yes"
	else
		printf 'warning: %s is %sx%s, not square — ignoring it\n' "$SMALL" "$SW" "$SH" >&2
	fi
fi

# The two sizes a simplified mark exists for.
render "$small_source" 16 icon_16x16.png
render "$small_source" 32 icon_16x16@2x.png
render "$small_source" 32 icon_32x32.png
render "$small_source" 64 icon_32x32@2x.png

render "$SOURCE" 128 icon_128x128.png
render "$SOURCE" 256 icon_128x128@2x.png
render "$SOURCE" 256 icon_256x256.png
render "$SOURCE" 512 icon_256x256@2x.png
render "$SOURCE" 512 icon_512x512.png
render "$SOURCE" 1024 icon_512x512@2x.png

iconutil -c icns "$iconset" -o "$staging/iconfile.icns" || die "iconutil failed"

if [ -d "$APP" ]; then
	cp "$staging/iconfile.icns" "$APP/Contents/Resources/iconfile.icns"
	# Without this the Dock keeps showing whatever it cached for this path.
	touch "$APP" "$APP/Contents/Info.plist" "$APP/Contents/Resources/iconfile.icns"
	printf 'Installed icon into %s\n' "$(basename "$APP")"
else
	cp "$staging/iconfile.icns" "$REPO_ROOT/apps/desktop/build/iconfile.icns"
	printf 'No app bundle; wrote build/iconfile.icns\n'
fi

if [ "$using_small" = "yes" ]; then
	printf '  16 and 32px use appicon-small.png\n'
else
	printf '  16 and 32px use appicon.png (add build/appicon-small.png for a simplified mark)\n'
fi
