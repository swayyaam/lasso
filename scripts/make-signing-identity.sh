#!/usr/bin/env bash
#
# Makes the "Lasso Signing" certificate that releases are signed with, once.
#
# macOS keeps what a person allowed an app — Full Disk Access, keychain items,
# the Downloads folder — against the app's signing identity. Signed ad hoc,
# that identity is a hash of one build, so every update was a new app and
# every permission had to be given again. Signed with this certificate, every
# release has the same identity and keeps them. See scripts/sign-app.sh.
#
# It is self-signed: free, and no Apple account. Gatekeeper does not trust it,
# exactly as it does not trust an ad-hoc signature, so a first download shows
# the same "cannot check it" prompt as before. Trust is not the point; staying
# the same is.
#
# THE KEY IS THE IDENTITY. Lose it and the next release is a different app to
# macOS: every user gives Lasso its permissions once more, and the updater —
# which refuses an update signed by anyone else — sends them to the DMG. Back
# it up: Keychain Access → login → My Certificates → "Lasso Signing" → Export,
# as a .p12 with a password, somewhere that is not this Mac.
#
# It never replaces a certificate that already exists, for the same reason.
#
#   LASSO_SIGN_KEYCHAIN  a keychain file to put it in instead of the login one
#
# Usage: scripts/make-signing-identity.sh

set -euo pipefail

NAME="Lasso Signing"

die() { printf '\nerror: %s\n' "$*" >&2; exit 1; }

keychain="${LASSO_SIGN_KEYCHAIN:-$HOME/Library/Keychains/login.keychain-db}"
[ -f "$keychain" ] || die "no keychain at $keychain"

if security find-identity -p codesigning "$keychain" 2>/dev/null | grep -q "\"$NAME\""; then
	die "\"$NAME\" is already in $keychain. Replacing it would give Lasso a new identity,
  and every user would have to allow it everything again. Nothing was changed"
fi

work="$(mktemp -d "${TMPDIR:-/tmp}/lasso-identity.XXXXXX")"
trap 'rm -rf "$work"' EXIT
chmod 700 "$work"

cat > "$work/cert.cnf" <<EOF
[req]
distinguished_name = dn
x509_extensions = ext
prompt = no
[dn]
CN = $NAME
[ext]
basicConstraints = critical,CA:false
keyUsage = critical,digitalSignature
extendedKeyUsage = critical,codeSigning
EOF

# Twenty years: a certificate that expired would be one more new identity.
openssl req -x509 -newkey rsa:2048 -nodes -days 7300 -config "$work/cert.cnf" \
	-keyout "$work/key.pem" -out "$work/cert.pem" 2>/dev/null \
	|| die "openssl could not make the certificate"

# The key never leaves the keychain but for this moment, inside a folder only
# this user can read, under a password nobody keeps. The older PKCS#12
# algorithms are the ones macOS's importer reads; OpenSSL 3's defaults fail
# with "MAC verification failed".
password="$(openssl rand -hex 24)"
openssl pkcs12 -export -inkey "$work/key.pem" -in "$work/cert.pem" -name "$NAME" \
	-keypbe PBE-SHA1-3DES -certpbe PBE-SHA1-3DES -macalg sha1 \
	-passout "pass:$password" -out "$work/identity.p12" 2>/dev/null \
	|| die "openssl could not package the certificate"

# -T lets codesign use the key without asking each time.
security import "$work/identity.p12" -k "$keychain" -P "$password" -T /usr/bin/codesign >/dev/null \
	|| die "could not add it to $keychain"

sha1="$(security find-identity -p codesigning "$keychain" | awk -v n="\"$NAME\"" '$0 ~ n { print $2; exit }')"
printf 'Made "%s" in %s\n' "$NAME" "$keychain"
printf '  SHA-1 %s\n' "$sha1"
printf '  Back it up now: Keychain Access → My Certificates → "%s" → Export.\n' "$NAME"
printf '  Without it, the next release is a new app to macOS.\n'
