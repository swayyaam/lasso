import type { CSSProperties, ReactNode } from "react";
import { cx } from "../cx";
import { Spinner } from "./Button";

/**
 * EmptyState fills a pane that has nothing in it yet.
 *
 * It says what will appear and how to make it appear, rather than just
 * announcing emptiness.
 */
export function EmptyState({
  title,
  description,
  action,
  className,
}: {
  title: string;
  description?: ReactNode;
  action?: ReactNode;
  className?: string;
}) {
  return (
    <div className={cx("flex flex-col items-center justify-center gap-xs px-lg py-xxl text-center", className)}>
      <p className="text-body-sm font-medium text-ink-muted">{title}</p>
      {description && <p className="max-w-note text-caption text-ink-tertiary">{description}</p>}
      {action && <div className="mt-xs">{action}</div>}
    </div>
  );
}

/** LoadingState is the waiting counterpart to EmptyState. */
export function LoadingState({ label, className }: { label: string; className?: string }) {
  return (
    <div className={cx("flex items-center justify-center gap-xs px-lg py-xxl text-ink-subtle", className)}>
      <Spinner />
      <span className="text-body-sm">{label}</span>
    </div>
  );
}

/**
 * Skeleton is a placeholder block for content whose shape is known before its
 * contents are. Used while a link is resolving, so the composer does not jump
 * when the real card arrives.
 */
export function Skeleton({ className, style }: { className?: string; style?: CSSProperties }) {
  return <div className={cx("animate-pulse rounded-md bg-surface-2", className)} style={style} />;
}

/** Banner carries a blocking problem, such as a helper program that will not run. */
export function Banner({
  tone = "danger",
  title,
  children,
  action,
}: {
  tone?: "danger" | "neutral";
  title: string;
  children?: ReactNode;
  action?: ReactNode;
}) {
  return (
    <div
      className={cx(
        "flex items-start gap-sm rounded-lg border p-sm",
        tone === "danger" ? "border-danger/40 bg-danger-surface" : "border-hairline bg-surface-1",
      )}
    >
      <div className="min-w-0 flex-1">
        <p className={cx("text-body-sm font-medium", tone === "danger" ? "text-danger" : "text-ink")}>{title}</p>
        {children && <div className="mt-xxs text-caption text-ink-muted">{children}</div>}
      </div>
      {action && <div className="shrink-0">{action}</div>}
    </div>
  );
}
