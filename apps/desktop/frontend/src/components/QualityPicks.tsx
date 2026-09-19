import { Chip, Eyebrow, cx } from "@lasso/ui";
import { core } from "../bindings";

export const AUDIO_PICKS = [
  { id: "audio-m4a", label: "M4A" },
  { id: "audio-mp3", label: "MP3" },
  { id: "audio-opus", label: "Opus" },
  { id: "audio-flac", label: "FLAC" },
] as const;

/** pickFor maps a tier height onto the quick-pick identifier. */
function pickFor(height: number): string {
  return `${height}p`;
}

/**
 * QualityPicks offers only what the source actually has.
 *
 * The tiers come from the backend, which derives them from the real format
 * list: a video with no 4K encode has no 4K chip, so the interface cannot
 * promise something the download would silently fail to deliver. A flat
 * playlist has no per-item formats, so it gets the standard ladder and says
 * "up to" instead.
 *
 * The same honesty cuts the other way: when the site has withheld everything
 * above its fallback stream, showing a lone 360p chip with no explanation reads
 * as "this video is only 360p". The note says otherwise.
 */
export function QualityPicks({
  quality,
  value,
  onChange,
  disabled,
}: {
  quality?: core.QualityOptions;
  value: string;
  onChange: (pick: string) => void;
  disabled?: boolean;
}) {
  // Before a link is resolved there is nothing to be accurate about, so the
  // standard ladder stands in.
  const tiers = quality?.tiers ?? [];
  const showVideo = quality ? quality.hasVideo : true;
  const showAudio = quality ? quality.hasAudio : true;
  const approximate = quality?.approximate ?? false;

  if (!showVideo && !showAudio) return null;

  return (
    <div className="flex flex-col gap-sm">
      {showVideo && (
        <div className="flex flex-col gap-xs">
          <div className="flex items-baseline gap-xs">
            <Eyebrow>Quality</Eyebrow>
            {approximate && <span className="text-caption text-ink-tertiary">up to, per item</span>}
          </div>

          {quality?.limited && (
            <p className="max-w-note text-caption text-ink-tertiary">
              Only the site&rsquo;s fallback stream was offered. Higher qualities are
              probably being withheld rather than missing — sites hold them back until
              you are signed in. Turn on cookies from your browser in Settings.
            </p>
          )}

          <div className="flex flex-wrap gap-xxs">
            <Chip selected={value === "best"} disabled={disabled} onClick={() => onChange("best")}>
              {quality?.bestLabel ? `Best · ${quality.bestLabel}` : "Best"}
            </Chip>

            {tiers.map((tier) => (
              <Chip
                key={tier.height}
                selected={value === pickFor(tier.height)}
                disabled={disabled}
                onClick={() => onChange(pickFor(tier.height))}
                title={tier.detail ? `${tier.label} (${tier.detail})` : tier.label}
              >
                <span className="flex items-baseline gap-xxs">
                  {tier.label}
                  {tier.detail && <span className="text-caption opacity-60">{tier.detail}</span>}
                  {(tier.hasHighFrameRate || tier.hasHDR) && (
                    <span className="flex gap-0.5">
                      {tier.hasHighFrameRate && <Badge>60</Badge>}
                      {tier.hasHDR && <Badge>HDR</Badge>}
                    </span>
                  )}
                </span>
              </Chip>
            ))}
          </div>
        </div>
      )}

      {showAudio && (
        <div className="flex flex-col gap-xs">
          <Eyebrow>Audio only</Eyebrow>
          <div className="flex flex-wrap gap-xxs">
            {AUDIO_PICKS.map((pick) => (
              <Chip
                key={pick.id}
                selected={value === pick.id}
                disabled={disabled}
                onClick={() => onChange(pick.id)}
              >
                {pick.label}
              </Chip>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}

/** Badge marks a capability that only some tiers have. */
function Badge({ children }: { children: React.ReactNode }) {
  return (
    <span
      className={cx(
        "rounded-xs bg-surface-4 px-1 text-[10px] font-medium leading-4 text-ink-subtle",
      )}
    >
      {children}
    </span>
  );
}
