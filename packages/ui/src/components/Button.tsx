import type { ButtonHTMLAttributes, ReactNode } from "react";
import { cx } from "../cx";

type Variant = "primary" | "secondary" | "tertiary" | "danger";
type Size = "sm" | "md" | "lg" | "icon";

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
 *
 * Depth comes from the document's layered drop shadow — five stops at very low
 * individual opacities, its only atmospheric effect — plus a one-pixel press.
 * A raised control that does not move when pressed reads as a picture of a
 * button, which is what "flat" actually means: not the absence of gradient,
 * but the absence of response.
 *
 * Primary additionally carries a shallow top-down gradient across the ink
 * ramp. That is a derivation: the source specifies a flat #080808 fill. On a
 * white canvas a large flat near-black rectangle reads as a hole rather than
 * as a raised surface, and two stops of the same ink give it a top edge
 * without introducing a colour the system does not have.
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
    // The canonical near-black CTA.
    primary: cx(
      "bg-primary bg-gradient-to-b from-ink-strong to-primary text-on-primary",
      "shadow-layered hover:from-ink-muted hover:to-ink-strong hover:shadow-layered-strong",
      "active:translate-y-px active:shadow-sm active:from-primary active:to-primary",
    ),
    // The white outline CTA: canvas fill, hairline border. Both the fill and
    // the border move, which is what makes it read as raised rather than as
    // text that happens to be boxed.
    secondary: cx(
      "bg-canvas text-ink border border-hairline shadow-sm",
      "hover:bg-surface-1 hover:border-hairline-strong hover:shadow-layered",
      "active:translate-y-px active:bg-surface-2 active:shadow-none",
    ),
    // Ghost. Nothing until pointed at, then a fill — the quiet actions in a
    // row of controls, where a border on each would fence the row into stripes.
    // No shadow at any point: it is not a raised surface, so lifting it on
    // hover would be a lie about what it is.
    tertiary: "bg-transparent text-ink-subtle hover:bg-surface-2 hover:text-ink active:bg-surface-3",
    danger: cx(
      "bg-canvas text-danger-strong border border-danger/30 shadow-sm",
      "hover:bg-danger-surface hover:border-danger/60 hover:shadow-layered",
      "active:translate-y-px active:shadow-none",
    ),
  };

  const sizes: Record<Size, string> = {
    // px-xs, not px-sm. A ghost button's padding is invisible, so at 12px two
    // adjacent labels sit ~32px apart with nothing between them to explain the
    // gap — which is what made the queue row's Pause and Cancel look unrelated
    // to each other and to the row.
    sm: "h-7 px-xs gap-xxs text-button",
    md: "h-9 px-md gap-xs text-button",
    // The document's button-md: 16px labels. Kept for the one action a
    // screen exists to take, so it is the largest thing you can press.
    lg: "h-12 px-lg gap-xs text-subhead",
    // Square, for a glyph with no label. The accessible name has to come from
    // aria-label or a tooltip.
    icon: "size-7 gap-0 text-button",
  };

  return (
    <button
      type="button"
      disabled={disabled || busy}
      className={cx(
        "no-drag inline-flex shrink-0 items-center justify-center rounded-sm",
        "whitespace-nowrap",
        "transition-[background-color,border-color,color,box-shadow,transform] duration-150 ease-standard",
        // Disabled resolves to documented colours rather than an opacity fade,
        // so a disabled primary does not read as a dimmer primary.
        "disabled:pointer-events-none disabled:border-transparent disabled:bg-surface-2 disabled:from-surface-2 disabled:to-surface-2 disabled:text-ink-faint disabled:shadow-none",
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
