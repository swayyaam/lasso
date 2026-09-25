# Privacy policy

*Effective 26 September 2026. Applies to Lasso 0.2 and later.*

Lasso is an app that runs on your Mac. It has no accounts, no servers of its
own, no analytics, no crash reporting and no advertising. The developer never
receives anything about you or what you download: there is nowhere for it to
be sent.

This page says exactly what Lasso keeps on your Mac and every connection it
makes, so you can check that for yourself. The source is public at
<https://github.com/swayyaam/lasso>.

## What Lasso keeps on your Mac

Everything is in `~/Library/Application Support/Lasso`, and every file there is
readable only by your macOS user account.

| File | What it holds |
|---|---|
| `settings.json` | Your download folder, how many downloads run at once, which browser (if any) to take cookies from, and whether Lasso is light, dark or follows your Mac. The browser's *name*, never its cookies. |
| `history.json` | Each finished download: its title, link, channel, length, the file it produced and where, its size, the choices used, and when it finished. |
| `queue.json` | Downloads not yet finished, so they carry on after Lasso quits. |
| `presets.json` | Choices you have saved under a name. |
| `thumbnails/` | Small copies of video pictures, so they are not fetched twice. |
| `github.json` | The last answers from GitHub's release pages, so update checks are not repeated. |
| `lasso.lock` | Stops two copies of Lasso running at once. |
| `bin/` | Lasso's helper programs: yt-dlp, ffmpeg, ffprobe and deno. |

The files you download go to the folder you chose, and are yours.

**Removing it.** History → Clear history forgets the list; Settings changes
the rest. To remove everything, quit Lasso and delete the folder above.
Deleting the app does not delete your downloads.

## Every connection Lasso makes

Lasso makes these five kinds of network request and no others. Each one follows
something you did, not a timer.

| When | To | What is sent |
|---|---|---|
| You paste a link, or a download runs | The site the link is on (YouTube, SoundCloud and so on), through yt-dlp | The requests a web browser would make to watch or listen. If you turned on cookies, the site sees them, so it knows the request is from your account. |
| A link, download or history row is shown | The site's own image servers | A request for the picture. Only over https, and never to your own network. |
| You open Settings › About & diagnostics, or press Check again | GitHub (`github.com/swayyaam/lasso/releases`) | A request for the newest version. At most every six hours unless you ask. The request names Lasso and its version. |
| You press Update yt-dlp: in Settings, in the doctor, or on a download that failed because a site changed | GitHub (`github.com/yt-dlp/yt-dlp/releases`) | Requests for yt-dlp's newest version, its checksums and signature, and the release itself. |
| You run the doctor with cookies turned on | YouTube | One request for a public video using your browser's cookies, to check they can be read. |

Every one of these, like any connection on the internet, shows the other end
your IP address. What GitHub does with its requests is covered by
[GitHub's privacy statement](https://docs.github.com/site-policy/privacy-policies/github-general-privacy-statement);
what a site does with yours is covered by that site's own policy. GitHub shows
the developer a total download count for each release file, which identifies
no one.

## Browser cookies

Cookies are off unless you choose a browser in Settings. When you do, yt-dlp
reads that browser's cookies on your Mac at the moment it needs them and sends
each one only to the site it belongs to, as the browser would. Lasso does not
copy them, store them or send them anywhere else.

macOS guards these on purpose. Chrome, Brave and Arc keep a key in your
Keychain, and macOS asks before Lasso may use it. Safari's cookies need Full
Disk Access, which you grant in System Settings and can take back there.

Using cookies means the site treats the download as you, signed in. That can
matter under the site's own terms; see the [Terms](TERMS.md).

## Notifications

"Download finished" notifications are shown by macOS on your Mac. macOS asks
for permission first, and you can turn them off in System Settings.

## Children

Lasso is not directed at children, and it collects nothing from anyone.

## Your rights

Because Lasso sends the developer nothing, the developer holds no data about
you to show, correct or delete. Everything Lasso keeps is in the folder above,
under your control.

## Changes

A change to this policy is published here, with the date at the top, and its
history is kept in the repository. If Lasso ever starts sending anything
anywhere new, this page will say so before a release does it.

## Questions

Open an issue at <https://github.com/swayyaam/lasso/issues>.
