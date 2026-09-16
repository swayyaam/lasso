#!/usr/bin/env bash
#
# Downloads the sidecar binaries pinned in binaries.lock.json, verifies their
# SHA256, and places them where the Wails build picks them up
# (apps/desktop/build/bin/<platform>/).
#
# The binaries are never committed. Re-running is cheap: a per-binary stamp
# records the version + digest already installed, and archives are cached.
#
# Usage:
#   scripts/fetch-binaries.sh                     # host platform
#   scripts/fetch-binaries.sh --platform darwin-amd64
#   scripts/fetch-binaries.sh --force             # re-install even if stamped
#   scripts/fetch-binaries.sh --all               # every platform in the lock

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
LOCK="$REPO_ROOT/binaries.lock.json"
CACHE_DIR="$REPO_ROOT/.cache/binaries"
BIN_BASE="$REPO_ROOT/apps/desktop/build/bin"

die() { printf '\nerror: %s\n' "$*" >&2; exit 1; }
info() { printf '  %s\n' "$*"; }

host_platform() {
	[ "$(uname -s)" = "Darwin" ] || die "Lasso targets macOS only (found $(uname -s))."
	case "$(uname -m)" in
		arm64) echo "darwin-arm64" ;;
		x86_64) echo "darwin-amd64" ;;
		*) die "unsupported architecture: $(uname -m)" ;;
	esac
}

FORCE=0
PLATFORMS=()
while [ $# -gt 0 ]; do
	case "$1" in
		--force) FORCE=1; shift ;;
		--platform) [ $# -ge 2 ] || die "--platform needs a value"; PLATFORMS+=("$2"); shift 2 ;;
		--all)
			while IFS= read -r p; do PLATFORMS+=("$p"); done < <(node -e '
				process.stdout.write(require(process.argv[1]).platforms.join("\n"))' "$LOCK")
			shift ;;
		-h|--help) sed -n '3,20p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
		*) die "unknown argument: $1" ;;
	esac
done
[ ${#PLATFORMS[@]} -gt 0 ] || PLATFORMS=("$(host_platform)")

for cmd in node curl shasum unzip; do
	command -v "$cmd" >/dev/null 2>&1 || die "required command not found: $cmd"
done
[ -f "$LOCK" ] || die "missing $LOCK"

# Emits one TSV row per binary: name, version, layout, entrypoint, url, sha256, archive
lock_rows() {
	node -e '
		const lock = require(process.argv[1]);
		const plat = process.argv[2];
		if (!lock.platforms.includes(plat)) {
			console.error(`platform "${plat}" is not in the lock file (have: ${lock.platforms.join(", ")})`);
			process.exit(2);
		}
		for (const [name, b] of Object.entries(lock.binaries)) {
			const a = b.artifacts[plat];
			if (!a) { console.error(`no artifact for ${name}/${plat}`); process.exit(2); }
			process.stdout.write([name, b.version, b.layout, b.entrypoint, a.url, a.sha256, a.archive].join("\t") + "\n");
		}
	' "$LOCK" "$1"
}

verify_sha256() {
	local file="$1" want="$2" got
	got="$(shasum -a 256 "$file" | awk '{print $1}')"
	[ "$got" = "$want" ] || die "checksum mismatch for $(basename "$file")
    expected: $want
    actual:   $got
  The pinned artifact changed or the download was corrupted. Not installing."
}

install_one() {
	local plat="$1" name="$2" version="$3" layout="$4" entrypoint="$5" url="$6" sha="$7" archive="$8"
	local bin_dir="$BIN_BASE/$plat"
	local stamp="$bin_dir/.stamps/$name"
	local target

	case "$layout" in
		file) target="$bin_dir/$entrypoint" ;;
		dir)  target="$bin_dir/$name/$entrypoint" ;;
		*) die "unknown layout \"$layout\" for $name" ;;
	esac

	if [ "$FORCE" -eq 0 ] && [ -f "$stamp" ] && [ -x "$target" ] \
		&& [ "$(cat "$stamp")" = "$version $sha" ]; then
		info "$name $version — already installed"
		return 0
	fi

	local cached="$CACHE_DIR/$name-$version-$plat.$archive"
	mkdir -p "$CACHE_DIR"
	if [ -f "$cached" ] && shasum -a 256 "$cached" | awk '{print $1}' | grep -qx "$sha"; then
		info "$name $version — using cached archive"
	else
		info "$name $version — downloading"
		rm -f "$cached"
		curl --fail --location --show-error --silent --progress-bar \
			--retry 3 --retry-delay 2 --connect-timeout 20 \
			-o "$cached.part" "$url" || die "download failed: $url"
		mv "$cached.part" "$cached"
	fi
	verify_sha256 "$cached" "$sha"

	local tmp
	tmp="$(mktemp -d "${TMPDIR:-/tmp}/lasso-bin.XXXXXX")"
	trap 'rm -rf "$tmp"' RETURN
	unzip -qq "$cached" -d "$tmp" || die "could not unpack $cached"

	mkdir -p "$bin_dir/.stamps"
	case "$layout" in
		file)
			[ -f "$tmp/$entrypoint" ] || die "$name archive did not contain \"$entrypoint\""
			rm -f "$target"
			mv "$tmp/$entrypoint" "$target"
			;;
		dir)
			[ -f "$tmp/$entrypoint" ] || die "$name archive did not contain \"$entrypoint\" at its root"
			rm -rf "$bin_dir/$name"
			mv "$tmp" "$bin_dir/$name"
			# mktemp -d creates 0700; the bundled copy must be world-readable.
			chmod 755 "$bin_dir/$name"
			trap - RETURN
			;;
	esac

	chmod +x "$target"
	# Defensive only: curl does not set com.apple.quarantine, but a proxy or a
	# hand-placed archive might. The authoritative strip happens at install time
	# in packages/binaries.
	xattr -d com.apple.quarantine "$target" 2>/dev/null || true

	printf '%s %s' "$version" "$sha" > "$stamp"
	info "$name $version — installed"
}

# The manifest travels with the binaries into the .app bundle. packages/binaries
# reads it at runtime to decide what to install and to stamp what it installed,
# so the lock file stays the single source of truth for versions.
write_manifest() {
	local plat="$1" dest="$BIN_BASE/$1/manifest.json"
	node -e '
		const lock = require(process.argv[1]);
		const plat = process.argv[2];
		const out = { schemaVersion: 1, platform: plat, binaries: {} };
		for (const [name, b] of Object.entries(lock.binaries)) {
			out.binaries[name] = { version: b.version, layout: b.layout, entrypoint: b.entrypoint };
		}
		process.stdout.write(JSON.stringify(out, null, 2) + "\n");
	' "$LOCK" "$plat" > "$dest"
	info "manifest.json — written"
}

for plat in "${PLATFORMS[@]}"; do
	printf '\nFetching sidecar binaries for %s\n' "$plat"
	while IFS=$'\t' read -r name version layout entrypoint url sha archive; do
		[ -n "$name" ] || continue
		install_one "$plat" "$name" "$version" "$layout" "$entrypoint" "$url" "$sha" "$archive"
	done < <(lock_rows "$plat")
	write_manifest "$plat"
done

printf '\nAll sidecar binaries verified against binaries.lock.json.\n'
