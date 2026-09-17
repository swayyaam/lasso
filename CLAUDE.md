# Lasso — project conventions

Lasso is a minimal macOS GUI over `yt-dlp`. The default experience is "paste a
link, pick a quality, download"; everything else hides behind progressive
disclosure. Target is macOS only — Apple Silicon first, Intel second.

## Git rules

These are not negotiable. Read them before you commit.

- **Conventional Commits**: `feat:`, `fix:`, `chore:`, `test:`, `docs:`. Keep
  messages short and specific.
- **No attribution of any kind in commit messages.** No `Co-Authored-By:`
  trailers, no "Generated with Claude Code" lines, no tool credits. A commit
  message contains only the message.
- **Never force-push, never rewrite pushed history, never change the remote URL.**
  The remote is `https://github.com/swayyaam/lasso.git` and it stays that way.
- **If a push fails** — auth, rejected, file too large, anything — stop and
  report the exact error verbatim. Do not invent a workaround, do not retry with
  different flags, do not rewrite history to get around it.
- **After every phase**: `make test` and `make lint` must pass, then commit and
  push. Never commit a red tree.
- **Before committing**, run `git status` and confirm no binary or large file is
  staged. The sidecar binaries are ~330 MB and must never enter the repo.

## Layout

```
apps/desktop/        Wails app: main.go, bindings, frontend/
packages/core/       Go: arg builder, metadata fetch, progress parser, queue
packages/binaries/   Go: locate/install/verify/update sidecar binaries
packages/presets/    Go: built-in + user presets, JSON persistence
packages/ui/         React: shared components + design tokens
scripts/             fetch-binaries.sh and build helpers
docs/design.md       The visual design system. Do not modify it.
```

Go modules are wired together with `go.work`; JS packages with pnpm workspaces
and Turborepo.

**`packages/core` must never import Wails.** It stays testable on its own.
Anything the UI needs crosses the boundary as a plain type that `apps/desktop`
adapts into a Wails event.

## Implementation rules

- Invoke yt-dlp with `exec.CommandContext` and an argument slice. **Never build a
  shell string, never use `sh -c`.** URLs and filename templates are untrusted.
- The arg builder is a pure function: `Options -> []string`. Unit test it
  thoroughly — every advanced option, every preset. Coverage here should stay high.
- Parse progress from `--newline` plus a custom `--progress-template` that emits
  JSON lines. Never scrape human-readable output.
- Cancelling must kill the whole process group — yt-dlp spawns ffmpeg, and
  orphans are a bug. Set `Setpgid` and signal the negative PID.
- Translate yt-dlp errors into plain language for the common cases (unavailable
  video, login/age restriction, network failure, unsupported URL). Keep the raw
  log behind a "details" toggle.
- No telemetry. No network calls except yt-dlp's own and the binary updater.

## Binary handling

- Versions are pinned with checksums in `binaries.lock.json`.
  `scripts/fetch-binaries.sh` downloads and verifies them into
  `apps/desktop/build/bin/<platform>/`. **Binaries are never committed.**
- yt-dlp is the **onedir** build (`yt-dlp_macos.zip`), not the onefile
  `yt-dlp_macos`. Onefile re-extracts a 37 MB archive on every exec (~5.8 s per
  invocation); onedir starts in ~0.16 s once warm. Do not "simplify" this back
  to the single file.
- On first launch, copy the binaries to `~/Library/Application Support/Lasso/bin/`,
  `chmod +x`, strip `com.apple.quarantine`, and ad-hoc codesign anything whose
  signature does not validate. **Always execute from there, never from inside
  the .app bundle.**
- Pass `--ffmpeg-location` explicitly and put deno on the yt-dlp subprocess PATH.
  Never rely on the system PATH.
- After `yt-dlp -U`, re-run the same fixups (chmod, quarantine strip, signature
  check) against the updated copy.
- On startup, verify each binary runs (`--version`) and show a clear, actionable
  error if one does not.

## Wails bindings

`apps/desktop` is the only package that imports Wails. It adapts core,
binaries and presets into bound methods and events; the logic stays in those
packages.

- **Payload types are the core types.** Bound methods take and return
  `core.Options`, `core.Item`, `core.Metadata`, `binaries.Status` and so on
  directly. Do not introduce parallel DTOs — there should be one definition of
  each shape, and the TypeScript is generated from it.
- **Regenerate bindings** with `wails generate module` in `apps/desktop` after
  changing a bound method signature or any type it mentions.
  `frontend/wailsjs/` is generated but committed, so a fresh clone can
  typecheck without running Wails.
- **Avoid `time.Time` in bound types.** Wails cannot model it and emits `any`.
  `core.Item.AddedAt` is Unix milliseconds for this reason.
- **Event names live in `events.go`** and reach the frontend through
  `App.Events()`. Never hardcode an event string in TypeScript.
- **Thumbnails are served by the app's own asset handler** at `/thumbs/<hash>.jpg`,
  not from a file:// path, which the webview cannot load from the asset scheme.
  The handler matches a strict sha256 filename pattern; that is what prevents a
  crafted request escaping the cache directory.
- **`frontend/dist` must exist** for `go build` to work, because `main.go`
  embeds it. A committed `.gitkeep` keeps it present on a fresh clone.

## Performance requirements

These are standing requirements, not optimisations to consider later.

- **Throttle progress events per queue item.** yt-dlp emits far faster than
  anyone can read, and a large playlist has every item emitting at once.
  `core.ProgressEmitter` coalesces to `DefaultProgressInterval` (150 ms) per
  item. Terminal states must bypass the throttle: always `Flush` when a
  download finishes, fails or is cancelled, or the UI freezes short of 100%.
- **Virtualise the queue list** (phase E). A playlist can add hundreds of rows;
  only the visible ones should be mounted.
- **Lazy-load thumbnails at display size** (phase E, enabled by core). Never
  point an `<img>` at a remote URL. Ask the backend for the width you are
  rendering: `core.BestThumbnail` picks the closest source and
  `core.ThumbnailCache` fetches, downscales once and caches it.

## Network policy

Lasso makes exactly three kinds of outbound request: yt-dlp's own traffic, the
binary updater, and thumbnail fetches through `core.ThumbnailCache`. There is
no telemetry and no analytics.

The thumbnail cache is the only one written in Go, and it is deliberately the
only place that fetches remote images — the webview never talks to a CDN
directly. Thumbnail URLs come from yt-dlp metadata, which the site being
downloaded from ultimately controls, so the cache requires https and refuses to
connect to loopback or private addresses. Do not relax either check.

## Updating yt-dlp

Lasso updates yt-dlp itself; it does not shell out to `yt-dlp -U`.

The bundled build is the PyInstaller **onedir** release, and yt-dlp's own
updater refuses it outright:

    ERROR: Auto-update is not supported for unpackaged executables

The onefile build does support `-U`, but costs ~5.8s of bootstrap on every
invocation against 0.16s warm, which is why Lasso ships onedir. So
`Manager.UpdateYtDlp` fetches the same asset the lock file pins, verifies it
against the release's published SHA2-256SUMS, stages it, proves it runs, and
only then swaps the folder. A failed or interrupted update leaves the working
copy in place.

After unpacking, `fixupTree` re-applies every fixup across the whole folder:
directory permissions, the executable bit, quarantine removal, and a signature
check on all ~107 Mach-O images. Signatures are verified and only repaired
where verification fails — re-signing valid images would replace yt-dlp's own
signatures with weaker ad-hoc ones for nothing. Nested images are signed before
the launcher, because signing a component invalidates a signature covering it.

`needsInstall` is what stops the bundled copy rolling back a self-update: for
yt-dlp, the bundle only wins when it is newer, which its date-based versions
make a string comparison.

## App icon

`apps/desktop/build/appicon.png` is the only icon input. `make build` hands it
to Wails, which regenerates `Contents/Resources/iconfile.icns` on every build,
so swapping the icon means replacing that one PNG and rebuilding — nothing else
to touch.

Apple's template wants roughly 824px of artwork centred in a 1024x1024 canvas.
Artwork that fills the canvas edge to edge renders slightly larger than its
neighbours in the Dock and does not share the system corner radius.

## Deferred: distribution

- **Thin the universal slices.** yt-dlp ships universal2: 107 of its files
  carry both arches, which is 55 MB of x86_64 that an arm64 build never runs.
  `lipo -thin arm64` across them takes yt-dlp from 124 MB to 67 MB and the
  bundle from 328 MB to 271 MB. Two catches: `lipo` strips code signatures, so
  every thinned image needs ad-hoc re-signing afterwards, and an update
  re-installs the universal build, so the saving applies to the shipped bundle
  rather than the installed copy. ffmpeg, ffprobe and deno are already
  arm64-only.
- Code signing and notarisation are not set up. The build is ad-hoc signed
  only, so a downloaded copy is quarantined.

## Design

`docs/design.md` is the visual language and is **read-only**. It documents
Linear's marketing site, so map it onto app screens rather than copying page
patterns: use the small end of the type scale, skip hero/pricing/footer
constructs.

Decisions made on top of it, for this app:

- **Dark only.** The system documents no light theme; Lasso does not ship one.
- **One added semantic colour**: an app-only error red, used solely for failed
  downloads and blocking startup errors. Everything else obeys the "no second
  chromatic accent" rule — lavender stays scarce (brand mark, primary CTA, focus
  ring, link emphasis).
- **Compressed density**: same tokens, smaller rungs — 16 px card padding,
  `body-sm` 14 px as the workhorse, `caption` 12 px for meta.
- **Two-pane window**: composer left, queue right; stacks below ~900 px.

Colours, spacing, radii and type come **only** from the token config in
`packages/ui`. No hardcoded hex in components — this is lint-enforced.

## Commands

```
make setup            toolchain check (incl. wails doctor), pnpm install, fetch binaries
make dev              wails dev
make build            build Lasso.app
make test             Go tests across all modules + JS tests
make lint             go vet + gofmt + JS lint
make fetch-binaries   re-download and verify sidecars
```
