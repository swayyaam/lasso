import { useEffect, useState } from "react";
import { api, core } from "../bindings";
import { cx } from "@lasso/ui";

type Status = "idle" | "loading" | "ready" | "failed";

/**
 * Thumbnail renders a preview at the size it is actually displayed.
 *
 * The backend picks the closest source, decodes it once at the requested
 * width, re-encodes it as JPEG and caches it, then serves it from the app's
 * own asset middleware — so the webview never talks to a CDN, and a re-render
 * costs nothing.
 *
 * The <img> is only mounted once the backend has confirmed the file exists,
 * and an onError still falls back to the placeholder: a broken-image icon is
 * never an acceptable thing to show someone.
 */
export function Thumbnail({
  thumbnails,
  width,
  className,
}: {
  thumbnails?: core.Thumbnail[];
  width: number;
  className?: string;
}) {
  const [src, setSrc] = useState("");
  const [status, setStatus] = useState<Status>("idle");

  // Resolve the source outside the effect so an equivalent array does not
  // retrigger the fetch on every render.
  const source = bestSource(thumbnails, width);

  useEffect(() => {
    if (!source) {
      setStatus("idle");
      return;
    }
    let cancelled = false;
    setStatus("loading");

    api
      .Thumbnail(source, width)
      .then((url) => {
        if (cancelled) return;
        setSrc(url);
        setStatus("ready");
      })
      .catch(() => {
        if (!cancelled) setStatus("failed");
      });

    return () => {
      cancelled = true;
    };
  }, [source, width]);

  const frame = cx("shrink-0 overflow-hidden rounded-lg bg-surface-2", className);
  const box = { width, aspectRatio: "16 / 9" };

  if (status === "loading") {
    return <div className={cx(frame, "animate-pulse")} style={box} aria-hidden />;
  }
  if (status !== "ready" || !src) {
    return <div className={frame} style={box} aria-hidden />;
  }

  return (
    <img
      src={src}
      alt=""
      loading="lazy"
      decoding="async"
      onError={() => setStatus("failed")}
      className={cx(frame, "block object-cover")}
      style={box}
    />
  );
}

/**
 * bestSource mirrors core.BestThumbnail: prefer the narrowest image at least
 * as wide as needed, fall back to the widest, and when nothing carries
 * dimensions trust yt-dlp's ordering, whose last entry is its best.
 */
function bestSource(thumbnails: core.Thumbnail[] | undefined, width: number): string {
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
