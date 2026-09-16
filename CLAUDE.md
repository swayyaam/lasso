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
