import { cx } from "../cx";

export type Tone = "neutral" | "active" | "success" | "danger";

/**
 * StatusBadge is the `status-badge` pill: caption type on surface-2.
 *
 * Only two tones carry colour. Success is the one semantic colour design.md
 * allows, and danger is the app-only addition for failures — everything else
 * stays neutral so those two remain findable at a glance in a long queue.
 */
export function StatusBadge({ tone = "neutral", children }: { tone?: Tone; children: React.ReactNode }) {
  const tones: Record<Tone, string> = {
    neutral: "bg-surface-2 text-ink-subtle",
    active: "bg-surface-2 text-ink-muted",
    success: "bg-success/15 text-success",
    danger: "bg-danger/15 text-danger",
  };

  return (
    <span
      className={cx(
        "inline-flex shrink-0 items-center rounded-pill px-2 py-0.5 text-caption font-medium",
        tones[tone],
      )}
    >
      {children}
    </span>
  );
}
