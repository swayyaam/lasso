import { Button, Details, MonoBlock, ProgressBar, StatusBadge, cx } from "@lasso/ui";
import type { Tone } from "@lasso/ui";
import { api, core } from "../bindings";
import { formatEta, formatSpeed } from "../format";

/**
 * QUEUE_ROW_HEIGHT must be at least the row's natural height.
 *
 * Virtualisation needs a fixed row height, but setting it too low does not
 * clip — it makes flexbox shrink the row's children, and the first thing to
 * vanish is the 4px progress track. Measured natural height is 96px.
 */
export const QUEUE_ROW_HEIGHT = 100;

const STATE_LABELS: Record<string, string> = {
  queued: "Queued",
  fetching: "Getting details",
  downloading: "Downloading",
  "post-processing": "Finishing",
  done: "Done",
  failed: "Failed",
  cancelled: "Cancelled",
};

const STATE_TONES: Record<string, Tone> = {
  done: "success",
  failed: "danger",
  cancelled: "neutral",
  queued: "neutral",
  fetching: "active",
  downloading: "active",
  "post-processing": "active",
};

/**
 * QueueRow is one download.
 *
 * Rows are a fixed height so the list can be virtualised arithmetically. A
 * failed row is the exception: it expands to carry the explanation and the raw
 * log, and the list gives it the extra height it needs.
 */
export function QueueRow({ item, expanded }: { item: core.Item; expanded: boolean }) {
  const state = item.state;
  const progress = item.progress;
  const terminal = state === "done" || state === "failed" || state === "cancelled";
  const failed = state === "failed";

  const percent = progress?.percent ?? -1;
  const showBar = state === "downloading" || state === "post-processing";

  return (
    <div
      className={cx(
        "flex flex-col gap-xs border-b border-hairline px-md py-sm",
        failed && "bg-danger-surface/40",
      )}
      style={{ height: expanded ? undefined : QUEUE_ROW_HEIGHT }}
    >
      <div className="flex min-w-0 items-start gap-sm">
        <div className="min-w-0 flex-1">
          <p className="truncate text-body-sm text-ink" title={item.title || item.options?.url}>
            {item.title || item.options?.url}
          </p>
          <p className="mt-0.5 truncate text-caption text-ink-tertiary">
            {describe(item)}
          </p>
        </div>
        <StatusBadge tone={STATE_TONES[state] ?? "neutral"}>{STATE_LABELS[state] ?? state}</StatusBadge>
      </div>

      {showBar ? (
        <ProgressBar percent={percent} />
      ) : (
        // Keeps the row height identical whether or not a bar is showing.
        <div className="h-1 shrink-0" aria-hidden />
      )}

      <div className="flex min-w-0 items-center gap-xs">
        <span className="min-w-0 flex-1 truncate text-caption text-ink-subtle tabular-nums">
          {statusLine(item)}
        </span>

        {!terminal && (
          <Button size="sm" variant="tertiary" onClick={() => void api.Cancel(item.id)}>
            Cancel
          </Button>
        )}
        {(state === "failed" || state === "cancelled") && (
          <Button size="sm" onClick={() => void api.Retry(item.id)}>
            Retry
          </Button>
        )}
        {state === "done" && progress?.filename && (
          <Button size="sm" variant="tertiary" onClick={() => void api.RevealInFinder(progress.filename)}>
            Show in Finder
          </Button>
        )}
      </div>

      {failed && item.message && (
        <div className="flex flex-col gap-xs pt-xxs">
          <p className="text-caption text-danger">{item.message}</p>
          {item.detail && (
            // The raw log stays one click away on every failure, never the
            // first thing a non-technical user is shown.
            <Details summary="Raw log">
              <MonoBlock text={item.detail} maxHeight="12rem" copyable />
            </Details>
          )}
        </div>
      )}

      {state === "done" && item.notice && (
        // The download succeeded with a caveat. It is not a failure, so it is
        // not shown in the error colour, but the reason stays reachable.
        <div className="flex flex-col gap-xs pt-xxs">
          <p className="text-caption text-ink-subtle">{item.notice}</p>
          {item.detail && (
            <Details summary="Why">
              <MonoBlock text={item.detail} maxHeight="10rem" copyable />
            </Details>
          )}
        </div>
      )}
    </div>
  );
}

/** describe names what is being downloaded, in the user's terms. */
function describe(item: core.Item): string {
  const pick = item.options?.pick ?? "";
  const quality = pick.startsWith("audio-") ? `${pick.slice(6).toUpperCase()} audio` : pick || "Best";

  const position = item.progress?.items > 0 ? ` · item ${item.progress.item} of ${item.progress.items}` : "";
  return `${quality}${position}`;
}

/** statusLine is the moving text under the bar. */
function statusLine(item: core.Item): string {
  const p = item.progress;
  switch (item.state) {
    case "queued":
      return "Waiting to start";
    case "fetching":
      return "Getting details…";
    case "downloading": {
      const parts = [
        p?.percent >= 0 ? `${p.percent.toFixed(0)}%` : "",
        formatSpeed(p?.speed ?? 0),
        formatEta(p?.eta ?? 0),
      ].filter(Boolean);
      return parts.length > 0 ? parts.join(" · ") : "Starting…";
    }
    case "post-processing":
      return p?.detail || "Finishing up…";
    case "done":
      return p?.filename || "Finished";
    case "cancelled":
      return "Cancelled";
    default:
      return "";
  }
}
