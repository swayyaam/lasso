import { useState } from "react";
import { Button, Icon, Tooltip, cx } from "@lasso/ui";
import { api, history } from "../bindings";
import { describeFile, formatWhen, kindOf } from "../format";
import { cleanError } from "../hooks/useLink";
import { MediaThumb } from "./MediaThumb";

/** HISTORY_ROW_HEIGHT is fixed so the full history can be virtualised. */
export const HISTORY_ROW_HEIGHT = 79;

/**
 * HistoryRow is one past download: its picture, what it is, when it arrived,
 * and the two things anyone wants from a finished download — open it, or find
 * it. Both are labelled; an icon alone made "show in Finder" a guess.
 *
 * `full` adds what only the History screen needs: running a download again
 * and forgetting it. Recent keeps to the two actions.
 */
export function HistoryRow({
  entry,
  full = false,
  onAgain,
}: {
  entry: history.Entry;
  full?: boolean;
  onAgain?: () => void;
}) {
  // Acting on a past download can fail in ways the row has to own: the file
  // may have moved since, and the link may have stopped working.
  const [error, setError] = useState("");

  async function run(action: () => Promise<unknown>, after?: () => void) {
    setError("");
    try {
      await action();
      after?.();
    } catch (e) {
      setError(cleanError(e));
    }
  }

  const done = entry.state === "done" && Boolean(entry.filePath);
  const failed = entry.state === "failed";

  const meta = failed
    ? entry.message || "Did not finish"
    : [entry.uploader, done ? describeFile(entry) : "Cancelled", formatWhen(entry.finishedAt)]
        .filter(Boolean)
        .join(" · ");

  return (
    <div
      className="flex items-center gap-md border-b border-surface-3 py-sm"
      style={{ height: HISTORY_ROW_HEIGHT }}
    >
      <MediaThumb source={entry.thumbnail} kind={kindOf(entry.options?.pick)} duration={entry.duration} />

      <div className="flex min-w-0 flex-1 flex-col gap-hair">
        <span className="truncate text-subhead text-ink" title={entry.title || entry.url} data-selectable>
          {entry.title || entry.url}
        </span>
        <span
          className={cx(
            "truncate text-body-sm",
            error || failed ? "text-danger-strong" : "text-ink-subtle",
          )}
          title={error || meta}
        >
          {error || meta}
        </span>
      </div>

      {done && (
        <>
          <Button size="md" onClick={() => void run(() => api.OpenFile(entry.id))}>
            Open
          </Button>
          <Button
            size="md"
            variant="tertiary"
            icon={<Icon.RevealInFinder className="size-3.5" strokeWidth={1.75} aria-hidden />}
            onClick={() => void run(() => api.RevealInFinder(entry.id))}
          >
            Show in Finder
          </Button>
        </>
      )}

      {full && (
        <>
          <Tooltip label="Download again with the same choices">
            <Button
              size="icon"
              variant="tertiary"
              aria-label="Download again"
              onClick={() => void run(() => api.DownloadAgain(entry.id), onAgain)}
            >
              <Icon.Retry className="size-3.5" strokeWidth={1.75} aria-hidden />
            </Button>
          </Tooltip>
          <Tooltip label="Forget this download (the file stays)">
            <Button
              size="icon"
              variant="tertiary"
              aria-label="Forget this download"
              onClick={() => void run(() => api.ForgetHistoryEntry(entry.id))}
            >
              <Icon.Cancel className="size-3.5" strokeWidth={1.75} aria-hidden />
            </Button>
          </Tooltip>
        </>
      )}
    </div>
  );
}
