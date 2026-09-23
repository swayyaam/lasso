import { useCallback, useRef, useState } from "react";
import { api, core } from "../bindings";

/**
 * Resolved links are kept for a few minutes, so the second press on a link —
 * downloading it again as audio, or at another size — does not ask the site a
 * second time. Short, because a link's formats can change (a premiere going
 * live, a site withdrawing an encode), and small, because each is a full
 * format list.
 */
const CACHE_FOR_MS = 10 * 60 * 1000;
const CACHE_SIZE = 8;
const cache = new Map<string, { metadata: core.Metadata; at: number }>();

function remembered(url: string): core.Metadata | null {
  const hit = cache.get(url);
  if (!hit) return null;
  if (Date.now() - hit.at > CACHE_FOR_MS) {
    cache.delete(url);
    return null;
  }
  return hit.metadata;
}

function remember(url: string, metadata: core.Metadata) {
  cache.delete(url);
  cache.set(url, { metadata, at: Date.now() });
  while (cache.size > CACHE_SIZE) {
    const oldest = cache.keys().next().value;
    if (oldest === undefined) break;
    cache.delete(oldest);
  }
}

/**
 * forgetResolved empties the cache. Settings call it on save: turning on
 * cookies is the usual fix for a link that came back with only a low quality,
 * and a cached answer would go on showing the low quality anyway.
 */
export function forgetResolved() {
  cache.clear();
}

export type Link = ReturnType<typeof useLink>;

/**
 * useLink is the link being looked at: what was pasted, what it resolved to,
 * and whether that is still in flight.
 *
 * It lives above the screens because two of them share it — the paste field
 * on Ready and Downloading starts a resolve, and Choose shows the result.
 * "Choosing" is simply having a link that is resolving, resolved or refused.
 */
export function useLink() {
  const [url, setUrl] = useState("");
  const [metadata, setMetadata] = useState<core.Metadata | null>(null);
  const [resolving, setResolving] = useState(false);
  const [error, setError] = useState("");
  // A resolve in flight must not overwrite a newer one that already finished,
  // nor reopen a link the user has since cleared.
  const token = useRef(0);

  const resolve = useCallback(async (target: string) => {
    const trimmed = target.trim();
    if (!trimmed) return;

    const mine = ++token.current;
    setUrl(trimmed);
    setError("");

    const known = remembered(trimmed);
    if (known) {
      setMetadata(known);
      setResolving(false);
      return;
    }

    setMetadata(null);
    setResolving(true);
    try {
      const result = await api.FetchMetadata(trimmed);
      if (mine !== token.current) return;
      remember(trimmed, result);
      setMetadata(result);
    } catch (e) {
      if (mine === token.current) setError(cleanError(e));
    } finally {
      if (mine === token.current) setResolving(false);
    }
  }, []);

  const clear = useCallback(() => {
    token.current++;
    setUrl("");
    setMetadata(null);
    setError("");
    setResolving(false);
  }, []);

  return {
    url,
    metadata,
    resolving,
    error,
    /** Whether a link is on screen: resolving, resolved or refused. */
    open: resolving || metadata !== null || error !== "",
    resolve,
    clear,
  };
}

/** cleanError strips Go's wrapping so the user sees the sentence, not a trace. */
export function cleanError(e: unknown): string {
  const text = String(e).replace(/^Error:\s*/, "").trim();
  return text || "Something went wrong.";
}

/**
 * explain splits a backend failure into the sentence to lead with and the raw
 * output to keep behind a toggle.
 *
 * A classified failure arrives as "plain message: yt-dlp's own output", and
 * everything yt-dlp writes to stderr starts with ERROR or WARNING. Anything
 * that does not match is shown whole rather than guessed at.
 */
export function explain(error: string): { message: string; detail: string } {
  const at = error.search(/:\s(?=ERROR[:\s]|WARNING[:\s])/);
  if (at === -1) return { message: error, detail: "" };
  // slice(0, at) drops the joining colon: the message already ends in a stop.
  return { message: error.slice(0, at).trim(), detail: error.slice(at + 2).trim() };
}

/**
 * needsFullDiskAccess spots the one failure with a remedy Lasso can open
 * directly. It keys on the phrase core/errors.go puts in that message, which a
 * Go test pins.
 */
export function needsFullDiskAccess(error: string): boolean {
  return error.includes("Full Disk Access");
}
