import type { ButtonHTMLAttributes, ReactNode } from "react";
import { cx } from "../cx";

type Variant = "primary" | "secondary" | "tertiary" | "danger";
type Size = "sm" | "md" | "icon";

/**
 * Button implements the button styles in DESIGN-webflow.md.
 *
 * The system's conversion hierarchy is deliberately two-colour: near-black
 * fill for the one action that matters on a surface, white-on-hairline for
 * everything else. The five chromatic accents are surface fills and never
 * appear here — "Don't use chromatic accents as button backgrounds."
 *
 * Corners are `sm` (4px) throughout. The document is explicit that the brand
 * never renders a CTA as a pill and that 4px is the canonical button radius.
 *
 * Every variant defines hover *and* active. A button that only changes on
 * hover feels unresponsive at the moment of clicking, which is the moment the
 * feedback is actually wanted.
 */
export function Button({
  variant = "secondary",
  size = "md",
  busy = false,
  icon,
  trailingIcon,
  className,
  children,
  disabled,
  ...rest
}: {
  variant?: Variant;
  size?: Size;
  /** Shows a spinner and blocks input while an action is in flight. */
  busy?: boolean;
  /** Leading icon. Keep it to a 14px lucide glyph so the label stays the subject. */
  icon?: ReactNode;
  /** Trailing icon, for actions that lead somewhere — the document's text-arrow button. */
  trailingIcon?: ReactNode;
} & ButtonHTMLAttributes<HTMLButtonElement>) {
  const variants: Record<Variant, string> = {
    // The canonical near-black CTA. Hover and press walk up the ink ramp
    // rather than fading, so the control never looks disabled mid-press.
    primary: "bg-primary text-on-primary hover:bg-primary-hover active:bg-primary-focus shadow-sm",
    // The white outline CTA: canvas fill, hairline border. Both the fill and
    // the border move, which is what makes it read as raised rather than as
    // text that happens to be boxed.
    secondary:
      "bg-canvas text-ink border border-hairline hover:bg-surface-2 hover:border-hairline-strong active:bg-surface-3",
    // Ghost. Nothing until pointed at, then a fill — the quiet actions in a
    // row of controls, where a border on each would fence the row into stripes.
    tertiary: "bg-transparent text-ink-subtle hover:bg-surface-2 hover:text-ink active:bg-surface-3",
    danger:
      "bg-transparent text-danger-strong border border-danger/30 hover:bg-danger-surface hover:border-danger/60 active:bg-danger-surface",
  };

  const sizes: Record<Size, string> = {
    sm: "h-7 px-sm gap-xxs",
    md: "h-9 px-md gap-xs",
    // Square, for a glyph with no label. The accessible name has to come from
    // aria-label or a tooltip.
    icon: "size-7 gap-0",
  };

  return (
    <button
      type="button"
      disabled={disabled || busy}
      className={cx(
        "no-drag inline-flex shrink-0 items-center justify-center rounded-sm",
        "text-button whitespace-nowrap",
        "transition-[background-color,border-color,color,box-shadow] duration-150 ease-standard",
        // Disabled resolves to documented colours rather than an opacity fade,
        // so a disabled primary does not read as a dimmer primary.
        "disabled:pointer-events-none disabled:border-transparent disabled:bg-surface-2 disabled:text-ink-faint disabled:shadow-none",
        sizes[size],
        variants[variant],
        className,
      )}
      {...rest}
    >
      {busy ? <Spinner /> : icon}
      {children}
      {trailingIcon}
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
