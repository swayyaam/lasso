import { useEffect, useState } from "react";
import { api, core } from "./bindings";

/**
 * Thumbnail renders a preview image at the size it is displayed.
 *
 * The backend picks the closest available source, downscales it once and caches
 * it, then serves it from the app's own asset handler — so the webview never
 * talks to a CDN, and a re-render costs nothing. loading="lazy" leaves the
 * browser to skip work for rows that are scrolled out of view.
 */
export function Thumbnail({
  thumbnails,
  width,
  alt,
}: {
  thumbnails?: core.Thumbnail[];
  width: number;
  alt: string;
}) {
  const [src, setSrc] = useState("");
  const [failed, setFailed] = useState(false);

  // Pick the source here so the effect does not re-run on every render of an
  // equivalent array.
  const source = bestSource(thumbnails, width);

  useEffect(() => {
    if (!source) return;
    let cancelled = false;

    api
      .Thumbnail(source, width)
      .then((url) => {
        if (!cancelled) setSrc(url);
      })
      .catch(() => {
        if (!cancelled) setFailed(true);
      });

    return () => {
      cancelled = true;
    };
  }, [source, width]);

  if (!source || failed) {
    return <div style={{ width, aspectRatio: "16 / 9", background: "#141516" }} aria-hidden />;
  }
  if (!src) {
    return <div style={{ width, aspectRatio: "16 / 9", background: "#141516" }} aria-label={`Loading preview for ${alt}`} />;
  }
  return <img src={src} width={width} alt="" loading="lazy" decoding="async" style={{ display: "block", borderRadius: 8 }} />;
}

/**
 * bestSource mirrors core.BestThumbnail: prefer the narrowest image at least as
 * wide as needed, fall back to the widest, and when nothing carries dimensions
 * trust yt-dlp's ordering, whose last entry is its best.
 */
function bestSource(thumbnails: core.Thumbnail[] | undefined, width: number): string {
  if (!thumbnails || thumbnails.length === 0) return "";

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
