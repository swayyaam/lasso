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

/**
 * The mark's segments, clockwise from the one centred on the top: the five
 * category accents, in the order scripts/icon/ring.go draws the app icon.
 * Written out whole so Tailwind can see every class.
 */
const MARK = [
  { stroke: "stroke-accent-purple", fill: "fill-accent-purple" },
  { stroke: "stroke-accent-pink", fill: "fill-accent-pink" },
  { stroke: "stroke-accent-blue", fill: "fill-accent-blue" },
  { stroke: "stroke-accent-orange", fill: "fill-accent-orange" },
  { stroke: "stroke-accent-green", fill: "fill-accent-green" },
];

// On an 18px ring the icon's band is 2.5px, which reads as a hairline beside
// 14px type; 3px holds its own, as the icon's small sizes do.
const MARK_SIZE = 18;
const MARK_BAND = 3;

function onRing(r: number, degrees: number) {
  const a = (degrees * Math.PI) / 180;
  const c = MARK_SIZE / 2;
  return [c + r * Math.sin(a), c - r * Math.cos(a)] as const;
}

/**
 * Mark is the app icon's ring without its tile. Each segment is a butted arc
 * plus a disc at its clockwise end, laid over the next segment's start: the
 * rounded ends that lap round the ring.
 */
function Mark() {
  const r = (MARK_SIZE - MARK_BAND) / 2;
  const step = 360 / MARK.length;
  const start = -step / 2;
  return (
    <svg width={MARK_SIZE} height={MARK_SIZE} viewBox={`0 0 ${MARK_SIZE} ${MARK_SIZE}`} fill="none" aria-hidden>
      {MARK.map(({ stroke }, i) => {
        const [x0, y0] = onRing(r, start + i * step);
        const [x1, y1] = onRing(r, start + (i + 1) * step);
        return (
          <path key={stroke} d={`M${x0} ${y0}A${r} ${r} 0 0 1 ${x1} ${y1}`} strokeWidth={MARK_BAND} className={stroke} />
        );
      })}
      {MARK.map(({ fill }, i) => {
        const [x, y] = onRing(r, start + (i + 1) * step);
        return <circle key={fill} cx={x} cy={y} r={MARK_BAND / 2} className={fill} />;
      })}
    </svg>
  );
}

/** Wordmark is the mark and the name. */
function Wordmark() {
  return (
    <div className="flex items-center gap-xs">
      <Mark />
      <span className="text-body-sm font-semibold text-ink">Lasso</span>
    </div>
  );
}
