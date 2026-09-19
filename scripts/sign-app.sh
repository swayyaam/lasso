#!/usr/bin/env bash
#
# Seals the built Lasso.app, and refuses to finish if the seal does not hold.
#
# This has to be the last thing `make build` does. Wails signs the bundle as
# part of its own build, but make-icon.sh then replaces Contents/Resources/
# iconfile.icns and bundle-binaries.sh writes 328 MB into
# Contents/Resources/bin — both after the fact. A bundle whose contents changed
# since it was signed has a broken seal, not a missing one.
#
# That distinction is the whole point. macOS treats the two differently:
#
#   no signature / broken seal  ->  "Lasso is damaged and can't be opened."
#                                   No way past it but the terminal.
#   valid but untrusted         ->  "Apple cannot check it for malicious
#                                   software", with Open Anyway in System
#                                   Settings.
#
# Only the second is recoverable by someone who just downloaded the app, and
# it is the one the README describes. v0.1.0 shipped with the first, because
# nothing checked.
#
# The signature is ad-hoc — there is no Developer ID yet — which is enough to
# make the bundle coherent and to satisfy Apple Silicon's requirement that
# every executable be signed at all. It is not enough to be trusted, and is not
# meant to be.
#
# Usage: scripts/sign-app.sh [path/to/Lasso.app]

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
APP="${1:-$REPO_ROOT/apps/desktop/build/bin/Lasso.app}"

die() { printf '\nerror: %s\n' "$*" >&2; exit 1; }

[ -d "$APP" ] || die "no app bundle at $APP — run \`make build\` first"
[ -x "$APP/Contents/MacOS/Lasso" ] || die "$APP has no executable; the build did not finish"

# --force replaces the stale seal Wails left behind. Deliberately not --deep:
# the bundled yt-dlp ships its own valid signatures across ~107 Mach-O images,
# and re-signing those would swap real signatures for weaker ad-hoc ones to no
# purpose. Sealing the outer bundle covers them as resources, which is what
# was missing.
codesign --force --sign - "$APP" >/dev/null 2>&1 \
	|| die "could not sign $APP"

# Verified the way Gatekeeper verifies it, so a broken seal fails the build
# here rather than on a stranger's Mac.
if ! output="$(codesign --verify --deep --strict --verbose=2 "$APP" 2>&1)"; then
	printf '%s\n' "$output" >&2
	die "the signature does not verify; this build would open as \"damaged\""
fi

printf 'Signed %s (ad-hoc)\n' "$(basename "$APP")"
printf '  seal verifies; downloads will show the “unidentified developer” prompt, not “damaged”\n'
