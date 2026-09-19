import { cx } from "../cx";

export type Tone = "neutral" | "active" | "success" | "danger";

/**
 * StatusBadge is the `badge-info-soft` pattern: caption type — the system's
 * signature 550 weight — in a 4px rectangle.
 *
 * Only the outcomes carry colour, and they use the -strong text shade over a
 * tinted surface. The raw accents are fill colours: #00d722 as 12px text on
 * white is around 1.7:1, so a badge that used it would be decorative rather
 * than legible. Everything in flight stays neutral, which is what keeps a
 * finished or failed row findable in a long queue.
 */
export function StatusBadge({ tone = "neutral", children }: { tone?: Tone; children: React.ReactNode }) {
  const tones: Record<Tone, string> = {
    neutral: "bg-surface-2 text-ink-subtle",
    active: "bg-info-surface text-info-strong",
    success: "bg-success-surface text-success-strong",
    danger: "bg-danger-surface text-danger-strong",
  };

  return (
    <span
      className={cx(
        "inline-flex shrink-0 items-center rounded-sm px-xxs py-0.5 text-caption",
        tones[tone],
      )}
    >
      {children}
    </span>
  );
}
