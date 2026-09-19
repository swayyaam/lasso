import type { HTMLAttributes, ReactNode } from "react";
import { cx } from "../cx";

/**
 * Card is the `card-feature` pattern: canvas, a hairline border and 8px
 * corners — the document's Level 1 elevation.
 *
 * A lifted card takes the layered multi-stop drop shadow, which is the
 * system's only atmospheric effect and its signature one: five stops at very
 * low individual opacities rather than a single soft blur.
 *
 * Padding is `md` rather than the document's 32px, which is the agreed
 * compression for app density.
 */
export function Card({
  className,
  lifted = false,
  padded = true,
  children,
  ...rest
}: {
  /** Applies the layered drop shadow, the treatment for a featured card. */
  lifted?: boolean;
  padded?: boolean;
  children: ReactNode;
} & HTMLAttributes<HTMLDivElement>) {
  return (
    <div
      className={cx(
        "rounded-md border bg-canvas",
        lifted ? "border-hairline shadow-layered" : "border-hairline",
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
