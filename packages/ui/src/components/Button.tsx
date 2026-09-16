import type { ButtonHTMLAttributes, ReactNode } from "react";
import { cx } from "../cx";

type Variant = "primary" | "secondary" | "tertiary" | "danger";
type Size = "sm" | "md";

/**
 * Button implements the four button styles in design.md.
 *
 * Corners are always `md` (8px): the document is explicit that CTAs are never
 * pill-rounded. Lavender appears on `primary` only — it is the app's single
 * chromatic accent and stays scarce.
 */
export function Button({
  variant = "secondary",
  size = "md",
  busy = false,
  icon,
  className,
  children,
  disabled,
  ...rest
}: {
  variant?: Variant;
  size?: Size;
  /** Shows a spinner and blocks input while an action is in flight. */
  busy?: boolean;
  icon?: ReactNode;
} & ButtonHTMLAttributes<HTMLButtonElement>) {
  const variants: Record<Variant, string> = {
    primary: "bg-primary text-on-primary hover:bg-primary-hover active:bg-primary-focus",
    secondary: "bg-surface-1 text-ink border border-hairline hover:bg-surface-2 hover:border-hairline-strong",
    tertiary: "bg-transparent text-ink-subtle hover:text-ink hover:bg-surface-1",
    danger: "bg-transparent text-danger border border-danger/40 hover:bg-danger/10",
  };

  return (
    <button
      type="button"
      disabled={disabled || busy}
      className={cx(
        "no-drag inline-flex items-center justify-center gap-xs rounded-md",
        "text-button font-medium whitespace-nowrap",
        "transition-colors duration-150",
        // Disabled reads as ink-tertiary rather than a faded copy of the
        // variant, so every state resolves to a documented colour.
        "disabled:pointer-events-none disabled:bg-surface-1 disabled:text-ink-tertiary disabled:border-hairline",
        size === "sm" ? "h-7 px-sm" : "h-8 px-sm",
        variants[variant],
        className,
      )}
      {...rest}
    >
      {busy ? <Spinner /> : icon}
      {children}
    </button>
  );
}

/** Spinner is the in-button activity indicator. */
export function Spinner({ className }: { className?: string }) {
  return (
    <span
      role="status"
      aria-label="Working"
      className={cx(
        "inline-block size-3.5 shrink-0 animate-spin rounded-pill",
        "border-2 border-current border-t-transparent opacity-70",
        className,
      )}
    />
  );
}
