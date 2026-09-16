import type { HTMLAttributes, ReactNode } from "react";
import { cx } from "../cx";

/**
 * Card is the `feature-card` pattern: a surface-1 panel with a 12px radius and
 * a hairline border. Depth comes from the surface ladder, never a shadow.
 *
 * Padding is `md` rather than the document's `lg`, which is the agreed
 * compression for app density.
 */
export function Card({
  className,
  lifted = false,
  padded = true,
  children,
  ...rest
}: {
  /** Lifts to surface-2, the document's treatment for a featured card. */
  lifted?: boolean;
  padded?: boolean;
  children: ReactNode;
} & HTMLAttributes<HTMLDivElement>) {
  return (
    <div
      className={cx(
        "rounded-lg border",
        lifted ? "border-hairline-strong bg-surface-2" : "border-hairline bg-surface-1",
        padded && "p-md",
        className,
      )}
      {...rest}
    >
      {children}
    </div>
  );
}

/** Eyebrow labels a group. Its positive tracking is what marks it as taxonomy. */
export function Eyebrow({ children, className }: { children: ReactNode; className?: string }) {
  return (
    <div className={cx("text-eyebrow font-medium tracking-wide text-ink-subtle uppercase", className)}>
      {children}
    </div>
  );
}
