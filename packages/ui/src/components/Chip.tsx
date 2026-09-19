import type { ButtonHTMLAttributes, ReactNode } from "react";
import { cx } from "../cx";

/**
 * Chip is the quality / preset toggle.
 *
 * Selection is carried by the brand primary, which is what DESIGN-webflow.md
 * specifies for an active state: "Active state uses brand primary as the
 * indicator". A near-black fill against white is unmistakable across a row of
 * eight rungs, where the previous treatment — a slightly lifted surface —
 * left the current choice needing to be hunted for.
 *
 * 4px corners, not pill: the document reserves full-round for circular icon
 * containers.
 */
export function Chip({
  selected = false,
  className,
  children,
  ...rest
}: {
  selected?: boolean;
  children: ReactNode;
} & ButtonHTMLAttributes<HTMLButtonElement>) {
  return (
    <button
      type="button"
      aria-pressed={selected}
      className={cx(
        "no-drag inline-flex h-7 items-center gap-xxs rounded-sm px-sm text-button",
        "border transition-[background-color,border-color,color] duration-150 ease-standard",
        "disabled:pointer-events-none disabled:border-transparent disabled:bg-surface-2 disabled:text-ink-faint",
        selected
          ? "border-primary bg-primary text-on-primary hover:bg-primary-hover"
          : "border-hairline bg-canvas text-ink-subtle hover:border-hairline-strong hover:bg-surface-2 hover:text-ink",
        className,
      )}
      {...rest}
    >
      {children}
    </button>
  );
}
