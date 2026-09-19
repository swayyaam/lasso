import { useEffect, useState } from "react";
import { Button, Details, Icon, MonoBlock, ProgressBar, StatusBadge, Tooltip, cx } from "@lasso/ui";
import type { Tone } from "@lasso/ui";
import { api, core } from "../bindings";
import { useDoctor } from "./DoctorPanel";
import { basename, formatEta, formatSpeed } from "../format";

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
  paused: "Paused",
  done: "Done",
  failed: "Failed",
  cancelled: "Cancelled",
};

const STATE_TONES: Record<string, Tone> = {
  done: "success",
  failed: "danger",
  cancelled: "neutral",
  queued: "neutral",
  paused: "neutral",
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
  // Opening or revealing can fail — the file may have been moved or deleted
  // since it finished. Reporting that in the status line keeps the row's fixed
  // height, which the virtualised list depends on.
  const [actionError, setActionError] = useState("");
  const openDoctor = useDoctor();
  // Whether a failure looks like a broken install rather than a bad link is
  // decided in Go, so the rule has one definition and is testable there.
  const [suggestsDoctor, setSuggestsDoctor] = useState(false);

  async function act(action: () => Promise<unknown>) {
    setActionError("");
    try {
      await action();
    } catch (e) {
      setActionError(String(e).replace(/^Error:\s*/, "").trim() || "That did not work.");
    }
  }

  useEffect(() => {
    if (item.state !== "failed" || !item.errorKind) {
      setSuggestsDoctor(false);
      return;
    }
    let cancelled = false;
    api.ShouldSuggestDoctor(item.errorKind).then((suggest) => {
      if (!cancelled) setSuggestsDoctor(suggest);
    });
    return () => {
      cancelled = true;
    };
  }, [item.state, item.errorKind]);

  const state = item.state;
  const progress = item.progress;
  const terminal = state === "done" || state === "failed" || state === "cancelled";
  const failed = state === "failed";
  const paused = state === "paused";
  // Pausing is only meaningful while bytes are moving. Post-processing is
  // ffmpeg working on a complete file, and stopping that just wastes it.
  const pausable = state === "downloading";

  const percent = progress?.percent ?? -1;
  // A paused row keeps its bar so the gap between where it stopped and the end
  // is visible — that is the whole difference between paused and not started.
  const showBar = state === "downloading" || state === "post-processing" || paused;

  return (
    <div
      className={cx(
        "flex flex-col gap-xs border-b border-hairline px-md py-sm",
        failed && "bg-danger-surface",
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
        <span
          className={cx(
            "min-w-0 flex-1 truncate text-caption tabular-nums",
            actionError ? "text-danger-strong" : "text-ink-subtle",
          )}
          title={actionError || statusLine(item)}
        >
          {actionError || statusLine(item)}
        </span>

        {pausable && (
          <Button
            size="sm"
            variant="tertiary"
            onClick={() => void api.Pause(item.id)}
            icon={<Icon.Pause className="size-3.5" strokeWidth={1.75} aria-hidden />}
          >
            Pause
          </Button>
        )}
        {paused && (
          <Button
            size="sm"
            variant="primary"
            onClick={() => void api.Resume(item.id)}
            icon={<Icon.Resume className="size-3.5" strokeWidth={1.75} aria-hidden />}
          >
            Resume
          </Button>
        )}
        {!terminal && (
          <Button
            size="sm"
            variant="tertiary"
            onClick={() => void api.Cancel(item.id)}
            icon={<Icon.Cancel className="size-3.5" strokeWidth={1.75} aria-hidden />}
          >
            Cancel
          </Button>
        )}
        {(state === "failed" || state === "cancelled") && (
          <Button
            size="sm"
            variant="primary"
            onClick={() => void api.Retry(item.id)}
            icon={<Icon.Retry className="size-3.5" strokeWidth={1.75} aria-hidden />}
          >
            Retry
          </Button>
        )}
        {state === "done" && item.filePath && (
          <>
            <Button
              size="sm"
              variant="tertiary"
              onClick={() => void act(() => api.OpenFile(item.filePath))}
              icon={<Icon.OpenFile className="size-3.5" strokeWidth={1.75} aria-hidden />}
            >
              Open
            </Button>
            <Tooltip label="Show in Finder">
              <Button
                size="icon"
                variant="tertiary"
                aria-label="Show in Finder"
                onClick={() => void act(() => api.RevealInFinder(item.filePath))}
              >
                <Icon.RevealInFinder className="size-3.5" strokeWidth={1.75} aria-hidden />
              </Button>
            </Tooltip>
          </>
        )}
        {(terminal || paused) && (
          // Clearing the row is tidying the queue, not forgetting the download:
          // its history entry stays. Icon-only, because it is the least
          // important control in the row and a label would give it equal
          // weight with Retry.
          <Tooltip label="Remove from queue">
            <Button
              size="icon"
              variant="tertiary"
              aria-label="Remove from queue"
              onClick={() => void api.RemoveFromQueue(item.id)}
            >
              <Icon.Cancel className="size-3.5" strokeWidth={1.75} aria-hidden />
            </Button>
          </Tooltip>
        )}
      </div>

      {failed && item.message && (
        <div className="flex flex-col gap-xs pt-xxs">
          <p className="text-caption text-danger-strong">{item.message}</p>

          {suggestsDoctor && (
            // This failure does not look like the site's answer, so the next
            // useful step is checking Lasso's own setup rather than trying the
            // same link again.
            <div>
              <Button
                size="sm"
                variant="secondary"
                onClick={openDoctor}
                icon={<Icon.Doctor className="size-3.5" strokeWidth={1.75} aria-hidden />}
              >
                Run diagnostics
              </Button>
            </div>
          )}

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
    case "paused":
      // Naming where it stopped is what makes resuming feel like continuing.
      return p?.percent >= 0 ? `Paused at ${p.percent.toFixed(0)}%` : "Paused";
    case "done":
      // item.filePath, not progress.filename: the latter names the file being
      // written, which a post-processor deletes on its way to the real one.
      return basename(item.filePath) || "Finished";
    case "cancelled":
      return "Cancelled";
    default:
      return "";
  }
}
