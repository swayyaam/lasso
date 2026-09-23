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
packages/history/    Go: the record of finished downloads, JSON persistence
packages/doctor/     Go: diagnoses why downloads fail, and repairs what it can
packages/updater/    Go: replaces Lasso with a newer release of itself
packages/ghrelease/  Go: reads GitHub release files politely, without the API
packages/ui/         React: shared components + design tokens
scripts/             fetch-binaries.sh and build helpers
DESIGN-webflow.md    The visual design system. Do not modify it.
docs/design.md       Superseded by the above; kept for history.
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

Lasso makes exactly four kinds of outbound request: yt-dlp's own traffic, the
yt-dlp updater, Lasso's own updater, and thumbnail fetches through
`core.ThumbnailCache`. There is no telemetry and no analytics.

Both updaters go through `packages/ghrelease`, and neither touches
`api.github.com` — see "Staying inside GitHub's allowance" below.

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

It learns the newest version from the redirect GitHub sends for
`/releases/latest` — `ghrelease.Client.LatestTag` reads the `Location` header
and never the page — and builds every download address from that tag. When the
tag matches what is installed, that one redirect is the whole cost.

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

`DESIGN-webflow.md` in the repo root is the visual language and is
**read-only**. It documents Webflow's marketing site, so map it onto app
screens rather than copying page patterns: use the small end of the type
scale, skip hero/pricing/footer constructs.

It replaced `docs/design.md`, which documented Linear's site and described a
dark app. That file is kept for history and is no longer the source of truth —
if the two disagree, this one wins.

Decisions made on top of it, for this app:

- **Light only.** The system is a white canvas with near-black ink. The window
  and the webview are set to Aqua so the traffic lights and any native control
  match it.
- **A near-white surface ladder.** The source has no grey steps: a card is
  canvas plus a hairline border. An app needs fills for hover, selection and
  inset panels, so `surface-1..4` step from canvas toward hairline.
- **Readable shades of the semantic accents.** `#00d722` green and `#ee1d36`
  red are surface-fill colours; as small text on white they fail contrast. Each
  has a `-strong` variant for text and a tinted surface. These are shades of
  the documented stops, not a sixth accent.
- **Compressed density**: the document's weights, tracking and shape system
  exactly; its marketing sizes taken at the small end. 16 px card padding,
  `body-sm` 14 px as the workhorse, `caption` 12.8 px at the signature 550
  weight for labels.
- **Two-pane window**: composer left, queue and history right; stacks below
  ~900 px.

Two rules from the document are easy to break and worth restating. The five
chromatic accents (purple / pink / blue / orange / green) are **surface fills,
never button backgrounds** — the conversion hierarchy is two-colour, near-black
for primary and white-on-hairline for secondary. And **nothing is a pill**:
buttons, chips and badges are 4 px, cards are 8 px, and full-round is reserved
for circular icon containers and scrollbar tracks.

Colours, spacing, radii and type come **only** from the token config in
`packages/ui`. No hardcoded hex in components — this is lint-enforced by
`scripts/check-tokens.mjs`.

## Components and icons

Most primitives in `packages/ui` are hand-rolled against the tokens. Three are
Radix, chosen where the hard part is behaviour rather than appearance:

- **Sheet** (Dialog) — focus trap, focus restore, inert background, scroll lock.
- **Dropdown** (Select) — typeahead, roving focus, scroll-into-view. A native
  `<select>` renders its popup with the OS's own chrome, which follows neither
  the tokens nor the shape system.
- **Tooltip** — dismissal rules, and not appearing on touch.

Anything else would be a styled div with extra weight, so it is not worth the
dependency.

Icons are lucide, and every one the app uses is named in `packages/ui/src/icons.ts`
rather than imported from lucide at the call site. That is what stops the same
action picking a different glyph in two places. Names describe the action
(`Retry`), not the picture (`RotateCcw`). They render at 14 px with
`strokeWidth={1.75}`: lucide's default 2 px stroke reads heavier than the
system's 400/500 type weights beside it.

## Music

Two options turn a video download into a music one, and both are off by
default.

**Tagging** adds `--parse-metadata "%(artist,title)s:(?P<meta_artist>.+?) - (?P<meta_title>.+)"`.
Reading *artist-then-title* is what makes it safe to apply unconditionally: a
site that states a real artist yields a string with no `" - "` in it, the
pattern does not match, nothing is overwritten, and `--embed-metadata` goes on
using the site's own fields. Only a bare video title gets split.

Do not replace this with a template like `%(artist)s:%(meta_artist)s`. A
template whose field is missing renders as the literal string `NA`, and that is
what lands in the file — tags reading `artist=NA` rather than no artist at all.

**Splitting** adds `--split-chapters` and the chapter output template. yt-dlp
cuts the tracks with the audio copied, which carries every tag across
unchanged, so all of an album's tracks arrive titled after the album.
`--postprocessor-args` cannot fix it either: it takes a fixed string and writes
it literally, so `%(section_title)s` ends up in the file as those characters.

So `core.Tagger` rewrites each track afterwards with the bundled ffmpeg —
`-map 0:a -map "0:v?" -c copy`, which keeps the cover art where there is one and
works where there is not. `ChapterTemplate` and `ChapterFile.Title` are a
contract: the template writes `NN - Title`, and Title parses it back by
rebuilding the prefix from the number yt-dlp reported, so a chapter genuinely
called "01 - Intro" survives. Change one and you change both.

A tagging failure never fails the download. The tracks exist and play; losing
them over a metadata rewrite would be a bad trade, so it lands as a notice.

**Audio quality.** `PickAudioOriginal` extracts the site's own stream and
changes only its container. Every other audio pick re-encodes, and re-encoding
a lossy stream loses a second time — asking for FLAC from a source that serves
Opus produces a genuine FLAC file several times the size carrying exactly the
same sound. `core.QualityOptions.LosslessAudio` is what lets the interface say
so instead of letting the green dot imply otherwise.

## Notifications

Notifications go through `UNUserNotificationCenter` from inside the process, so
they carry Lasso's bundle identifier and therefore Lasso's name and icon. The
earlier implementation shelled out to `osascript`, and every notification
arrived from **Script Editor** — that being the process AppleScript runs in.

Two things keep that working:

- The bundle check. `UNUserNotificationCenter` raises rather than returning an
  error when there is no `CFBundleIdentifier`, and a raise from Objective-C
  takes the process down. `go test` is exactly that case.
- The AppleScript fallback, kept for when the framework refuses. A notification
  under the wrong name beats none at all, and the doctor reports which one is
  in use rather than leaving an inexplicable banner.

`announce` posts from a goroutine: the queue's state callback must not block,
and the first notification waits on a permission prompt, which waits on a
person.

## Updating Lasso itself

`packages/updater` replaces the app in place. It matters more than
convenience: the app is not notarised, so a copy downloaded through a browser
is quarantined and has to be let past Gatekeeper by hand every time. An update
the app installs is never quarantined.

The downloaded asset is **the app without its helper programs**. They are
328 MB of the 329 MB bundle and change only when `binaries.lock.json` does, so
`Contents/Resources/bin` is emptied down to its `manifest.json` in
`Lasso-app.zip` and the installed copies are carried across during the update.
5.5 MB rather than 147.

That manifest is the safety catch. If the release pins different helper
versions, carrying the old ones across would leave someone on a build that
says it updated and did not, so the update is refused with `ErrHelpersChanged`
and the DMG is the way through.

**Every release publishes `latest.json`.** It is what the updater reads —
version, notes, and each file's size and SHA-256 — from
`releases/latest/download/latest.json`. `make release-assets NOTES=notes.md`
writes it through `cmd/release-manifest`, which uses the same
`updater.Manifest` type the app parses and refuses anything the app would. A
release without it cannot be installed from inside Lasso; the app says so and
points at the releases page.

The manifest never carries an address. Downloads come from
`<releases>/download/v<version>/`, and the version must match `\d+.\d+.\d+`
before it goes into that path, so a manifest cannot send the updater
anywhere else. **Keep publishing `SHA256SUMS` too**: Lasso 0.1.2 to 0.1.5 find
releases through the API and verify against it, and they need to be able to
update to whatever comes next.

Two things are easy to get wrong here, and one of them already shipped:

- **Reseal after carrying the helpers in.** They are written into a bundle
  that was signed without them, so the seal no longer matches and the app
  opens as "damaged". `prepare` re-signs and then verifies, and refuses to
  install anything that does not.
- **Swap with two renames on the same volume**, old copy kept until the new
  one is in place and moved back if it is not. `os.MkdirTemp` stages beside
  the bundle for exactly this reason — the system temp directory is usually
  another volume, where a rename is a copy and no longer atomic.

The old bundle stays until the next launch, because the process doing the
replacing is running out of it. `updater.CleanUp` removes it at startup, from
a derived path rather than a search.

### Staying inside GitHub's allowance

**Neither updater uses GitHub's API.** The REST API allows 60 unauthenticated
requests an hour per address — an address being a whole office as easily as
one person — and an update check is exactly the small, repeated request that
runs it down. None of it is needed: GitHub serves release files from its
download CDN at documented addresses, and those do not count against the
allowance. That was measured, not assumed: three downloads through
`releases/latest/download/` left the anonymous quota where it was.

- Lasso's own releases: `releases/latest/download/latest.json`, then
  `releases/download/v<version>/Lasso-app.zip`.
- yt-dlp's: the `releases/latest` redirect for the tag, then
  `releases/download/<tag>/SHA2-256SUMS` and the archive.

Being off the API is not a licence to be careless, so `packages/ghrelease`
still behaves as a good client, and both updaters go through it rather than
around it:

- **Answers are cached on disk** (`github.json` in Application Support). A
  check younger than `updater.CheckMaxAge` (six hours) is answered without
  asking, including after a relaunch — a fresh process used to mean a fresh
  request, which is how relaunching during development ran the quota down.
- **Expired copies revalidate** with `If-None-Match`, so an unchanged file
  comes back as a 304 with no body.
- **A refusal is honoured until it expires, across relaunches too.** A 429, or
  any answer with `Retry-After`, holds every later request — checks, forced
  checks and downloads alike — for as long as GitHub said, and at least a
  minute when it said nothing. The explanation is repeated from memory rather
  than asked for again. A rule the interface can opt out of is not a rule.
- **Requests are serial**, and each identifies itself with a User-Agent naming
  Lasso and where it comes from.
- **Ten seconds minimum between requests for the same file**, which serves the
  last answer rather than erroring: the answer cannot change in ten seconds.
- **Failures back off** from 30 s to 30 min, so a dead network is not retried
  every time the settings screen opens.

**Keep one `ghrelease.Client` for the life of the process** and share it:
`App.releases` is created in `startup` and handed to both updaters. Two
clients would each miss the other's refusal, and a client rebuilt per check
remembers nothing — which was the original bug.

Nothing checks on a timer. Every request follows something the person did:
opening Settings, or pressing a button.

`ditto`, not `archive/zip`: it carries the extended attributes and symlinks a
signed bundle depends on. Every external command goes through the injected
`Runner`, which is what lets the unit tests cover the failure paths without a
toolchain — and `LASSO_RELEASES_URL` points the whole thing at a local server so
the integration test exercises a real update on a real bundle.

## Releasing

The version lives in `apps/desktop/wails.json` (`info.productVersion`) and
reaches Info.plist, where the updater and the release manifest both read it.

1. Bump the version, commit `chore: <version>`, push.
2. `make release-assets NOTES=notes.md` — build, reseal, DMG, update zip,
   `latest.json`, `SHA256SUMS`. The notes are shown in the app before
   installing, so write them for someone deciding whether to update.
3. Verify before tagging, because a pushed tag cannot be moved: version,
   `codesign --verify --deep --strict`, `shasum -c SHA256SUMS`, `latest.json`
   sizes and digests against the files, and
   `LASSO_INTEGRATION=1 go test ./packages/updater -run RealUpdate`.
4. Tag and push the tag.
5. **Publish in stages, never with one `gh release create` carrying the
   assets.** When an upload fails, `gh` deletes the draft it created — taking
   the files already uploaded with it — and can still exit 0; that is how
   0.1.3 existed as a tag with no release. So: `gh release create --draft`
   with notes only; `gh release upload` the small files; upload the DMG on its
   own; then `gh release edit --draft=false --latest`. A draft is invisible to
   both discovery paths, so nobody sees a half-uploaded release.
6. Check both ways clients find it: `releases/latest` through the API (0.1.2 to
   0.1.5) and `releases/latest/download/latest.json` (0.1.6 on), downloading the
   zip and checking it against both.

## Errors and the doctor

`packages/core` classifies a yt-dlp failure into an `ErrorKind` and a
plain-language sentence, keeping the raw output for a details toggle. Add a new
signature to the classifier with a test that uses the **verbatim** output — the
wordings that matter are the ones real runs produce, and several have been
missed by paraphrasing them.

`packages/doctor` answers the other half: whether the problem is Lasso's own
setup. It checks the helper programs, the download folder, free space and
whether the configured browser's cookies can actually be read, and repairs what
it can. Every check says what was found, what it means and what to do, and the
ones Lasso can fix expose a button rather than a paragraph.

`doctor.SuggestsDoctor` decides whether a failure is worth offering diagnostics
for, and is deliberately narrow: a private video or a geo-block is the site's
answer, not a local problem, and offering diagnostics for those would teach the
user that the offer means nothing.

## Commands

```
make setup            toolchain check (incl. wails doctor), pnpm install, fetch binaries
make dev              wails dev
make build            build Lasso.app
make test             Go tests across all modules + JS tests
make lint             go vet + gofmt + JS lint
make fetch-binaries   re-download and verify sidecars
make release-assets NOTES=notes.md   DMG, update zip, latest.json and SHA256SUMS
```
