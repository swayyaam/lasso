import type { ReactNode } from "react";
import { cx } from "../cx";

/**
 * The five chromatic stops, mapped to capabilities.
 *
 * DESIGN-webflow.md ties each accent to one of the platform's product
 * categories and uses them as full-saturation surface fills — never as button
 * backgrounds. Lasso has no product categories, but it does have capabilities
 * that a download either has or does not, and they are exactly the thing worth
 * spotting across a list of eight quality rungs.
 *
 * One capability, one colour, and no sixth:
 *
 *   purple  dynamic range — HDR10, HLG, Dolby Vision
 *   blue    motion — frame rates above 50fps
 *   pink    the top of the ladder — 8K
 *   orange  lossy audio
 *   green   lossless audio
 *
 * Text colours are the document's own for these fills: white on every stop
 * except green, whose card pattern specifies near-black.
 */
export type TagTone = "purple" | "blue" | "pink" | "orange" | "green" | "neutral";

const TONES: Record<TagTone, string> = {
  purple: "bg-accent-purple text-on-primary",
  blue: "bg-accent-blue-deep text-on-primary",
  pink: "bg-accent-pink text-on-primary",
  orange: "bg-accent-orange text-on-primary",
  green: "bg-accent-green text-primary",
  neutral: "bg-surface-3 text-ink-subtle",
};

/**
 * Tag marks a capability on a quality option.
 *
 * It is deliberately tiny and deliberately loud: the whole job is to be found
 * in peripheral vision while reading something else. 4px corners, like every
 * other small element in the system.
 */
export function Tag({
  tone = "neutral",
  title,
  children,
}: {
  tone?: TagTone;
  /** Spells the abbreviation out on hover — "HDR" is not obvious to everyone. */
  title?: string;
  children: ReactNode;
}) {
  return (
    <span
      title={title}
      className={cx(
        "inline-flex shrink-0 items-center rounded-xs px-1 text-[10px] leading-4 font-medium",
        TONES[tone],
      )}
    >
      {children}
    </span>
  );
}

/**
 * Dot is the quietest form of the same idea: a category colour with no label,
 * for a control whose text already says what it is.
 *
 * The audio format chips use it. "FLAC" already reads as FLAC; what the chip
 * cannot say in four letters is that it is the lossless one of the four, and a
 * green dot says that at a glance without a second word.
 */
export function Dot({ tone, title }: { tone: TagTone; title?: string }) {
  const fills: Record<TagTone, string> = {
    purple: "bg-accent-purple",
    blue: "bg-accent-blue-deep",
    pink: "bg-accent-pink",
    orange: "bg-accent-orange",
    green: "bg-accent-green",
    neutral: "bg-hairline-tertiary",
  };

  return (
    <span
      title={title}
      aria-hidden
      className={cx("size-1.5 shrink-0 rounded-pill", fills[tone])}
    />
  );
}
