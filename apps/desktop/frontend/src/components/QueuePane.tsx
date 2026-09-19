import { useMemo } from "react";
import { Button, EmptyState, VirtualList } from "@lasso/ui";
import { core } from "../bindings";
import { QUEUE_ROW_HEIGHT, QueueRow } from "./QueueRow";

/**
 * QueuePane lists everything queued, running and finished this session.
 *
 * ActivityPane owns the header and the tab it sits behind; this renders only
 * the list, so the two tabs cannot drift apart on spacing.
 *
 * The list is virtualised because a playlist can add hundreds of items at once
 * and each one is emitting progress. Failed rows are rendered outside the
 * virtualised list, above it: there are only ever a few, they need more height
 * than a uniform row allows, and putting them first means a failure is the
 * first thing seen rather than something to scroll for.
 */
export function QueuePane({ items }: { items: core.Item[] }) {
  const { failed, rest } = useMemo(() => {
    const failed: core.Item[] = [];
    const rest: core.Item[] = [];
    // An item carrying a notice needs the same extra height a failure does,
    // so it is pinned rather than squeezed into a uniform virtualised row.
    for (const item of items) {
      const needsRoom = item.state === "failed" || (item.state === "done" && Boolean(item.notice));
      (needsRoom ? failed : rest).push(item);
    }
    return { failed, rest };
  }, [items]);

  return (
    <>
      {items.length === 0 ? (
        <EmptyState
          title="Nothing downloading"
          description="Paste a link on the left and pick a quality. Downloads appear here with progress, and you can cancel or retry any of them."
        />
      ) : (
        <div className="flex min-h-0 flex-1 flex-col">
          {/* Failures are pinned above the scrolling list rather than mixed
              into it: they need more height than a uniform row allows, and a
              failure should not be something you have to scroll to find. The
              cap stops a run of failures swallowing the whole pane. */}
          {failed.length > 0 && (
            <div className="max-h-1/2 shrink-0 overflow-y-auto border-b border-hairline-strong">
              {failed.map((item) => (
                <QueueRow key={item.id} item={item} expanded />
              ))}
            </div>
          )}

          <VirtualList
            items={rest}
            rowHeight={QUEUE_ROW_HEIGHT}
            getKey={(item) => item.id}
            renderRow={(item) => <QueueRow item={item} expanded={false} />}
            className="min-h-0 flex-1"
          />
        </div>
      )}
    </>
  );
}

/** QueueError is shown when the queue itself cannot run. */
export function QueueUnavailable({ onOpenSettings }: { onOpenSettings: () => void }) {
  return (
    <EmptyState
      title="Downloads are unavailable"
      description="Lasso's helper programs did not start, so nothing can be downloaded yet."
      action={
        <Button variant="secondary" onClick={onOpenSettings}>
          Open Settings
        </Button>
      }
    />
  );
}
