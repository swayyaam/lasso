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
 * formatEta renders a countdown. Anything beyond an hour is rounded to whole
 * hours, since the exact figure is guesswork at that range anyway.
 */
export function formatEta(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds <= 0) return "";
  if (seconds < 60) return `${Math.round(seconds)}s left`;
  if (seconds < 3600) return `${Math.round(seconds / 60)}m left`;
  return `${Math.round(seconds / 3600)}h left`;
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
