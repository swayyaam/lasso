import { Button, Icon, Spinner } from "@lasso/ui";

/**
 * TitleBar is the app's own header, standing in for the system title bar.
 *
 * The window is frameless with inset traffic lights, so this strip doubles as
 * the drag region — a stock macOS title bar would show as a grey band against
 * the canvas. The left padding clears the traffic lights: the inset buttons
 * end 80px in, and 96px leaves the wordmark the gap it would have beside any
 * other control. At 80px it touched the green button.
 *
 * It carries the two places to go that are not the task at hand, History and
 * Settings, and — only while the downloads are off screen — how many are still
 * running, as the way back to them.
 */
export function TitleBar({
  onOpenSettings,
  onOpenHistory,
  activity,
  busy,
  onShowDownloads,
}: {
  onOpenSettings: () => void;
  onOpenHistory?: () => void;
  /** "2 downloading" or "1 paused", shown while the downloads are off screen. */
  activity: string;
  /** Whether anything is moving, which is what the spinner claims. */
  busy: boolean;
  onShowDownloads: () => void;
}) {
  return (
    <header className="drag-region flex h-14 shrink-0 items-center gap-xs border-b border-hairline pr-md pl-24">
      <Wordmark />

      <div className="flex-1" />

      {activity && (
        <Button
          variant="tertiary"
          size="sm"
          onClick={onShowDownloads}
          icon={busy ? <Spinner /> : <Icon.Pause className="size-3.5" strokeWidth={1.75} aria-hidden />}
        >
          {activity}
        </Button>
      )}
      {onOpenHistory && (
        <Button
          variant="tertiary"
          size="sm"
          onClick={onOpenHistory}
          icon={<Icon.History className="size-3.5" strokeWidth={1.75} aria-hidden />}
        >
          History
        </Button>
      )}
      <Button
        variant="tertiary"
        size="sm"
        onClick={onOpenSettings}
        icon={<Icon.Settings className="size-3.5" strokeWidth={1.75} aria-hidden />}
      >
        Settings
      </Button>
    </header>
  );
}

/** Wordmark is the lasso loop, drawn from an ellipse and a trailing stroke. */
function Wordmark() {
  return (
    <div className="flex items-center gap-xs">
      <svg width="18" height="18" viewBox="0 0 18 18" fill="none" aria-hidden>
        <ellipse cx="9" cy="6.5" rx="5.5" ry="4" stroke="currentColor" strokeWidth="1.6" className="text-primary" />
        <path
          d="M6.2 9.8C5.2 11.4 5.6 14 7.4 15.2"
          stroke="currentColor"
          strokeWidth="1.6"
          strokeLinecap="round"
          className="text-primary"
        />
      </svg>
      <span className="text-body-sm font-semibold text-ink">Lasso</span>
    </div>
  );
}
