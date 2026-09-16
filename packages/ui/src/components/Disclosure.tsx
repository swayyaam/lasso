import type { ReactNode } from "react";
import { cx } from "../cx";
import { Eyebrow } from "./Card";

/**
 * Disclosure is one group inside the advanced drawer.
 *
 * The drawer exists so the default experience stays "paste a link, pick a
 * quality, download" — everything yt-dlp can do is reachable, just not in the
 * way until it is asked for.
 */
export function Disclosure({
  title,
  summary,
  open,
  onToggle,
  children,
}: {
  title: string;
  /** A short description of the current settings, shown while collapsed. */
  summary?: string;
  open: boolean;
  onToggle: () => void;
  children: ReactNode;
}) {
  return (
    <div className="border-b border-hairline last:border-b-0">
      <button
        type="button"
        onClick={onToggle}
        aria-expanded={open}
        className={cx(
          "no-drag flex w-full items-center justify-between gap-sm py-sm text-left",
          "transition-colors duration-150 hover:bg-surface-2/40",
        )}
      >
        <span className="flex min-w-0 flex-col gap-0.5">
          <Eyebrow>{title}</Eyebrow>
          {summary && !open && <span className="truncate text-caption text-ink-tertiary">{summary}</span>}
        </span>
        <span
          className={cx(
            "shrink-0 text-ink-subtle transition-transform duration-150",
            open && "rotate-90",
          )}
          aria-hidden
        >
          ›
        </span>
      </button>
      {open && <div className="flex flex-col gap-sm pb-md">{children}</div>}
    </div>
  );
}
