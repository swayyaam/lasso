import { useMemo, useState } from "react";
import { Banner, Button, EmptyState, Icon, Input, VirtualList } from "@lasso/ui";
import { api, history } from "../bindings";
import { HISTORY_ROW_HEIGHT, HistoryRow } from "../components/HistoryRow";
import { describeFile } from "../format";

/**
 * HistoryScreen is everything Lasso has saved, newest first, to find again.
 *
 * Search matches the title, the channel and the link — what it was called,
 * who made it, where it came from — and the line under each title, so "4K"
 * or "mp3" finds what they say.
 */
export function HistoryScreen({
  entries,
  onBack,
  onAgain,
}: {
  entries: history.Entry[] | null;
  onBack: () => void;
  onAgain: () => void;
}) {
  const [query, setQuery] = useState("");
  const [confirming, setConfirming] = useState(false);

  const all = entries ?? [];
  const shown = useMemo(() => {
    const words = query.trim().toLowerCase().split(/\s+/).filter(Boolean);
    if (words.length === 0) return all;
    return all.filter((e) => {
      const haystack = `${e.title} ${e.uploader} ${e.url} ${describeFile(e)}`.toLowerCase();
      return words.every((w) => haystack.includes(w));
    });
  }, [all, query]);

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="mx-auto flex w-full max-w-page shrink-0 flex-col gap-md px-xl pt-md">
        <div>
          <Button
            size="sm"
            variant="tertiary"
            onClick={onBack}
            icon={<Icon.Back className="size-3.5" strokeWidth={1.75} aria-hidden />}
          >
            Back
          </Button>
        </div>
        <div className="flex items-end justify-between gap-md">
          <div className="flex flex-col gap-hair">
            <h1 className="text-display text-ink">History</h1>
            <span className="text-body-sm text-ink-subtle tabular-nums">
              {query.trim()
                ? `${shown.length} of ${all.length} ${all.length === 1 ? "download" : "downloads"}`
                : `${all.length} ${all.length === 1 ? "download" : "downloads"}`}
            </span>
          </div>
          {all.length > 0 && (
            <div className="flex items-center gap-xs">
              <Input
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                placeholder="Search by title, channel or link"
                aria-label="Search history"
                className="w-72"
                leading={<Icon.Find className="size-3.5 text-ink-subtle" strokeWidth={1.75} aria-hidden />}
                onKeyDown={(e) => {
                  if (e.key === "Escape" && query) {
                    e.stopPropagation();
                    setQuery("");
                  }
                }}
              />
              <Button variant="tertiary" onClick={() => setConfirming(true)}>
                Clear history
              </Button>
            </div>
          )}
        </div>

        {confirming && (
          <Banner
            title={`Forget all ${all.length} ${all.length === 1 ? "download" : "downloads"}?`}
            tone="neutral"
            action={
              <div className="flex items-center gap-xs">
                <Button
                  size="sm"
                  variant="danger"
                  onClick={() => {
                    setConfirming(false);
                    void api.ClearHistory();
                  }}
                >
                  Forget all
                </Button>
                <Button size="sm" variant="tertiary" onClick={() => setConfirming(false)}>
                  Keep
                </Button>
              </div>
            }
          >
            Only the list is cleared. The files stay where they are.
          </Banner>
        )}
      </div>

      <div className="mx-auto mt-sm flex min-h-0 w-full max-w-page flex-1 flex-col px-xl pb-md">
        {entries === null ? null : all.length === 0 ? (
          <EmptyState
            tone="purple"
            icon={<Icon.History className="size-5" strokeWidth={1.75} aria-hidden />}
            title="Nothing saved yet"
            description="Everything Lasso downloads is listed here, so you can open it, find it, or download it again."
          />
        ) : shown.length === 0 ? (
          <EmptyState
            title="Nothing matches"
            description="Try part of the title, the channel's name, or the site."
          />
        ) : (
          <div className="flex min-h-0 flex-1 flex-col border-t border-hairline">
            <VirtualList
              items={shown}
              rowHeight={HISTORY_ROW_HEIGHT}
              getKey={(entry) => entry.id}
              renderRow={(entry) => <HistoryRow entry={entry} full onAgain={onAgain} />}
              className="min-h-0 flex-1"
            />
          </div>
        )}
      </div>
    </div>
  );
}
