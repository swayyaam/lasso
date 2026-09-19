# Lasso

A small macOS app for saving video and audio from the web. Paste a link, pick a
quality, download.

It is a front end for [yt-dlp](https://github.com/yt-dlp/yt-dlp), and it brings
its own copy — along with ffmpeg, ffprobe and deno. Nothing to install, nothing
to keep up to date, no terminal.

> **Apple Silicon Macs.** The app is not code-signed yet, so the first launch
> needs one extra click. See [Installing](#installing).

## Install

1. Download `Lasso.dmg` from the [latest release](https://github.com/swayyaam/lasso/releases/latest).
2. Open it and drag **Lasso** into **Applications**.
3. Open Lasso. macOS will refuse, saying it cannot verify the developer.
4. Go to **System Settings → Privacy & Security**, scroll to Security, and click
   **Open Anyway** next to the message about Lasso.
5. Open it again and confirm.

macOS remembers, so that is a one-time thing. It is what happens to any app
that has not been through Apple's paid signing process.

If macOS instead says Lasso **"is damaged and can't be opened"**, you have a
build from before v0.1.1, whose signature was broken by the build itself.
Download again from the [latest release](https://github.com/swayyaam/lasso/releases/latest).

The first launch takes a few seconds: Lasso copies its helper programs into
`~/Library/Application Support/Lasso` and lets macOS scan them. You will see a
setup screen while that happens.

## Using it

Paste a link and press **Fetch**. Lasso looks the video up and shows you what it
actually has — then pick a quality and press **Download**.

You can paste with ⌘V anywhere in the window; you do not have to click the box
first. Enter fetches an unresolved link and downloads a resolved one, so
paste-and-hold-Enter is the whole flow.

### Picking a quality

The quality row only offers what the video really has. If there is no 4K
encode, there is no 4K button — so you can never pick something the download
would quietly fail to deliver.

Each option shows roughly how big the file will be, and a tag for anything
notable:

| Tag | Means |
|---|---|
| **HDR10** / **HLG** / **DV** | A high-dynamic-range encode is available |
| **60** | 60fps or better |
| **8K** | The top of the ladder |

If a site only offers you its lowest quality, Lasso says so rather than letting
it look like that is all the video has. Usually that means the site is holding
the good formats back until you are signed in — see [Signed-in
downloads](#signed-in-downloads).

### Audio and music

**Audio only** saves just the sound. **Original** is the one to reach for: it
takes the site's own audio stream and only changes the container around it.
Every other option re-encodes, and re-encoding audio that is already compressed
loses a little more.

That is worth knowing before you pick **FLAC**. FLAC is a lossless *format*, but
it cannot recover what a site already threw away — asking for it from YouTube
gives you a much larger file of exactly the same sound. Lasso tells you when
that is the case. On a source that genuinely serves lossless audio, FLAC does
what you would expect.

Two music options live under **Advanced → Music**:

- **Tag as music** fills in artist, title and year. A video called
  "Artist — Song" becomes those two fields; a site that already states a real
  artist is left alone.
- **Split chapters into tracks** turns one long "full album" upload into a file
  per chapter, each numbered and titled properly so a music library reads them
  as separate songs. The full recording is kept alongside them.

The **Music** and **Album** presets set these up for you.

### Presets

Five come built in — Best quality, Archive (MKV), Podcast audio, Music and
Album. Set up anything you like and press **Save current** to make your own.

### The queue

Downloads appear on the right with progress, speed and time remaining. You can
**pause** one and pick it up later — it resumes from where it stopped rather
than starting over — or cancel, retry and clear it away.

**History** remembers what you have downloaded after you quit. From there you
can find a file again, or run the same download a second time with exactly the
options it used before.

When something finishes, Lasso tells you.

### Signed-in downloads

Some videos need you to be signed in: private and members-only ones, anything
age-restricted, and increasingly YouTube's higher qualities. Lasso can borrow
the cookies from a browser you are already signed in to — **Settings → Cookies
from browser**.

Chrome, Firefox, Brave and Arc work straight away. **Safari needs Full Disk
Access**, because macOS keeps Safari's cookies somewhere apps cannot read
without it; Lasso will tell you and offer to open the right settings pane.

### Advanced

Everything yt-dlp can do is still there, behind **Advanced**: container and
codec preferences, HDR, subtitles (including auto-generated), SponsorBlock,
embedded chapters and artwork, playlist ranges, a speed limit, and the filename
template. **Show command** prints the exact yt-dlp command your choices produce,
so you can paste it into a terminal and get the same file.

## When something goes wrong

Most failed downloads are not about the link. **Settings → Run the doctor**
checks the things that actually break: whether the helper programs run, whether
your download folder exists and can be written to, whether there is space, and
whether your browser's cookies can really be read. It explains what it finds and
repairs what it can.

If a download fails in a way that looks like a setup problem rather than a bad
link, the failed row offers the doctor directly.

Sites change often, and yt-dlp changes with them. **Settings → Update yt-dlp**
fetches the newest version, checks it against the official checksums, proves it
runs, and only then swaps it in. A failed update leaves your working copy alone.

## Privacy

Lasso makes three kinds of network request and no others: yt-dlp's own traffic,
the yt-dlp updater, and fetching thumbnails to show you. There is no telemetry
and no analytics, and nothing is sent anywhere about what you download.

---

## Building from source

You need macOS, [Go](https://go.dev/dl/) 1.24+, [Node](https://nodejs.org/) 20+,
[pnpm](https://pnpm.io/) 10+ and [Wails](https://wails.io/) v2
(`go install github.com/wailsapp/wails/v2/cmd/wails@latest`, with
`$(go env GOPATH)/bin` on your `PATH`).

```bash
git clone https://github.com/swayyaam/lasso.git
cd lasso
make setup     # toolchain check, JS deps, fetch the helper binaries
make dev       # run it
```

`make build` produces `apps/desktop/build/bin/Lasso.app` and `make dmg`
packages it. `make help` lists the rest.

### How it is put together

```
apps/desktop/      Wails app (Go backend + React frontend)
packages/core/     yt-dlp arg builder, metadata, progress parsing, queue, tagging
packages/binaries/ helper-program install, verification, updates
packages/presets/  built-in and user presets
packages/history/  the record of finished downloads
packages/doctor/   diagnoses why a download failed, and repairs what it can
packages/ui/       shared React components + design tokens
```

`packages/core` has no Wails dependency, so it can be tested on its own.
`DESIGN-webflow.md` is the visual language.

### Helper binaries

Versions are pinned with SHA256 checksums in
[`binaries.lock.json`](binaries.lock.json), fetched and verified by
`scripts/fetch-binaries.sh`. They are never committed.

| Binary | Version | Source |
|---|---|---|
| yt-dlp | 2026.08.19 | GitHub release, `yt-dlp_macos.zip` (universal2, onedir) |
| ffmpeg | 9.0.1 | [ffmpeg.martin-riedl.de](https://ffmpeg.martin-riedl.de/) |
| ffprobe | 9.0.1 | [ffmpeg.martin-riedl.de](https://ffmpeg.martin-riedl.de/) |
| deno | 2.9.6 | GitHub release |

Lasso ships the **onedir** yt-dlp build rather than the single file. The
single-file one re-extracts a 37 MB archive on every run — about 5.8 seconds
each time — against roughly 0.16 seconds for onedir once macOS has scanned it.
That scan is warmed during install, so you never wait for it.

## Licence

[GPL-3.0](LICENSE).

Lasso ships an ffmpeg built with `--enable-gpl --enable-version3`, so the
distributed app is a GPLv3 work and is licensed to match. You are free to use,
study, change and share it, as long as anything you pass on carries the same
freedoms.

The programs it bundles keep their own licences: yt-dlp is
[Unlicense](https://github.com/yt-dlp/yt-dlp/blob/master/LICENSE), ffmpeg and
ffprobe are GPLv3 for this build, and deno is
[MIT](https://github.com/denoland/deno/blob/main/LICENSE.md).
