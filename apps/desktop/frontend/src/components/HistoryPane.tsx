import { useState } from "react";
import { Button, EmptyState, StatusBadge, VirtualList, cx } from "@lasso/ui";
import type { Tone } from "@lasso/ui";
import { api, history } from "../bindings";
import { basename, formatBytes } from "../format";

/**
 * HISTORY_ROW_HEIGHT matches the queue's row height for the same reason: the
 * list is virtualised arithmetically, so every row must be this tall.
 */
export const HISTORY_ROW_HEIGHT = 76;

const STATE_TONES: Record<string, Tone> = {
  done: "success",
  failed: "danger",
  cancelled: "neutral",
};

const STATE_LABELS: Record<string, string> = {
  done: "Done",
  failed: "Failed",
  cancelled: "Cancelled",
};

/**
 * HistoryPane is the record of what Lasso has downloaded.
 *
 * The queue describes work in flight and is discarded when the app quits; this
 * outlives it. Every row can be run again with exactly the options it used the
 * first time, which is the reason the entry keeps them whole.
 */
export function HistoryPane({ entries }: { entries: history.Entry[] | null }) {
  if (entries === null) return null;

  if (entries.length === 0) {
    return (
      <EmptyState
        title="No downloads yet"
        description="Finished downloads are listed here, so you can find the file again or run the same download a second time."
      />
    );
  }

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <VirtualList
        items={entries}
        rowHeight={HISTORY_ROW_HEIGHT}
        getKey={(entry) => entry.id}
        renderRow={(entry) => <HistoryRow entry={entry} />}
        className="min-h-0 flex-1"
      />
    </div>
  );
}

function HistoryRow({ entry }: { entry: history.Entry }) {
  // Acting on a past download can fail in ways the row has to own: the file may
  // have been moved since, and the link may have stopped working.
  const [error, setError] = useState("");

  async function run(action: () => Promise<unknown>) {
    setError("");
    try {
      await action();
    } catch (e) {
      setError(String(e).replace(/^Error:\s*/, "").trim() || "That did not work.");
    }
  }

  return (
    <div
      className="flex flex-col justify-center gap-xxs border-b border-hairline px-md py-sm"
      style={{ height: HISTORY_ROW_HEIGHT }}
    >
      <div className="flex min-w-0 items-center gap-sm">
        <p className="min-w-0 flex-1 truncate text-body-sm text-ink" title={entry.title || entry.url}>
          {entry.title || entry.url}
        </p>
        <StatusBadge tone={STATE_TONES[entry.state] ?? "neutral"}>
          {STATE_LABELS[entry.state] ?? entry.state}
        </StatusBadge>
      </div>

      <div className="flex min-w-0 items-center gap-xs">
        <span
          className={cx(
            "min-w-0 flex-1 truncate text-caption tabular-nums",
            error ? "text-danger" : "text-ink-tertiary",
          )}
          title={error || entry.filePath || entry.url}
        >
          {error || describe(entry)}
        </span>

        {entry.state === "done" && entry.filePath && (
          <Button size="sm" variant="tertiary" onClick={() => void run(() => api.RevealInFinder(entry.filePath))}>
            Show
          </Button>
        )}
        <Button size="sm" variant="tertiary" onClick={() => void run(() => api.DownloadAgain(entry.id))}>
          Again
        </Button>
        <Button size="sm" variant="tertiary" onClick={() => void run(() => api.ForgetHistoryEntry(entry.id))}>
          Forget
        </Button>
      </div>
    </div>
  );
}

/** describe is the secondary line: what the file is and when it arrived. */
function describe(entry: history.Entry): string {
  if (entry.state === "failed" && entry.message) return entry.message;

  const parts = [
    basename(entry.filePath),
    formatBytes(entry.bytes),
    formatWhen(entry.finishedAt),
  ].filter(Boolean);
  return parts.join(" · ");
}

/**
 * formatWhen is a relative time, which is what a person actually wants from a
 * download list — "2 hours ago" locates something better than a timestamp does.
 */
export function formatWhen(millis: number): string {
  if (!Number.isFinite(millis) || millis <= 0) return "";

  const seconds = Math.round((Date.now() - millis) / 1000);
  if (seconds < 60) return "just now";
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m ago`;
  if (seconds < 86400) return `${Math.floor(seconds / 3600)}h ago`;
  if (seconds < 604800) return `${Math.floor(seconds / 86400)}d ago`;
  return new Date(millis).toLocaleDateString();
}
