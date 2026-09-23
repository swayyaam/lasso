import { useEffect, useState } from "react";
import { Icon, cx } from "@lasso/ui";
import { api, core } from "../bindings";
import { formatDuration } from "../format";
import type { Kind } from "../format";

/**
 * useThumbnail turns a remote image URL into one the webview may load.
 *
 * The backend picks up the image, decodes it once at the width asked for,
 * re-encodes it as JPEG and caches it, then serves it from the app's own asset
 * handler — so the webview never talks to a CDN, and a re-render costs
 * nothing. Ask for the width actually rendered, doubled for the Retina screen
 * every supported Mac has.
 */
export function useThumbnail(source: string, width: number): string {
  const [served, setServed] = useState("");

  useEffect(() => {
    setServed("");
    if (!source) return;
    let cancelled = false;
    api
      .Thumbnail(source, width)
      .then((url) => {
        if (!cancelled) setServed(url);
      })
      .catch(() => {
        // A missing picture falls back to the placeholder; it is never worth
        // an error message.
      });
    return () => {
      cancelled = true;
    };
  }, [source, width]);

  return served;
}

/**
 * bestSource mirrors core.BestThumbnail: prefer the narrowest image at least
 * as wide as needed, fall back to the widest, and when nothing carries
 * dimensions trust yt-dlp's ordering, whose last entry is its best.
 */
export function bestSource(thumbnails: core.Thumbnail[] | undefined, width: number): string {
  if (!thumbnails?.length) return "";

  let best: core.Thumbnail | undefined;
  let largest: core.Thumbnail | undefined;
  for (const t of thumbnails) {
    if (!t.url || !t.width || t.width <= 0) continue;
    if (!largest || t.width > largest.width) largest = t;
    if (t.width >= width && (!best || t.width < best.width)) best = t;
  }
  if (best) return best.url;
  if (largest) return largest.url;

  for (let i = thumbnails.length - 1; i >= 0; i--) {
    if (thumbnails[i].url) return thumbnails[i].url;
  }
  return "";
}

const SIZES = {
  // The Recent and History rows.
  row: { width: 96, badge: "size-5", glyph: "size-3" },
  // A playlist's videos inside a group card.
  small: { width: 56, badge: "", glyph: "" },
  // The downloading card and a resolved link's header.
  large: { width: 160, badge: "size-6", glyph: "size-3.5" },
} as const;

/**
 * MediaThumb is a download's picture: the thumbnail at 16:9, the kind as a
 * coloured badge, and the running time.
 *
 * Until the picture arrives — or when a site gave none — it is a dark frame
 * with the kind's glyph, not a grey box: an empty light rectangle reads as
 * something that failed to load, a dark one as a video that has not started.
 */
export function MediaThumb({
  source,
  kind,
  duration = 0,
  size = "row",
  className,
}: {
  source: string;
  kind?: Kind;
  duration?: number;
  size?: keyof typeof SIZES;
  className?: string;
}) {
  const spec = SIZES[size];
  const served = useThumbnail(source, spec.width * 2);
  const [broken, setBroken] = useState(false);
  useEffect(() => setBroken(false), [served]);

  const Glyph = kind === "audio" ? Icon.Audio : Icon.Video;
  const length = size === "small" ? "" : formatDuration(duration);

  return (
    <div
      className={cx("relative shrink-0 overflow-hidden rounded-sm bg-inverse-surface-1", className)}
      style={{ width: spec.width, aspectRatio: "16 / 9" }}
      aria-hidden
    >
      {served && !broken ? (
        <img
          src={served}
          alt=""
          decoding="async"
          onError={() => setBroken(true)}
          className="absolute inset-0 size-full object-cover"
        />
      ) : (
        <span className="absolute inset-0 flex items-center justify-center text-inverse-ink/50">
          <Glyph className={size === "large" ? "size-6" : "size-4"} strokeWidth={1.75} />
        </span>
      )}

      {kind && spec.badge && (
        // The category colour, on a circle: the one shape the system keeps
        // full-round for. Green carries ink rather than white, as the document
        // pairs it.
        <span
          className={cx(
            "absolute bottom-1.5 left-1.5 flex items-center justify-center rounded-pill",
            spec.badge,
            kind === "audio" ? "bg-accent-green text-primary" : "bg-accent-purple text-on-primary",
          )}
        >
          <Glyph className={spec.glyph} strokeWidth={2} />
        </span>
      )}

      {length && (
        <span className="absolute right-1 bottom-1 rounded-xs bg-overlay/80 px-1 py-0.5 text-caption leading-none text-inverse-ink tabular-nums">
          {length}
        </span>
      )}
    </div>
  );
}
