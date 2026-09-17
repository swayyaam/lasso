# Lasso

A minimal macOS app for downloading video and audio with
[yt-dlp](https://github.com/yt-dlp/yt-dlp).

Paste a link, pick a quality, download. Everything yt-dlp can do is still
reachable — subtitles, SponsorBlock, codec selection, playlist ranges, filename
templates — it just stays behind an advanced drawer until you want it.

Lasso bundles its own yt-dlp, ffmpeg, ffprobe and deno. It does not use, or
need, anything installed on your system.

> **Status:** working, unsigned. Everything in the feature list below is built
> and runs. The app is not code-signed or notarised yet, so macOS needs one
> extra click the first time — see [Installing](#installing).

## Requirements

- macOS (Apple Silicon; Intel support is a follow-up)
- [Go](https://go.dev/dl/) 1.24+
- [Node](https://nodejs.org/) 20+
- [pnpm](https://pnpm.io/) 10+
- [Wails](https://wails.io/) v2 — `go install github.com/wailsapp/wails/v2/cmd/wails@latest`

Make sure `$(go env GOPATH)/bin` is on your `PATH` so the `wails` command is
found.

## Installing

Lasso is not code-signed or notarised, so macOS will not open it on the first
try. This is expected for an unsigned app and takes one extra click.

1. Open `Lasso.dmg` and drag **Lasso** into **Applications**.
2. Open Lasso from Applications. macOS will refuse, saying it cannot verify the
   developer.
3. Open **System Settings → Privacy & Security**, scroll to the Security
   section, and click **Open Anyway** next to the message about Lasso.
4. Open Lasso again and confirm.

macOS remembers the decision, so this is only needed once.

The first launch copies the bundled copies of yt-dlp, ffmpeg, ffprobe and deno
into `~/Library/Application Support/Lasso/bin` and lets macOS scan them, which
takes a few seconds. Lasso shows a setup screen while that happens.

## Building from source

```bash
git clone https://github.com/swayyaam/lasso.git
cd lasso
make setup
make dev
```

`make setup` verifies your toolchain (including `wails doctor`), installs JS
dependencies, and downloads the sidecar binaries. `make build` produces
`apps/desktop/build/bin/Lasso.app`, and `make dmg` packages it.

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
make build            build Lasso.app
make dmg              build Lasso.app and package it as a DMG
make test             Go tests across all modules + JS tests
make lint             go vet + gofmt + JS lint + the design-token check
make fetch-binaries   re-download and verify sidecars
make clean            remove build output and fetched binaries
```

### App icon

`apps/desktop/build/appicon.png` is the only icon input; `make build`
regenerates the `.icns` from it. It must be square, and macOS expects the
artwork to sit inside about 824px of a 1024x1024 canvas rather than filling it.
To prepare artwork that is the wrong shape or size:

```bash
cd scripts/icon && go run . -in artwork.png -out ../../apps/desktop/build/appicon.png
```

That fits the image without stretching it and centres it on a transparent
canvas. A detailed icon turns to mush at 16 and 32px; drop a simplified mark at
`apps/desktop/build/appicon-small.png` and those two sizes will use it instead.

## Licensing note

The bundled ffmpeg is a `--enable-gpl --enable-version3` build, so a
distributed Lasso app falls under GPLv3. Worth settling before any public
release.
