# Lasso

A minimal macOS app for downloading video and audio with
[yt-dlp](https://github.com/yt-dlp/yt-dlp).

Paste a link, pick a quality, download. Everything yt-dlp can do is still
reachable — subtitles, SponsorBlock, codec selection, playlist ranges, filename
templates — it just stays behind an advanced drawer until you want it.

Lasso bundles its own yt-dlp, ffmpeg, ffprobe and deno. It does not use, or
need, anything installed on your system.

> **Status:** in development. Phase A (scaffold) is complete; the app itself is
> being built phase by phase. `make dev` starts working in phase D.

## Requirements

- macOS (Apple Silicon; Intel support is a follow-up)
- [Go](https://go.dev/dl/) 1.24+
- [Node](https://nodejs.org/) 20+
- [pnpm](https://pnpm.io/) 10+
- [Wails](https://wails.io/) v2 — `go install github.com/wailsapp/wails/v2/cmd/wails@latest`

Make sure `$(go env GOPATH)/bin` is on your `PATH` so the `wails` command is
found.

## Quick start

```bash
git clone https://github.com/swayyaam/lasso.git
cd lasso
make setup
make dev
```

`make setup` verifies your toolchain (including `wails doctor`), installs JS
dependencies, and downloads the sidecar binaries.

## Sidecar binaries

Versions are pinned with SHA256 checksums in
[`binaries.lock.json`](binaries.lock.json). `scripts/fetch-binaries.sh`
downloads each one, verifies its digest, and installs it into
`apps/desktop/build/bin/<platform>/`, where the Wails build picks it up.

**Binaries are never committed.** The fetch is idempotent — a per-binary stamp
records what is already installed, and archives are cached under `.cache/`.

| Binary | Version | Source |
|---|---|---|
| yt-dlp | 2026.08.19 | GitHub release, `yt-dlp_macos.zip` (universal2, onedir) |
| ffmpeg | 9.0.1 | [ffmpeg.martin-riedl.de](https://ffmpeg.martin-riedl.de/) |
| ffprobe | 9.0.1 | [ffmpeg.martin-riedl.de](https://ffmpeg.martin-riedl.de/) |
| deno | 2.9.6 | GitHub release |

Lasso uses the **onedir** yt-dlp build rather than the single-file one. The
onefile binary re-extracts a 37 MB archive on every invocation — about 5.8
seconds each time — while onedir starts in roughly 0.16 seconds once macOS has
scanned it. That first scan is warmed during install so you never see it.

To fetch for the other architecture:

```bash
./scripts/fetch-binaries.sh --platform darwin-amd64
```

## Layout

```
apps/desktop/        Wails app (Go backend + React frontend)
packages/core/       yt-dlp arg builder, metadata, progress parsing, queue
packages/binaries/   sidecar install, verification, updates
packages/presets/    built-in and user presets
packages/ui/         shared React components + design tokens
scripts/             fetch-binaries.sh
docs/design.md       the visual design system
```

`packages/core` has no Wails dependency, so it can be tested on its own.

## Development

```bash
make help             list every target
make test             Go tests across all modules + JS tests
make lint             go vet + gofmt + JS lint
make fetch-binaries   re-download and verify sidecars
make clean            remove build output and fetched binaries
```

## Licensing note

The bundled ffmpeg is a `--enable-gpl --enable-version3` build, so a
distributed Lasso app falls under GPLv3. Worth settling before any public
release.
