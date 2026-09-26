import { useEffect, useMemo, useState } from "react";
import { Button, Details, Eyebrow, Icon, MonoBlock, ProgressBar, Tooltip, VirtualList, cx } from "@lasso/ui";
import { api, core } from "../bindings";
import { describeClip, describeFile, describePick, formatBytes, formatEta, formatSpeed, isWhole, kindOf } from "../format";
import { LinkField } from "../components/LinkField";
import { MediaThumb } from "../components/MediaThumb";
import { useDoctor, useSuggestsDoctor } from "../components/DoctorPanel";
import { cleanError } from "../hooks/useLink";

const RUNNING = new Set(["fetching", "downloading", "post-processing"]);
const FINISHED = new Set(["done", "cancelled"]);

function isTerminal(state: string) {
  return state === "done" || state === "failed" || state === "cancelled";
}

/** A playlist's downloads, gathered back together. */
type Group = { id: string; title: string; items: core.Item[] };

/**
 * DownloadsScreen is what Lasso is doing: the download in progress as the
 * thing to look at, a playlist as one card rather than hundreds of rows, and
 * what has finished below with the two buttons anyone wants from it.
 */
export function DownloadsScreen({
  items,
  onSubmit,
  disabled,
}: {
  items: core.Item[];
  onSubmit: (url: string) => void;
  disabled: boolean;
}) {
  const view = useMemo(() => arrange(items), [items]);
  const downloading = items.filter((i) => i.state === "downloading");
  const paused = items.filter((i) => i.state === "paused");
  const hasActive = view.focus || view.waiting.length || view.activeGroups.length;
  const hasFinished = view.finished.length || view.finishedGroups.length;

  return (
    <div className="min-h-0 flex-1 overflow-y-auto">
      <div className="mx-auto flex w-full max-w-page flex-col gap-lg px-xl pt-md pb-xl">
        <LinkField size="compact" onSubmit={onSubmit} disabled={disabled} placeholder="Paste another link" />

        {view.failed.length > 0 && (
          // Above what is running: a failure is waiting on a decision, and
          // nothing else on the screen is.
          <section className="flex flex-col gap-xs">
            <Eyebrow>Needs attention</Eyebrow>
            {view.failed.map((item) => (
              <FailedCard key={item.id} item={item} />
            ))}
          </section>
        )}

        {hasActive ? (
          <section className="flex flex-col gap-xs">
            <div className="flex items-center justify-between gap-sm">
              <Eyebrow>Downloading</Eyebrow>
              {downloading.length > 0 ? (
                <Button size="sm" variant="tertiary" onClick={() => downloading.forEach((i) => void api.Pause(i.id))}>
                  Pause all
                </Button>
              ) : paused.length > 0 ? (
                <Button size="sm" variant="tertiary" onClick={() => paused.forEach((i) => void api.Resume(i.id))}>
                  Resume all
                </Button>
              ) : null}
            </div>

            {view.focus && <FocusCard item={view.focus} />}
            {view.waiting.length > 0 && <WaitingList items={view.waiting} />}
            {view.activeGroups.map((group) => (
              <GroupCard key={group.id} group={group} />
            ))}
          </section>
        ) : null}

        {hasFinished ? (
          <section className="flex flex-col gap-xs">
            <div className="flex items-center justify-between gap-sm">
              <Eyebrow>Finished</Eyebrow>
              <Button size="sm" variant="tertiary" onClick={() => void api.ClearFinished()}>
                Clear
              </Button>
            </div>
            {view.finishedGroups.map((group) => (
              <GroupCard key={group.id} group={group} />
            ))}
            {view.finished.length > 0 && (
              <div className="border-t border-hairline">
                {view.finished.map((item) => (
                  <FinishedRow key={item.id} item={item} />
                ))}
              </div>
            )}
          </section>
        ) : null}
      </div>
    </div>
  );
}

/**
 * arrange sorts the queue into what the screen shows.
 *
 * The focus is the first single download that is actually moving, or failing
 * that the first paused one; the rest of the singles wait below it. A playlist's downloads become one group, which is
 * active until every one of them has finished or been cancelled — a failure
 * keeps it active, because it is waiting on a decision.
 */
function arrange(items: core.Item[]) {
  const singles: core.Item[] = [];
  const byGroup = new Map<string, Group>();
  for (const item of items) {
    if (!item.group) {
      singles.push(item);
      continue;
    }
    let group = byGroup.get(item.group);
    if (!group) {
      group = { id: item.group, title: item.groupTitle || "Playlist", items: [] };
      byGroup.set(item.group, group);
    }
    group.items.push(item);
  }

  const active = singles.filter((i) => !isTerminal(i.state));
  // A paused download keeps the card it had: pausing is a moment, and moving
  // it into the list would lose its place and its Resume button's size.
  const focus = active.find((i) => RUNNING.has(i.state)) ?? active.find((i) => i.state === "paused") ?? null;
  const groups = [...byGroup.values()];

  return {
    focus,
    waiting: active.filter((i) => i !== focus),
    failed: singles.filter((i) => i.state === "failed"),
    // Newest first: the one that just finished is the one being looked for.
    finished: singles.filter((i) => FINISHED.has(i.state)).reverse(),
    activeGroups: groups.filter((g) => g.items.some((i) => !FINISHED.has(i.state))),
    finishedGroups: groups.filter((g) => g.items.every((i) => FINISHED.has(i.state))),
  };
}

/** FocusCard is the one download the screen is about. */
function FocusCard({ item }: { item: core.Item }) {
  const kind = kindOf(item.options?.pick);
  const title = item.title || item.options?.url;
  const waiting = item.progress?.stage === "waiting";
  const now = useNow(waiting);

  return (
    <div className="flex items-center gap-lg rounded-md border border-hairline bg-canvas p-lg shadow-layered">
      <MediaThumb source={item.thumbnail} kind={kind} size="large" />
      <div className="flex min-w-0 flex-1 flex-col gap-sm">
        <div className="flex min-w-0 flex-col gap-hair">
          <span className="truncate text-card-title text-ink" title={title}>
            {title}
          </span>
          <span className="truncate text-body-sm text-ink-subtle">
            {[item.uploader, describePick(item.options?.pick), describeClip(item.options?.clip)].filter(Boolean).join(" · ")}
          </span>
        </div>
        {/* Grey while waiting: nothing is moving, and a coloured bar would say
            otherwise. */}
        <ProgressBar percent={barPercent(item)} tone={waiting ? "paused" : kind} size="md" label={title} />
        <div className="flex min-w-0 items-center gap-xs">
          <span className="min-w-0 flex-1 truncate text-body-sm text-ink-muted tabular-nums">
            {transferLine(item, now)}
          </span>
          <ItemControls item={item} labelled />
        </div>
      </div>
    </div>
  );
}

/** How many waiting singles are listed before the rest are summarised. */
const WAITING_SHOWN = 12;

/** WaitingList is every other single download that has not finished. */
function WaitingList({ items }: { items: core.Item[] }) {
  const [all, setAll] = useState(false);
  const shown = all ? items : items.slice(0, WAITING_SHOWN);

  return (
    <div className="overflow-hidden rounded-md border border-hairline bg-canvas">
      {shown.map((item, i) => (
        <QueueLine key={item.id} item={item} flush={i === 0} />
      ))}
      {items.length > WAITING_SHOWN && (
        <button
          type="button"
          onClick={() => setAll((v) => !v)}
          className="no-drag w-full border-t border-surface-3 px-md py-xs text-left text-body-sm font-medium text-info-strong hover:bg-surface-1"
        >
          {all ? "Show fewer" : `Show ${items.length - WAITING_SHOWN} more waiting`}
        </button>
      )}
    </div>
  );
}

/** GROUP_LINE_HEIGHT is fixed so a large playlist can be virtualised. */
const GROUP_LINE_HEIGHT = 52;

/** How many of a playlist's videos a collapsed card shows. */
const GROUP_PREVIEW = 4;

/**
 * GroupCard is a playlist: one header that says how far it has got, the
 * videos that need looking at, and the rest one press away.
 */
function GroupCard({ group }: { group: Group }) {
  const [expanded, setExpanded] = useState(false);
  const fix = useUpdateAndRetry();
  const items = group.items;
  const kind = kindOf(items[0]?.options?.pick);
  const Glyph = kind === "audio" ? Icon.Audio : Icon.Video;

  const done = items.filter((i) => i.state === "done").length;
  const failed = items.filter((i) => i.state === "failed");
  const downloading = items.filter((i) => i.state === "downloading");
  const paused = items.filter((i) => i.state === "paused");
  const finished = items.every((i) => FINISHED.has(i.state));

  // Progress across the playlist: whole videos done, plus how far the ones in
  // flight have got, so the bar moves during a long video rather than jumping.
  const partial = items
    .filter((i) => i.state === "downloading" && i.progress?.percent > 0)
    .reduce((sum, i) => sum + i.progress.percent / 100, 0);
  const percent = items.length ? ((done + partial) / items.length) * 100 : 0;

  // Collapsed, the card shows what needs attention or is moving; with none of
  // that, the next few in line.
  const preview = useMemo(() => {
    const urgent = items.filter((i) => i.state === "failed" || RUNNING.has(i.state) || i.state === "paused");
    const rest = items.filter((i) => !urgent.includes(i) && !isTerminal(i.state));
    return [...urgent, ...rest].slice(0, GROUP_PREVIEW);
  }, [items]);

  const summary = [
    "Playlist",
    describePick(items[0]?.options?.pick),
    `${done} of ${items.length} done`,
    failed.length ? `${failed.length} ${failed.length === 1 ? "needs" : "need"} attention` : "",
  ]
    .filter(Boolean)
    .join(" · ");

  return (
    <div className="overflow-hidden rounded-md border border-hairline bg-canvas">
      <div className="flex items-center gap-md px-md py-sm">
        <span
          className={cx(
            "flex size-8 shrink-0 items-center justify-center rounded-pill",
            kind === "audio" ? "bg-accent-green text-on-green" : "bg-accent-purple text-on-fill",
          )}
          aria-hidden
        >
          <Glyph className="size-4" strokeWidth={1.75} />
        </span>
        <div className="flex min-w-0 flex-1 flex-col">
          <span className="truncate text-subhead text-ink" title={group.title}>
            {group.title}
          </span>
          <span className="truncate text-body-sm text-ink-subtle">{summary}</span>
          {fix.message && <span className="text-body-sm text-ink-muted">{fix.message}</span>}
        </div>

        {downloading.length > 0 && (
          <Button size="sm" variant="tertiary" onClick={() => downloading.forEach((i) => void api.Pause(i.id))}>
            Pause
          </Button>
        )}
        {downloading.length === 0 && paused.length > 0 && (
          <Button size="sm" variant="tertiary" onClick={() => paused.forEach((i) => void api.Resume(i.id))}>
            Resume
          </Button>
        )}
        {failed.length > 0 && !finished && (
          failed.some((i) => i.errorKind === SITE_CHANGED) ? (
            <Button size="sm" variant="tertiary" busy={fix.updating} onClick={() => void fix.run(failed.map((i) => i.id))}>
              {fix.updating ? "Updating yt-dlp…" : "Update yt-dlp and retry"}
            </Button>
          ) : (
            <Button size="sm" variant="tertiary" onClick={() => failed.forEach((i) => void api.Retry(i.id))}>
              Retry {failed.length === 1 ? "failed" : `${failed.length} failed`}
            </Button>
          )
        )}
        {finished && (
          <Button size="sm" variant="tertiary" onClick={() => items.forEach((i) => void api.RemoveFromQueue(i.id))}>
            Remove
          </Button>
        )}
        {(expanded || items.length > preview.length) && (
          <Button size="sm" onClick={() => setExpanded((v) => !v)}>
            {expanded ? "Show less" : `Show all ${items.length}`}
          </Button>
        )}
      </div>

      <div
        className="h-[3px] bg-surface-3"
        role="progressbar"
        aria-label={group.title}
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={Math.round(percent)}
      >
        <div
          className={cx("h-full transition-[width] duration-200 ease-out", kind === "audio" ? "bg-accent-green" : "bg-accent-purple")}
          style={{ width: `${Math.min(100, percent)}%` }}
        />
      </div>

      {expanded ? (
        <div style={{ height: Math.min(items.length, 7) * GROUP_LINE_HEIGHT }} className="flex flex-col">
          <VirtualList
            items={items}
            rowHeight={GROUP_LINE_HEIGHT}
            getKey={(item) => item.id}
            renderRow={(item) => <QueueLine item={item} />}
            className="min-h-0 flex-1"
          />
        </div>
      ) : (
        preview.map((item) => <QueueLine key={item.id} item={item} />)
      )}
    </div>
  );
}

/**
 * QueueLine is one download in a list: a small picture, the title, and either
 * how far it has got or how it ended. Fixed height, so a long playlist can be
 * virtualised; a failure's reason is cut to one line with the whole of it in
 * the tooltip and behind Retry's neighbour.
 */
function QueueLine({ item, flush = false }: { item: core.Item; /** First in its card: no rule above it. */ flush?: boolean }) {
  const kind = kindOf(item.options?.pick);
  const [actionError, setActionError] = useState("");
  const failed = item.state === "failed";
  const title = item.title || item.options?.url;

  return (
    <div
      className={cx("flex items-center gap-sm px-md", !flush && "border-t border-surface-3", failed && "bg-warning-surface")}
      style={{ height: GROUP_LINE_HEIGHT }}
    >
      <MediaThumb source={item.thumbnail} kind={kind} size="small" />
      <div className="flex min-w-0 flex-1 flex-col">
        <span className="truncate text-body-sm font-medium text-ink" title={title}>
          {title}
        </span>
        {failed && (
          <span className="flex min-w-0 items-center gap-xxs text-body-sm text-warning-strong" title={item.message}>
            <Icon.Warn className="size-3.5 shrink-0" strokeWidth={1.75} aria-hidden />
            <span className="truncate">{item.message || "Did not finish"}</span>
          </span>
        )}
        {actionError && <span className="truncate text-body-sm text-danger-strong">{actionError}</span>}
      </div>

      {item.state === "done" ? (
        <>
          <span className="flex shrink-0 items-center gap-xxs text-body-sm text-success-strong">
            <Icon.Ok className="size-3.5" strokeWidth={1.75} aria-hidden />
            {["Done", formatBytes(item.bytes)].filter(Boolean).join(" · ")}
          </span>
          <Button
            size="sm"
            variant="tertiary"
            onClick={() => api.OpenFile(item.id).catch((e) => setActionError(cleanError(e)))}
          >
            Open
          </Button>
        </>
      ) : RUNNING.has(item.state) || item.state === "paused" ? (
        <>
          <div className="w-32 shrink-0">
            <ProgressBar
              percent={barPercent(item)}
              tone={item.state === "paused" || item.progress?.stage === "waiting" ? "paused" : kind}
              label={title}
            />
          </div>
          {item.progress?.stage === "waiting" ? (
            <span className="shrink-0 text-body-sm text-ink-subtle" title={item.progress.detail}>
              Reconnecting
            </span>
          ) : (
            <span className="w-10 shrink-0 text-right text-body-sm text-ink-muted tabular-nums">
              {item.progress?.percent >= 0 ? `${Math.round(item.progress.percent)}%` : ""}
            </span>
          )}
          <ItemControls item={item} />
        </>
      ) : (
        <>
          <span className="shrink-0 text-body-sm text-ink-subtle">
            {item.state === "queued" ? "Waiting" : item.state === "cancelled" ? "Cancelled" : ""}
          </span>
          <ItemControls item={item} />
        </>
      )}
    </div>
  );
}

/**
 * ItemControls are what can be done to a download that has not finished.
 *
 * Pause only while bytes are moving — post-processing is ffmpeg working on a
 * complete file, and stopping that just wastes it.
 */
function ItemControls({ item, labelled = false }: { item: core.Item; labelled?: boolean }) {
  const state = item.state;
  const controls = [];

  if (state === "downloading") {
    controls.push(
      <Control key="pause" label="Pause" labelled={labelled} onClick={() => void api.Pause(item.id)}>
        <Icon.Pause className="size-3.5" strokeWidth={1.75} aria-hidden />
      </Control>,
    );
  }
  if (state === "paused") {
    controls.push(
      <Control key="resume" label="Resume" labelled={labelled} primary onClick={() => void api.Resume(item.id)}>
        <Icon.Resume className="size-3.5" strokeWidth={1.75} aria-hidden />
      </Control>,
    );
  }
  if (state === "failed" || state === "cancelled") {
    controls.push(
      <Control key="retry" label="Retry" labelled onClick={() => void api.Retry(item.id)}>
        <Icon.Retry className="size-3.5" strokeWidth={1.75} aria-hidden />
      </Control>,
    );
  }
  if (!isTerminal(state)) {
    controls.push(
      <Control key="cancel" label="Cancel" labelled={labelled} quiet onClick={() => void api.Cancel(item.id)}>
        <Icon.Cancel className="size-3.5" strokeWidth={1.75} aria-hidden />
      </Control>,
    );
  }
  return <div className="flex shrink-0 items-center gap-hair">{controls}</div>;
}

function Control({
  label,
  labelled,
  primary,
  quiet,
  onClick,
  children,
}: {
  label: string;
  labelled: boolean;
  primary?: boolean;
  quiet?: boolean;
  onClick: () => void;
  children: React.ReactNode;
}) {
  if (labelled) {
    return (
      <Button size="sm" variant={primary ? "primary" : quiet ? "tertiary" : "secondary"} icon={quiet ? undefined : children} onClick={onClick}>
        {label}
      </Button>
    );
  }
  return (
    <Tooltip label={label}>
      <Button size="icon" variant="tertiary" aria-label={label} onClick={onClick}>
        {children}
      </Button>
    </Tooltip>
  );
}

/**
 * FailedCard is a single download that did not finish, with its reason in
 * full and the next step: Retry, and diagnostics when the reason looks like
 * Lasso's own setup rather than the site's answer.
 */
function FailedCard({ item }: { item: core.Item }) {
  const kind = kindOf(item.options?.pick);
  const suggests = useSuggestsDoctor(item.errorKind);
  const openDoctor = useDoctor();
  const title = item.title || item.options?.url;
  const siteChanged = item.errorKind === SITE_CHANGED;
  const fix = useUpdateAndRetry();

  return (
    <div className="flex gap-md rounded-md border border-danger/30 bg-danger-surface p-md">
      <MediaThumb source={item.thumbnail} kind={kind} />
      <div className="flex min-w-0 flex-1 flex-col gap-xxs">
        <span className="truncate text-subhead text-ink" title={title}>
          {title}
        </span>
        <p className="text-body-sm text-danger-strong" data-selectable>
          {item.message || "The download did not finish."}
        </p>
        {fix.message && <p className="text-body-sm text-ink-muted">{fix.message}</p>}
        {item.detail && (
          <Details summary="What yt-dlp said">
            <MonoBlock text={item.detail} maxHeight="12rem" copyable />
          </Details>
        )}
      </div>
      <div className="flex shrink-0 flex-col items-end gap-xxs">
        <div className="flex items-center gap-hair">
          {siteChanged && (
            <Button
              size="sm"
              variant="primary"
              busy={fix.updating}
              icon={<Icon.Recheck className="size-3.5" strokeWidth={1.75} aria-hidden />}
              onClick={() => void fix.run([item.id])}
            >
              {fix.updating ? "Updating yt-dlp…" : "Update yt-dlp and retry"}
            </Button>
          )}
          <Button
            size="sm"
            variant={siteChanged ? "tertiary" : "primary"}
            icon={<Icon.Retry className="size-3.5" strokeWidth={1.75} aria-hidden />}
            onClick={() => void api.Retry(item.id)}
          >
            Retry
          </Button>
          <Tooltip label="Remove from the list">
            <Button size="icon" variant="tertiary" aria-label="Remove" onClick={() => void api.RemoveFromQueue(item.id)}>
              <Icon.Cancel className="size-3.5" strokeWidth={1.75} aria-hidden />
            </Button>
          </Tooltip>
        </div>
        {suggests && (
          <Button
            size="sm"
            variant="tertiary"
            icon={<Icon.Doctor className="size-3.5" strokeWidth={1.75} aria-hidden />}
            onClick={openDoctor}
          >
            Diagnose
          </Button>
        )}
      </div>
    </div>
  );
}

/** FinishedRow is a single download that came to an end this session. */
function FinishedRow({ item }: { item: core.Item }) {
  const [error, setError] = useState("");
  const kind = kindOf(item.options?.pick);
  const done = item.state === "done" && Boolean(item.filePath);
  const title = item.title || item.options?.url;

  async function run(action: () => Promise<unknown>) {
    setError("");
    try {
      await action();
    } catch (e) {
      setError(cleanError(e));
    }
  }

  const meta = done ? [item.uploader, describeFile(item)].filter(Boolean).join(" · ") : "Cancelled";

  return (
    <div className="flex items-center gap-md border-b border-surface-3 py-sm">
      <MediaThumb source={item.thumbnail} kind={kind} duration={item.duration} />
      <div className="flex min-w-0 flex-1 flex-col gap-hair">
        <span className="truncate text-subhead text-ink" title={title}>
          {title}
        </span>
        <span className={cx("truncate text-body-sm", error ? "text-danger-strong" : "text-ink-subtle")} title={error || meta}>
          {error || meta}
        </span>
        {item.notice && (
          // Succeeded with a caveat: not a failure, so not the error colour,
          // but the reason stays reachable.
          <span className="flex min-w-0 items-center gap-xxs text-body-sm text-warning-strong" title={item.detail || item.notice}>
            <Icon.Info className="size-3.5 shrink-0" strokeWidth={1.75} aria-hidden />
            <span className="truncate">{item.notice}</span>
          </span>
        )}
      </div>

      {done ? (
        <>
          <Button onClick={() => void run(() => api.OpenFile(item.id))}>Open</Button>
          <Button
            variant="tertiary"
            icon={<Icon.RevealInFinder className="size-3.5" strokeWidth={1.75} aria-hidden />}
            onClick={() => void run(() => api.RevealInFinder(item.id))}
          >
            Show in Finder
          </Button>
        </>
      ) : (
        <Button
          variant="tertiary"
          icon={<Icon.Retry className="size-3.5" strokeWidth={1.75} aria-hidden />}
          onClick={() => void run(() => api.Retry(item.id))}
        >
          Retry
        </Button>
      )}
      <Tooltip label="Remove from the list (the file stays)">
        <Button size="icon" variant="tertiary" aria-label="Remove from the list" onClick={() => void run(() => api.RemoveFromQueue(item.id))}>
          <Icon.Cancel className="size-3.5" strokeWidth={1.75} aria-hidden />
        </Button>
      </Tooltip>
    </div>
  );
}

/**
 * barPercent is what the bar shows. Getting ready and finishing up have no
 * honest percentage — the first has no size yet and the second is ffmpeg
 * working through a file — so both sweep rather than sit at a number.
 */
/**
 * waitingLine is a dropped download counting down to its next try. The detail
 * carries which try it is; the countdown is worked out here, from when the
 * backend said it would go again.
 */
function waitingLine(p: core.Progress, now: number): string {
  const seconds = Math.max(0, Math.ceil((p.retryAt - now) / 1000));
  const when = seconds > 0 ? `trying again in ${seconds} s` : "trying again";
  return `${p.detail || "Connection lost"} · ${when}`;
}

/**
 * useNow re-renders once a second while a countdown is showing, and not at
 * all otherwise.
 */
function useNow(ticking: boolean): number {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    if (!ticking) return;
    setNow(Date.now());
    const timer = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(timer);
  }, [ticking]);
  return now;
}

function barPercent(item: core.Item): number {
  if (item.state === "fetching" || item.state === "post-processing") return -1;
  return item.progress?.percent ?? -1;
}

/** transferLine is the moving sentence under the focus card's bar. */
function transferLine(item: core.Item, now: number): string {
  const p = item.progress;
  switch (item.state) {
    case "fetching":
      return "Getting ready…";
    case "downloading": {
      if (p?.stage === "waiting") return waitingLine(p, now);
      const amount =
        p?.total > 0
          ? `${formatBytes(p.downloaded)} of ${formatBytes(p.total)}`
          : p?.downloaded > 0
            ? `${formatBytes(p.downloaded)} so far`
            : "";
      const position = p?.items > 1 ? `part ${p.item} of ${p.items}` : "";
      const parts = [amount, formatSpeed(p?.speed ?? 0), formatEta(p?.eta ?? 0), position].filter(Boolean);
      // ffmpeg cuts a clip without reporting until it is done, so there are no
      // bytes to show; say what it is doing instead of "Starting" throughout.
      if (!parts.length && !isWhole(item.options?.clip)) return "Cutting the clip…";
      return parts.length ? parts.join(" · ") : "Starting…";
    }
    case "post-processing":
      return p?.detail ? `${p.detail}…` : "Finishing up…";
    case "paused":
      return p?.percent >= 0 ? `Paused at ${Math.round(p.percent)}%` : "Paused";
    case "queued":
      return "Waiting to start";
    default:
      return "";
  }
}

/** SITE_CHANGED is core.ErrSiteChanged: a failure a newer yt-dlp fixes. */
const SITE_CHANGED = "site-changed";

/**
 * useUpdateAndRetry is the fix for a site that changed under yt-dlp: update
 * yt-dlp, then retry. When yt-dlp is already the newest there is nothing to
 * retry with, and it says so rather than failing the same way again.
 */
function useUpdateAndRetry() {
  const [updating, setUpdating] = useState(false);
  const [message, setMessage] = useState("");

  async function run(ids: string[]) {
    setUpdating(true);
    setMessage("");
    try {
      const result = await api.UpdateYtDlp();
      if (!result.updated) {
        setMessage(
          `yt-dlp ${result.versionAfter} is already the newest. This needs a fix in yt-dlp itself, which usually comes within days.`,
        );
        return;
      }
      for (const id of ids) await api.Retry(id);
    } catch (e) {
      setMessage(cleanError(e));
    } finally {
      setUpdating(false);
    }
  }

  return { updating, message, run };
}

