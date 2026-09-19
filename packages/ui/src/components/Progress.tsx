import { cx } from "../cx";

/**
 * ProgressBar shows download progress.
 *
 * A negative percent means the total size is not known yet, which is normal
 * for fragmented and live streams; that renders as an indeterminate sweep
 * rather than a bar stuck at zero.
 *
 * The fill carries the category colour for what is happening — blue while
 * bytes are moving, purple while ffmpeg is working on a file that has already
 * arrived. Those are genuinely different waits: the first has an end you can
 * predict from the speed, and the second does not, so it is worth being able
 * to tell them apart from across the room.
 */
export function ProgressBar({
  percent,
  tone = "downloading",
}: {
  percent: number;
  tone?: "downloading" | "processing" | "paused" | "danger";
}) {
  const indeterminate = percent < 0;

  const fills: Record<string, string> = {
    downloading: "bg-accent-blue-deep",
    processing: "bg-accent-purple",
    // A paused bar is a fact, not an activity, so it drops to ink.
    paused: "bg-ink-tertiary",
    danger: "bg-danger",
  };

  return (
    <div
      // shrink-0 matters: the bar lives in a fixed-height row, and without it
      // flexbox collapses a 4px track to nothing the moment content grows.
      className="h-1 w-full shrink-0 overflow-hidden rounded-pill bg-surface-3"
      role="progressbar"
      aria-valuenow={indeterminate ? undefined : Math.round(percent)}
      aria-valuemin={0}
      aria-valuemax={100}
    >
      <div
        className={cx(
          "h-full rounded-pill",
          fills[tone] ?? fills.downloading,
          indeterminate ? "w-1/3 animate-[sweep_1.4s_ease-in-out_infinite]" : "transition-[width] duration-200 ease-out",
        )}
        style={indeterminate ? undefined : { width: `${Math.min(100, Math.max(0, percent))}%` }}
      />
    </div>
  );
}
