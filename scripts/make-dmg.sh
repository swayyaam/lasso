#!/usr/bin/env bash
#
# Packages the built Lasso.app into a compressed DMG with a laid-out install
# window: the app on the left, Applications on the right, and a background
# saying what to do with them.
#
# Finder stores that layout in a .DS_Store inside the image, and the only way
# to write one is to have Finder do it — which means creating a writable image,
# mounting it, driving Finder over AppleScript, and only then compressing. A
# DMG built in one shot from a folder has no layout at all, which is the grey
# window with two icons in the corner.
#
# The result is unsigned and un-notarised, so macOS will warn about the
# developer on a machine that did not build it. That is expected. What is not
# acceptable is "damaged", which is what a broken seal produces — see
# scripts/sign-app.sh.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
APP="${1:-$REPO_ROOT/apps/desktop/build/bin/Lasso.app}"
OUT="${2:-$REPO_ROOT/apps/desktop/build/bin/Lasso.dmg}"

die() { printf '\nerror: %s\n' "$*" >&2; exit 1; }

[ -d "$APP" ] || die "no app bundle at $APP — run \`make build\` first"
[ -x "$APP/Contents/MacOS/Lasso" ] || die "$APP has no executable; the build did not finish"

# A DMG whose app has a broken seal opens as "damaged" and cannot be recovered
# from without a terminal. Refuse to ship one.
codesign --verify --deep --strict "$APP" >/dev/null 2>&1 \
	|| die "$APP is not validly signed; run scripts/sign-app.sh (make build does)"

VERSION="$(/usr/libexec/PlistBuddy -c 'Print :CFBundleShortVersionString' "$APP/Contents/Info.plist" 2>/dev/null || echo 0.0.0)"
VOLUME="Lasso $VERSION"

# These have to match scripts/dmg-background/main.go, which draws the picture
# the icons are positioned on top of.
WINDOW_W=640
WINDOW_H=420
APP_X=172
APPS_X=468
ICON_Y=182
ICON_SIZE=128

staging="$(mktemp -d "${TMPDIR:-/tmp}/lasso-dmg.XXXXXX")"
scratch="$(mktemp -d "${TMPDIR:-/tmp}/lasso-dmg-work.XXXXXX")"
mountpoint=""

cleanup() {
	[ -n "$mountpoint" ] && hdiutil detach "$mountpoint" -quiet 2>/dev/null || true
	rm -rf "$staging" "$scratch"
}
trap cleanup EXIT

# ---- contents ---------------------------------------------------------

cp -R "$APP" "$staging/"
ln -s /Applications "$staging/Applications"

# The background lives in a dot-directory so Finder does not show it as a file
# sitting in the window it is the background of.
mkdir -p "$staging/.background"
(
	cd "$REPO_ROOT/scripts/dmg-background"
	GOWORK=off go run . -out "$scratch/background.png" -scale 1 >/dev/null
	GOWORK=off go run . -out "$scratch/background@2x.png" -scale 2 >/dev/null
) || die "could not draw the background"

# One .tiff carrying both resolutions. Finder picks whichever the display
# needs, so the window is not soft on a retina screen and not oversized on
# anything else.
tiffutil -cathidpicheck "$scratch/background.png" "$scratch/background@2x.png" \
	-out "$staging/.background/background.tiff" >/dev/null 2>&1 \
	|| die "could not combine the background images"

# ---- writable image, laid out by Finder, then compressed --------------

rw="$scratch/rw.dmg"
# Room for the app plus the slack Finder needs to write its .DS_Store.
size_mb=$(( $(du -sm "$staging" | awk '{print $1}') + 80 ))

hdiutil create -volname "$VOLUME" -srcfolder "$staging" -fs HFS+ \
	-format UDRW -size "${size_mb}m" -quiet "$rw" \
	|| die "could not create the writable image"

mountpoint="$(hdiutil attach "$rw" -nobrowse -noautoopen 2>/dev/null \
	| sed -n 's|.*\(/Volumes/.*\)$|\1|p' | tail -1)"
[ -n "$mountpoint" ] && [ -d "$mountpoint" ] || die "could not mount the writable image"

# Finder needs a moment after the mount before it will answer about the window.
sleep 2

osascript - "$VOLUME" "$WINDOW_W" "$WINDOW_H" "$APP_X" "$APPS_X" "$ICON_Y" "$ICON_SIZE" <<'APPLESCRIPT' >/dev/null || die "Finder would not lay out the window"
on run argv
	set volumeName to item 1 of argv
	set winW to (item 2 of argv) as integer
	set winH to (item 3 of argv) as integer
	set appX to (item 4 of argv) as integer
	set appsX to (item 5 of argv) as integer
	set iconY to (item 6 of argv) as integer
	set iconSize to (item 7 of argv) as integer

	tell application "Finder"
		tell disk volumeName
			open
			set current view of container window to icon view
			set toolbar visible of container window to false
			set statusbar visible of container window to false
			-- Position is arbitrary; only the size has to match the picture.
			set the bounds of container window to {200, 140, 200 + winW, 140 + winH}

			set opts to the icon view options of container window
			set arrangement of opts to not arranged
			set icon size of opts to iconSize
			set text size of opts to 12
			set background picture of opts to file ".background:background.tiff"

			set position of item "Lasso.app" of container window to {appX, iconY}
			set position of item "Applications" of container window to {appsX, iconY}

			-- Nudge Finder into flushing the layout to .DS_Store. Without the
			-- close/open it frequently writes nothing and the DMG ships bare.
			update without registering applications
			delay 1
			close
			open
			delay 1
		end tell
	end tell
end run
APPLESCRIPT

# Let Finder finish writing .DS_Store before the volume goes away.
sync
sleep 2

# The script above leaves the window open, and Finder can go on holding the
# volume for a moment after it: 0.2.4's first build failed right here. Ask
# again rather than fail the release, and never force it, which could cut off
# the .DS_Store write the layout depends on.
for attempt in 1 2 3 4 5; do
	hdiutil detach "$mountpoint" -quiet && break
	[ "$attempt" = 5 ] && die "could not unmount the writable image"
	sleep 2
done
mountpoint=""

rm -f "$OUT"
hdiutil convert "$rw" -format UDZO -imagekey zlib-level=9 -quiet -o "$OUT" \
	|| die "could not compress the image"

printf 'Built %s\n' "$OUT"
du -h "$OUT" | awk '{printf "  %s\n", $1}'
