import type { ButtonHTMLAttributes, ReactNode } from "react";
import { cx } from "../cx";

/**
 * Chip is the `pricing-tab` toggle from design.md, used here for the quick
 * quality picks.
 *
 * Selection is shown by lifting to surface-2, not by filling with lavender:
 * the document reserves the accent for the brand mark, the primary CTA, focus
 * and link emphasis.
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
        "no-drag h-7 rounded-pill px-sm text-button font-medium transition-colors duration-150",
        "disabled:pointer-events-none disabled:text-ink-tertiary",
        selected
          ? "bg-surface-2 text-ink ring-1 ring-hairline-strong"
          : "bg-transparent text-ink-subtle hover:bg-surface-1 hover:text-ink-muted",
        className,
      )}
      {...rest}
    >
      {children}
    </button>
  );
}
