import { Chip, Dot, Eyebrow, Tag } from "@lasso/ui";
import type { TagTone } from "@lasso/ui";
import { core } from "../bindings";
import { formatBytes } from "../format";

/**
 * The audio formats, each with its place in the five-stop category palette.
 *
 * The dot says the one thing four letters cannot: which of these throws data
 * away and which does not. FLAC is the only lossless target, so it is the only
 * green one — and when the source is itself lossy, the note below says so
 * rather than letting the colour imply a quality the file cannot have.
 */
export const AUDIO_PICKS: { id: string; label: string; tone: TagTone; hint: string }[] = [
  {
    id: "audio-original",
    label: "Original",
    tone: "neutral",
    hint: "The site's own audio stream, untouched. The best any site can give you — every other option re-encodes it.",
  },
  { id: "audio-m4a", label: "M4A", tone: "blue", hint: "AAC in an MP4 container — plays everywhere" },
  { id: "audio-mp3", label: "MP3", tone: "orange", hint: "The most compatible, and the oldest" },
  { id: "audio-opus", label: "Opus", tone: "purple", hint: "Best sound per byte; what YouTube usually serves" },
  { id: "audio-flac", label: "FLAC", tone: "green", hint: "Lossless container — only as good as its source" },
];

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
 *
 * Each rung carries what it will cost and what it can do — a size, and a tag
 * for HDR, high frame rate or the top of the ladder. Those are the facts that
 * decide between two rungs, and reading them off the chips beats opening a
 * format table.
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

  // Only worth saying when it changes what the user should expect, which is
  // when they have actually asked for the lossless one.
  const losslessFromLossy = value === "audio-flac" && quality != null && !quality.losslessAudio;

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
              <span className="flex items-baseline gap-xxs">
                {quality?.bestLabel ? `Best · ${quality.bestLabel}` : "Best"}
                <Size bytes={quality?.bestBytes ?? 0} selected={value === "best"} />
              </span>
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
                  <Size bytes={tier.bytes} selected={value === pickFor(tier.height)} />
                  <Capabilities tier={tier} />
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
                title={pick.hint}
                onClick={() => onChange(pick.id)}
              >
                <span className="flex items-center gap-xxs">
                  <Dot tone={pick.tone} />
                  {pick.label}
                  <Size bytes={quality?.audioBytes ?? 0} selected={value === pick.id} />
                </span>
              </Chip>
            ))}
          </div>

          {losslessFromLossy && (
            <p className="max-w-note text-caption text-ink-tertiary">
              This source only serves compressed audio, so the FLAC file will be a
              larger copy of the same sound rather than a better one.{" "}
              <strong className="font-medium text-ink-muted">Original</strong> keeps
              exactly what the site sent, without re-encoding it.
            </p>
          )}
        </div>
      )}
    </div>
  );
}

/**
 * Size is what the rung will cost.
 *
 * Nothing is rendered when the source did not say, which is normal for
 * fragmented and live streams — a guess here would be read as a fact.
 */
function Size({ bytes, selected }: { bytes: number; selected: boolean }) {
  const text = formatBytes(bytes);
  if (!text) return null;

  return (
    <span className={selected ? "text-caption opacity-70" : "text-caption text-ink-tertiary"}>
      {text}
    </span>
  );
}

/** Capabilities are the tags that separate two rungs of the same height. */
function Capabilities({ tier }: { tier: core.ResolutionTier }) {
  const tags = [];

  if (tier.height >= 4320) {
    tags.push(
      <Tag key="max" tone="pink" title="The top of the ladder">
        8K
      </Tag>,
    );
  }
  if (tier.hasHDR) {
    tags.push(
      <Tag key="hdr" tone="purple" title={`High dynamic range (${tier.hdrFormat || "HDR"})`}>
        {tier.hdrFormat || "HDR"}
      </Tag>,
    );
  }
  if (tier.hasHighFrameRate) {
    tags.push(
      <Tag key="fps" tone="blue" title="Higher frame rate — 60fps or above">
        60
      </Tag>,
    );
  }

  if (tags.length === 0) return null;
  return <span className="flex gap-0.5">{tags}</span>;
}
