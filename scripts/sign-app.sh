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
# Which signature: a certificate named "Lasso Signing" when the keychain has
# one (scripts/make-signing-identity.sh makes it), ad hoc otherwise. Neither is
# trusted by Gatekeeper — that needs a paid Developer ID — and both get the
# same "cannot check it" prompt on a first download. What differs is
# identity. macOS keeps what a person allowed an app (Full Disk Access,
# keychain items, the Downloads folder) against its designated requirement.
# Ad hoc, that is a hash of this one build, so every release is a stranger
# and every permission must be given again. With the certificate it is
# 'identifier "com.swayyaam.lasso" and certificate leaf = H"…"', which every
# release signed with the same key satisfies.
#
# A release must never go out ad hoc, so make-release-assets.sh runs this
# with LASSO_REQUIRE_IDENTITY=1. A local build may: it only costs the person
# running it their own permissions.
#
#   LASSO_SIGN_IDENTITY     a certificate's SHA-1 or name, instead of looking
#   LASSO_SIGN_KEYCHAIN     a keychain file to use instead of the search list
#   LASSO_REQUIRE_IDENTITY  1 to fail rather than sign ad hoc
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
identity="${LASSO_SIGN_IDENTITY:-}"
if [ -z "$identity" ]; then
	# Untrusted, so not among find-identity's "valid" ones: take the SHA-1 of
	# the first certificate by that name from the whole list.
	identity="$(security find-identity -p codesigning ${LASSO_SIGN_KEYCHAIN:+"$LASSO_SIGN_KEYCHAIN"} 2>/dev/null \
		| awk '/"Lasso Signing"/ { print $2; exit }')"
fi

keychain=()
[ -n "${LASSO_SIGN_KEYCHAIN:-}" ] && keychain=(--keychain "$LASSO_SIGN_KEYCHAIN")

if [ -n "$identity" ]; then
	codesign --force --sign "$identity" ${keychain[@]+"${keychain[@]}"} "$APP" >/dev/null 2>&1 \
		|| die "could not sign $APP with $identity"
	how="Lasso Signing"
elif [ "${LASSO_REQUIRE_IDENTITY:-}" = 1 ]; then
	die "no \"Lasso Signing\" certificate in the keychain. A release signed ad hoc makes every
  user give Lasso its permissions again. Make one with scripts/make-signing-identity.sh,
  or restore the one releases are signed with"
else
	codesign --force --sign - "$APP" >/dev/null 2>&1 \
		|| die "could not sign $APP"
	how="ad-hoc"
fi

# Verified the way Gatekeeper verifies it, so a broken seal fails the build
# here rather than on a stranger's Mac.
if ! output="$(codesign --verify --deep --strict --verbose=2 "$APP" 2>&1)"; then
	printf '%s\n' "$output" >&2
	die "the signature does not verify; this build would open as \"damaged\""
fi

printf 'Signed %s (%s)\n' "$(basename "$APP")" "$how"
printf '  seal verifies; downloads will show the “unidentified developer” prompt, not “damaged”\n'
if [ "$how" = ad-hoc ]; then
	printf '  ad hoc: macOS will treat this build as a new app and forget its permissions\n'
else
	printf '  %s\n' "$(codesign -d -r- "$APP" 2>&1 | sed -n 's/^designated => //p')"
fi
