import * as RadixTooltip from "@radix-ui/react-tooltip";
import type { ReactNode } from "react";
import { cx } from "../cx";

/**
 * TooltipProvider belongs once, at the root of the app.
 *
 * The delay is shared from here rather than per-tooltip, so moving between two
 * icon buttons does not re-wait: Radix skips the delay while the user is
 * already inside a tooltip group.
 */
export function TooltipProvider({ children }: { children: ReactNode }) {
  return (
    <RadixTooltip.Provider delayDuration={400} skipDelayDuration={200}>
      {children}
    </RadixTooltip.Provider>
  );
}

/**
 * Tooltip labels a control whose meaning is carried by an icon.
 *
 * This is Radix rather than a hand-rolled popover because the hard parts are
 * not the visuals: dismissing on Escape, not trapping focus, staying inside
 * the viewport, and — the one most often missed — not appearing on touch, where
 * there is no hover to reveal it.
 *
 * It is a label, never the only place information lives. An icon button still
 * carries its own aria-label; this is what a sighted user gets in place of it.
 */
export function Tooltip({
  label,
  side = "top",
  children,
}: {
  label: ReactNode;
  side?: "top" | "right" | "bottom" | "left";
  children: ReactNode;
}) {
  return (
    <RadixTooltip.Root>
      <RadixTooltip.Trigger asChild>{children}</RadixTooltip.Trigger>
      <RadixTooltip.Portal>
        <RadixTooltip.Content
          side={side}
          sideOffset={6}
          className={cx(
            // Polarity-flipped, the way the document's card-feature-dark is:
            // a tooltip is transient and should not read as another card.
            "z-50 rounded-sm bg-inverse-canvas px-xs py-xxs",
            "text-caption text-inverse-ink shadow-layered-strong",
            "select-none",
          )}
        >
          {label}
          <RadixTooltip.Arrow className="fill-inverse-canvas" width={10} height={5} />
        </RadixTooltip.Content>
      </RadixTooltip.Portal>
    </RadixTooltip.Root>
  );
}
