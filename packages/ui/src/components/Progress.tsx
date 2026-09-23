import { cx } from "../cx";

/**
 * ProgressBar shows download progress.
 *
 * A negative percent means the total size is not known yet, which is normal
 * for fragmented and live streams; that renders as an indeterminate sweep
 * rather than a bar stuck at zero.
 *
 * The fill is a category colour. The app's own screens use the colour of what
 * is being saved — purple for video, green for audio — so a row reads the same
 * as the choice that started it. The older blue and purple pair, bytes moving
 * against ffmpeg working, remains for anything without a kind.
 */
export function ProgressBar({
  percent,
  tone = "downloading",
  size = "sm",
  label,
}: {
  percent: number;
  /**
   * video and audio are the category colours, so a bar says what is being
   * saved as well as how far along it is — the same purple and green as the
   * choice that started it.
   */
  tone?: "downloading" | "processing" | "paused" | "danger" | "video" | "audio";
  /** md is for the one download a screen is focused on. */
  size?: "sm" | "md";
  /** Names the bar for assistive technology, usually the download's title. */
  label?: string;
}) {
  const indeterminate = percent < 0;

  const fills: Record<string, string> = {
    downloading: "bg-accent-blue-deep",
    processing: "bg-accent-purple",
    // A paused bar is a fact, not an activity, so it drops to ink.
    paused: "bg-ink-tertiary",
    danger: "bg-danger",
    video: "bg-accent-purple",
    audio: "bg-accent-green",
  };

  return (
    <div
      // shrink-0 matters: the bar lives in a fixed-height row, and without it
      // flexbox collapses a 4px track to nothing the moment content grows.
      className={cx("w-full shrink-0 overflow-hidden rounded-pill bg-surface-3", size === "md" ? "h-1.5" : "h-1")}
      role="progressbar"
      aria-label={label}
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
