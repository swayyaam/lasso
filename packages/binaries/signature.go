package binaries

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"strings"

	"github.com/ProtonMail/go-crypto/openpgp"
)

// ytDlpSigningKey is yt-dlp's release signing key, as published at
// https://github.com/yt-dlp/yt-dlp/blob/master/public.key.
//
//go:embed ytdlp-signing-key.asc
var ytDlpSigningKey []byte

// ytDlpKeyFingerprint pins that key: "Simon Sawicki (yt-dlp signing key)".
// The file and the fingerprint must agree, so replacing one alone — by
// mistake or otherwise — fails every update rather than trusting a new key.
const ytDlpKeyFingerprint = "AC0CBBE6848D6A873464AF4E57CF65933B5A7581"

// sumsSigAsset is the detached signature yt-dlp publishes over sumsAsset.
const sumsSigAsset = sumsAsset + ".sig"

// errNotSigned is a checksum list the pinned key did not sign.
var errNotSigned = errors.New("not signed by the pinned key")

// verifySums checks that sums carries a valid detached signature from the
// key whose primary fingerprint is fingerprint.
//
// The checksum list and the archive come from the same GitHub release, so a
// checksum alone proves only that the download was not damaged on the way.
// The signature is what proves the release is yt-dlp's: someone able to
// replace the archive on GitHub could replace the list beside it, but not sign
// it with a key that never leaves yt-dlp's maintainers.
func verifySums(armoredKey []byte, fingerprint string, sums, signature []byte) error {
	keyring, err := openpgp.ReadArmoredKeyRing(bytes.NewReader(armoredKey))
	if err != nil {
		return fmt.Errorf("reading the signing key: %w", err)
	}
	if len(keyring) != 1 {
		return fmt.Errorf("expected one signing key, found %d", len(keyring))
	}
	got := strings.ToUpper(fmt.Sprintf("%X", keyring[0].PrimaryKey.Fingerprint))
	if got != strings.ToUpper(fingerprint) {
		return fmt.Errorf("the signing key is %s, not the pinned %s", got, fingerprint)
	}

	if _, err := openpgp.CheckDetachedSignature(keyring, bytes.NewReader(sums), bytes.NewReader(signature), nil); err != nil {
		return fmt.Errorf("%w: %v", errNotSigned, err)
	}
	return nil
}
