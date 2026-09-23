import { history } from "../bindings";
import { HistoryRow } from "../components/HistoryRow";
import { LinkField } from "../components/LinkField";

/** How many past downloads Ready shows before "Show all". */
const RECENT = 5;

/**
 * ReadyScreen is Lasso with nothing going on: one thing to do — paste — and
 * the last few things it saved, where an empty queue pane used to be.
 */
export function ReadyScreen({
  entries,
  onSubmit,
  onShowHistory,
  disabled,
}: {
  entries: history.Entry[] | null;
  onSubmit: (url: string) => void;
  onShowHistory: () => void;
  disabled: boolean;
}) {
  const recent = (entries ?? []).filter((e) => e.state === "done").slice(0, RECENT);

  return (
    <div className="mx-auto flex w-full max-w-page flex-col gap-xxl px-xl pt-xxl pb-xl">
      <section className="flex flex-col gap-sm">
        <h1 className="text-display text-ink">Save a video or a song</h1>
        <LinkField
          size="large"
          autoFocus
          disabled={disabled}
          onSubmit={onSubmit}
          placeholder="Paste a link from YouTube, SoundCloud or a thousand other sites"
        />
        <p className="text-body-sm text-ink-subtle">
          Or paste anywhere in this window. A playlist opens as a list of its videos, so you can pick
          which ones to keep.
        </p>
      </section>

      {recent.length > 0 && (
        <section className="flex flex-col gap-xs">
          <div className="flex items-baseline justify-between">
            <h2 className="text-card-title text-ink">Recent</h2>
            <button
              type="button"
              onClick={onShowHistory}
              className="no-drag rounded-sm text-body-sm font-medium text-info-strong hover:text-ink"
            >
              Show all
            </button>
          </div>
          <div className="border-t border-hairline">
            {recent.map((entry) => (
              <HistoryRow key={entry.id} entry={entry} />
            ))}
          </div>
        </section>
      )}
    </div>
  );
}
