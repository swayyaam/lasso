import { Button } from "@lasso/ui";

/**
 * TitleBar is the app's own header, standing in for the system title bar.
 *
 * The window is frameless with inset traffic lights, so this strip doubles as
 * the drag region — a stock macOS title bar would show as a grey band against
 * the canvas. The left padding clears the traffic lights.
 */
export function TitleBar({
  onOpenSettings,
  ytDlpVersion,
}: {
  onOpenSettings: () => void;
  ytDlpVersion?: string;
}) {
  return (
    <header className="drag-region flex h-14 shrink-0 items-center gap-sm border-b border-hairline pl-20 pr-md">
      <Wordmark />

      <div className="flex-1" />

      {ytDlpVersion && (
        <span className="text-caption text-ink-tertiary tabular-nums">yt-dlp {ytDlpVersion}</span>
      )}
      <Button variant="tertiary" size="sm" onClick={onOpenSettings}>
        Settings
      </Button>
    </header>
  );
}

/**
 * Wordmark is one of the few places design.md permits the lavender accent.
 * The glyph is a lasso loop drawn from a circle and a trailing stroke.
 */
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
      <span className="text-body-sm font-medium tracking-tight text-ink">Lasso</span>
    </div>
  );
}
