/** Formatting helpers shared across the interface. */

/** formatDuration renders seconds as 1:02:03 or 4:05. */
export function formatDuration(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds <= 0) return "";

  const total = Math.round(seconds);
  const h = Math.floor(total / 3600);
  const m = Math.floor((total % 3600) / 60);
  const s = total % 60;

  const pad = (n: number) => String(n).padStart(2, "0");
  return h > 0 ? `${h}:${pad(m)}:${pad(s)}` : `${m}:${pad(s)}`;
}

/** formatBytes renders a size in the largest unit that keeps it readable. */
export function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) return "";

  const units = ["B", "KB", "MB", "GB", "TB"];
  let value = bytes;
  let unit = 0;
  while (value >= 1000 && unit < units.length - 1) {
    value /= 1000;
    unit++;
  }
  return `${value < 10 && unit > 0 ? value.toFixed(1) : Math.round(value)} ${units[unit]}`;
}

/** formatSpeed renders bytes per second. */
export function formatSpeed(bytesPerSecond: number): string {
  const size = formatBytes(bytesPerSecond);
  return size ? `${size}/s` : "";
}

/**
 * formatEta renders a countdown in words. Beyond a few minutes it rounds, since
 * yt-dlp's estimate swings with the connection and a ticking "4 min 37 s"
 * claims a precision it does not have.
 */
export function formatEta(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds <= 0) return "";
  if (seconds < 60) return "less than a minute left";
  if (seconds < 600) {
    const m = Math.floor(seconds / 60);
    const s = Math.round((seconds % 60) / 10) * 10;
    return s > 0 && s < 60 ? `about ${m} min ${s} s left` : `about ${m} min left`;
  }
  if (seconds < 3600) return `about ${Math.round(seconds / 60)} min left`;
  const h = Math.floor(seconds / 3600);
  const m = Math.round((seconds % 3600) / 600) * 10;
  return m > 0 && m < 60 ? `about ${h} h ${m} min left` : `about ${h} h left`;
}

/**
 * formatLength renders a running time in words, e.g. "6 h 20 min". Minutes
 * round down, so it never reads longer than the 10:35 on the picture beside it.
 */
export function formatLength(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds <= 0) return "";
  if (seconds < 60) return `${Math.round(seconds)} s`;
  const h = Math.floor(seconds / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  if (h === 0) return `${m} min`;
  return m > 0 ? `${h} h ${m} min` : `${h} h`;
}

/**
 * formatWhen places a finished download in time the way a person would: "Just
 * now", "Today", "Yesterday", then the date. A list of downloads is scanned
 * for "the one from this morning", which a timestamp answers worse.
 */
export function formatWhen(millis: number): string {
  if (!Number.isFinite(millis) || millis <= 0) return "";

  const then = new Date(millis);
  const now = new Date();
  const seconds = (now.getTime() - millis) / 1000;
  if (seconds < 120) return "Just now";

  const day = (d: Date) => new Date(d.getFullYear(), d.getMonth(), d.getDate()).getTime();
  const days = Math.round((day(now) - day(then)) / 86_400_000);
  if (days === 0) return "Today";
  if (days === 1) return "Yesterday";
  return then.toLocaleDateString(undefined, {
    month: "short",
    day: "numeric",
    year: then.getFullYear() === now.getFullYear() ? undefined : "numeric",
  });
}

/** Kind is what a download is, as the interface talks about it. */
export type Kind = "video" | "audio";

/** kindOf reads a download's kind off its pick. */
export function kindOf(pick: string | undefined): Kind {
  return pick?.startsWith("audio-") ? "audio" : "video";
}

/** extensionOf is a file's extension as a person reads it: "MP4", "M4A". */
export function extensionOf(path: string): string {
  const name = basename(path);
  const dot = name.lastIndexOf(".");
  return dot > 0 ? name.slice(dot + 1).toUpperCase() : "";
}

/**
 * describeFile is the line under a finished download's title: what it is, what
 * it came out as, and how big it is — "Video · 1080p MP4 · 1.4 GB".
 *
 * Every part is read from the file rather than the request: "Best" says
 * nothing about what arrived, and a 1080p pick of a 720p video is a 720p file.
 */
export function describeFile(d: {
  options?: { pick?: string; clip?: Clip };
  filePath: string;
  resolution?: string;
  bytes?: number;
}): string {
  const kind = kindOf(d.options?.pick);
  const format = [
    kind === "audio" && d.options?.pick === "audio-original" ? "Original" : "",
    kind === "video" ? d.resolution ?? "" : "",
    extensionOf(d.filePath),
  ]
    .filter(Boolean)
    .join(" ");

  return [kind === "audio" ? "Audio" : "Video", format, formatBytes(d.bytes ?? 0), describeClip(d.options?.clip)]
    .filter(Boolean)
    .join(" · ");
}

/** A clip's times, as core.Clip carries them: seconds, End 0 for "to the end". */
export interface Clip {
  start: number;
  end: number;
}

export function isWhole(clip?: Clip): boolean {
  return !clip || (clip.start <= 0 && clip.end <= 0);
}

/** describeClip is "Clip 1:00–2:30", "Clip from 1:30", or "" for the whole thing. */
export function describeClip(clip?: Clip): string {
  if (!clip || isWhole(clip)) return "";
  const from = formatDuration(clip.start) || "0:00";
  return clip.end > 0 ? `Clip ${from}–${formatDuration(clip.end)}` : `Clip from ${from}`;
}

/**
 * parseTime reads a time the way people type one — 90, 1:30, 1:02:03, 1:30.5 —
 * into seconds, or NaN when it is not one. Only the last part may have a
 * fraction, and a minutes or seconds part after the first stays under 60.
 */
export function parseTime(text: string): number {
  const parts = text.trim().split(":");
  if (parts.length > 3 || parts.some((p) => p === "")) return NaN;
  let total = 0;
  for (let i = 0; i < parts.length; i++) {
    const last = i === parts.length - 1;
    if (!(last ? /^\d+(\.\d+)?$/ : /^\d+$/).test(parts[i])) return NaN;
    const value = Number(parts[i]);
    if (i > 0 && value >= 60) return NaN;
    total = total * 60 + value;
  }
  return total;
}

/** describePick names what a download was asked to be, before it exists. */
export function describePick(pick: string | undefined): string {
  if (!pick || pick === "best") return "Video, best quality";
  if (pick === "audio-original") return "Audio, original quality";
  if (pick.startsWith("audio-")) return `Audio as ${pick.slice(6).toUpperCase()}`;
  return `Video, ${pick}`;
}

/**
 * siteOf names the site a link is on, for the line under a title. Only the
 * well-known ones get a proper name; anything else shows its host, which is
 * still more useful than nothing.
 */
export function siteOf(url: string): string {
  let host = "";
  try {
    host = new URL(url.trim()).hostname.toLowerCase().replace(/^(www|m|music)\./, "");
  } catch {
    return "";
  }
  const names: Record<string, string> = {
    "youtube.com": "YouTube",
    "youtu.be": "YouTube",
    "vimeo.com": "Vimeo",
    "soundcloud.com": "SoundCloud",
    "twitch.tv": "Twitch",
    "x.com": "X",
    "twitter.com": "X",
    "instagram.com": "Instagram",
    "tiktok.com": "TikTok",
    "bandcamp.com": "Bandcamp",
    "archive.org": "Internet Archive",
    "reddit.com": "Reddit",
    "dailymotion.com": "Dailymotion",
  };
  for (const [domain, name] of Object.entries(names)) {
    if (host === domain || host.endsWith("." + domain)) return name;
  }
  return host;
}

/** shortLink is a link without its scheme and www, for showing in a field. */
export function shortLink(url: string): string {
  return url.trim().replace(/^https?:\/\/(www\.)?/i, "");
}

/**
 * looksLikeURL is the test for whether a pasted string should be resolved.
 *
 * Deliberately permissive: the backend validates properly, and a false
 * positive here just produces a clear error instead of silently ignoring
 * something the user meant to paste.
 */
export function looksLikeURL(text: string): boolean {
  const trimmed = text.trim();
  if (!trimmed || /\s/.test(trimmed)) return false;
  return /^https?:\/\/\S+\.\S+/i.test(trimmed);
}

/**
 * basename is the last path segment — the filename a person recognises.
 *
 * Download paths are absolute and long enough to truncate away to nothing in a
 * queue row, so the row shows this and keeps the full path in the tooltip.
 */
export function basename(path: string): string {
  if (!path) return "";
  const parts = path.split("/");
  return parts[parts.length - 1] || path;
}
