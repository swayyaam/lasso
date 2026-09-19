import { useState } from "react";
import { Button, Icon, cx } from "@lasso/ui";
import { api, core, history } from "../bindings";
import { HistoryPane } from "./HistoryPane";
import { QueuePane } from "./QueuePane";

type Tab = "queue" | "history";

/**
 * ActivityPane is the right-hand column: what is happening now, and what has
 * happened before.
 *
 * The two are separate views rather than one merged list because they answer
 * different questions. The queue is work in flight and is discarded when Lasso
 * quits; history is the record of what came out of it and outlives the session.
 * Merging them would mean a list that is mostly old on every launch.
 */
export function ActivityPane({
  items,
  entries,
}: {
  items: core.Item[];
  entries: history.Entry[] | null;
}) {
  const [tab, setTab] = useState<Tab>("queue");

  const finished = items.filter(
    (i) => i.state === "done" || i.state === "failed" || i.state === "cancelled",
  ).length;
  // Paused counts as neither: it is not finished, and calling it active would
  // suggest something is still happening.
  const paused = items.filter((i) => i.state === "paused").length;
  const active = items.length - finished - paused;

  return (
    <section className="flex min-h-0 flex-1 flex-col">
      <header className="flex h-10 shrink-0 items-center justify-between gap-sm border-b border-hairline px-md">
        <div className="flex items-center gap-md">
          <TabButton
            selected={tab === "queue"}
            onClick={() => setTab("queue")}
            icon={<Icon.Queue className="size-3.5" strokeWidth={1.75} aria-hidden />}
          >
            Queue
          </TabButton>
          <TabButton
            selected={tab === "history"}
            onClick={() => setTab("history")}
            icon={<Icon.History className="size-3.5" strokeWidth={1.75} aria-hidden />}
          >
            History
          </TabButton>
        </div>

        {tab === "queue" ? (
          <div className="flex items-center gap-xs">
            {items.length > 0 && (
              <span className="text-caption text-ink-tertiary tabular-nums">
                {[active > 0 ? `${active} active` : "", paused > 0 ? `${paused} paused` : "", `${items.length} total`]
                  .filter(Boolean)
                  .join(" · ")}
              </span>
            )}
            {finished > 0 && (
              // Only offered when there is something to clear, so the control
              // never invites a click that would do nothing.
              <Button
                size="sm"
                variant="tertiary"
                onClick={() => void api.ClearFinished()}
                icon={<Icon.Clear className="size-3.5" strokeWidth={1.75} aria-hidden />}
              >
                Clear finished
              </Button>
            )}
          </div>
        ) : (
          entries !== null &&
          entries.length > 0 && (
            <div className="flex items-center gap-xs">
              <span className="text-caption text-ink-tertiary tabular-nums">
                {entries.length} kept
              </span>
              <Button
                size="sm"
                variant="tertiary"
                onClick={() => void api.ClearHistory()}
                icon={<Icon.Clear className="size-3.5" strokeWidth={1.75} aria-hidden />}
              >
                Clear
              </Button>
            </div>
          )
        )}
      </header>

      {tab === "queue" ? <QueuePane items={items} /> : <HistoryPane entries={entries} />}
    </section>
  );
}

/**
 * TabButton is a text tab with a primary underline.
 *
 * The indicator is the brand primary, which is what DESIGN-webflow.md
 * specifies for an active state. Ink weight alone was carrying it before, and
 * two uppercase labels a few shades apart is not a strong enough signal for
 * the control that decides which of two lists you are looking at.
 *
 * The underline is always rendered and only changes colour, so selecting a tab
 * does not move the header by a pixel.
 */
function TabButton({
  selected,
  onClick,
  icon,
  children,
}: {
  selected: boolean;
  onClick: () => void;
  icon?: React.ReactNode;
  children: React.ReactNode;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      aria-current={selected ? "page" : undefined}
      className={cx(
        "no-drag inline-flex h-10 items-center gap-xxs border-b-2 text-eyebrow uppercase",
        "transition-[color,border-color] duration-150 ease-standard",
        selected
          ? "border-primary text-ink"
          : "border-transparent text-ink-tertiary hover:text-ink-subtle",
      )}
    >
      {icon}
      {children}
    </button>
  );
}
