import { useState } from "react";
import { Card, Checkbox, Disclosure, Dropdown, Field, Input } from "@lasso/ui";
import type { DropdownOption } from "@lasso/ui";
import { core } from "../bindings";

/**
 * The option tables live here rather than inline so the drawer reads as the
 * shape of the form instead of a wall of <option> tags.
 *
 * Browsers carry a hint because the choice has a consequence the label cannot
 * show: macOS keeps Safari's cookies inside a protected container, so Lasso
 * cannot read them without Full Disk Access. Chrome and Firefox need nothing.
 */
const CONTAINERS: DropdownOption[] = [
  { value: "", label: "Automatic" },
  { value: "mp4", label: "MP4" },
  { value: "mkv", label: "MKV" },
  { value: "webm", label: "WebM" },
];

const VIDEO_CODECS: DropdownOption[] = [
  { value: "", label: "Any" },
  { value: "h264", label: "H.264" },
  { value: "h265", label: "H.265" },
  { value: "vp9", label: "VP9" },
  { value: "av01", label: "AV1" },
];

const AUDIO_CODECS: DropdownOption[] = [
  { value: "", label: "Any" },
  { value: "aac", label: "AAC" },
  { value: "opus", label: "Opus" },
  { value: "mp3", label: "MP3" },
];

const SPONSORBLOCK: DropdownOption[] = [
  { value: "", label: "Off" },
  { value: "remove", label: "Remove segments" },
  { value: "mark", label: "Mark as chapters" },
];

export const BROWSERS: DropdownOption[] = [
  { value: "", label: "None" },
  { value: "chrome", label: "Chrome" },
  { value: "firefox", label: "Firefox" },
  { value: "brave", label: "Brave" },
  { value: "arc", label: "Arc" },
  { value: "safari", label: "Safari", hint: "Needs Full Disk Access" },
];

type Section = "format" | "subtitles" | "enhancements" | "music" | "playlist" | "network" | "output";

/**
 * AdvancedDrawer is the progressive-disclosure half of the app: everything
 * yt-dlp can do is reachable here, but nothing is in the way until asked for.
 *
 * Only one group is open at a time. The drawer is dense enough that two open
 * groups push the download button off screen, and the collapsed summary means
 * a closed group still says what it is set to.
 */
export function AdvancedDrawer({
  options,
  onChange,
  onRequestClose,
}: {
  options: core.Options;
  onChange: (next: core.Options) => void;
  onRequestClose: () => void;
}) {
  const [open, setOpen] = useState<Section | null>("format");
  const toggle = (section: Section) => setOpen((current) => (current === section ? null : section));

  const patch = (partial: Partial<core.Options>) => onChange({ ...options, ...partial } as core.Options);
  const audioOnly = options.pick?.startsWith("audio-") ?? false;

  return (
    <Card
      padded={false}
      className="px-md"
      onKeyDown={(e) => {
        if (e.key === "Escape") {
          e.stopPropagation();
          onRequestClose();
        }
      }}
    >
      <Disclosure
        title="Format"
        open={open === "format"}
        onToggle={() => toggle("format")}
        summary={summarise([
          options.container || "auto container",
          options.videoCodec,
          options.audioCodec,
          options.preferHDR ? "HDR" : "",
        ])}
      >
        <Field label="Container" hint={audioOnly ? "Audio-only downloads use the audio format instead." : undefined}>
          <Dropdown
            ariaLabel="Container"
            value={options.container ?? ""}
            disabled={audioOnly}
            options={CONTAINERS}
            onChange={(value) => patch({ container: value } as Partial<core.Options>)}
          />
        </Field>

        <div className="grid grid-cols-2 gap-sm">
          <Field label="Video codec">
            <Dropdown
              ariaLabel="Video codec"
              value={options.videoCodec ?? ""}
              disabled={audioOnly}
              options={VIDEO_CODECS}
              onChange={(value) => patch({ videoCodec: value } as Partial<core.Options>)}
            />
          </Field>
          <Field label="Audio codec">
            <Dropdown
              ariaLabel="Audio codec"
              value={options.audioCodec ?? ""}
              options={AUDIO_CODECS}
              onChange={(value) => patch({ audioCodec: value } as Partial<core.Options>)}
            />
          </Field>
        </div>

        <Checkbox
          label="Prefer HDR"
          hint="Takes the high-dynamic-range encode where the video has one. A preference, not a filter — an SDR-only video still downloads."
          disabled={audioOnly}
          checked={options.preferHDR ?? false}
          onChange={(e) => patch({ preferHDR: e.target.checked } as Partial<core.Options>)}
        />
      </Disclosure>

      <Disclosure
        title="Subtitles"
        open={open === "subtitles"}
        onToggle={() => toggle("subtitles")}
        summary={options.subtitles?.download || options.subtitles?.embed ? "On" : "Off"}
      >
        <Checkbox
          label="Download subtitles"
          checked={options.subtitles?.download ?? false}
          onChange={(e) => patch({ subtitles: { ...options.subtitles, download: e.target.checked } } as Partial<core.Options>)}
        />
        <Checkbox
          label="Embed into the file"
          hint="Keeps subtitles with the video instead of a separate file."
          checked={options.subtitles?.embed ?? false}
          onChange={(e) => patch({ subtitles: { ...options.subtitles, embed: e.target.checked } } as Partial<core.Options>)}
        />
        <Checkbox
          label="Include auto-generated"
          hint="Machine transcription, when no written subtitles exist."
          checked={options.subtitles?.autoGenerated ?? false}
          onChange={(e) => patch({ subtitles: { ...options.subtitles, autoGenerated: e.target.checked } } as Partial<core.Options>)}
        />
        <Field label="Languages" hint="Comma separated, for example en, es. Leave empty for the default.">
          <Input
            value={(options.subtitles?.languages ?? []).join(", ")}
            placeholder="en"
            onChange={(e) =>
              patch({
                subtitles: {
                  ...options.subtitles,
                  languages: e.target.value.split(",").map((s) => s.trim()).filter(Boolean),
                },
              } as Partial<core.Options>)
            }
          />
        </Field>
      </Disclosure>

      <Disclosure
        title="Enhancements"
        open={open === "enhancements"}
        onToggle={() => toggle("enhancements")}
        summary={summarise([
          options.enhancements?.sponsorBlock ? `SponsorBlock: ${options.enhancements.sponsorBlock}` : "",
          options.enhancements?.embedChapters ? "chapters" : "",
          options.enhancements?.embedThumbnail ? "thumbnail" : "",
          options.enhancements?.embedMetadata ? "metadata" : "",
        ])}
      >
        <Field label="SponsorBlock" hint="Uses the community database of sponsored segments.">
          <Dropdown
            ariaLabel="SponsorBlock"
            value={options.enhancements?.sponsorBlock ?? ""}
            options={SPONSORBLOCK}
            onChange={(value) =>
              patch({ enhancements: { ...options.enhancements, sponsorBlock: value } } as Partial<core.Options>)
            }
          />
        </Field>
        <Checkbox
          label="Embed chapters"
          checked={options.enhancements?.embedChapters ?? false}
          onChange={(e) =>
            patch({ enhancements: { ...options.enhancements, embedChapters: e.target.checked } } as Partial<core.Options>)
          }
        />
        <Checkbox
          label="Embed thumbnail"
          checked={options.enhancements?.embedThumbnail ?? false}
          onChange={(e) =>
            patch({ enhancements: { ...options.enhancements, embedThumbnail: e.target.checked } } as Partial<core.Options>)
          }
        />
        <Checkbox
          label="Embed metadata"
          hint="Title, uploader and date, written into the file."
          checked={options.enhancements?.embedMetadata ?? false}
          onChange={(e) =>
            patch({ enhancements: { ...options.enhancements, embedMetadata: e.target.checked } } as Partial<core.Options>)
          }
        />
      </Disclosure>

      <Disclosure
        title="Music"
        open={open === "music"}
        onToggle={() => toggle("music")}
        summary={summarise([
          options.music?.tags ? "tagging" : "",
          options.music?.splitChapters ? "split into tracks" : "",
        ])}
      >
        <Checkbox
          label="Tag as music"
          hint="Fills in artist, title and year. A video titled “Artist — Song” is split into the two fields; a site that states a real artist is left alone."
          checked={options.music?.tags ?? false}
          onChange={(e) => patch({ music: { ...options.music, tags: e.target.checked } } as Partial<core.Options>)}
        />
        <Checkbox
          label="Split chapters into tracks"
          hint="Turns one long “full album” upload into a file per chapter, each numbered and titled. The whole recording is kept alongside them."
          checked={options.music?.splitChapters ?? false}
          onChange={(e) =>
            patch({ music: { ...options.music, splitChapters: e.target.checked } } as Partial<core.Options>)
          }
        />
      </Disclosure>

      <Disclosure
        title="Playlist"
        open={open === "playlist"}
        onToggle={() => toggle("playlist")}
        summary={playlistSummary(options.playlist)}
      >
        <div className="grid grid-cols-2 gap-sm">
          <Field label="From item">
            <Input
              type="number"
              min={1}
              value={options.playlist?.start || ""}
              placeholder="1"
              onChange={(e) =>
                patch({ playlist: { ...options.playlist, start: Number(e.target.value) || 0 } } as Partial<core.Options>)
              }
            />
          </Field>
          <Field label="To item">
            <Input
              type="number"
              min={1}
              value={options.playlist?.end || ""}
              placeholder="last"
              onChange={(e) =>
                patch({ playlist: { ...options.playlist, end: Number(e.target.value) || 0 } } as Partial<core.Options>)
              }
            />
          </Field>
        </div>
        <Checkbox
          label="Reverse order"
          checked={options.playlist?.reverse ?? false}
          onChange={(e) =>
            patch({ playlist: { ...options.playlist, reverse: e.target.checked } } as Partial<core.Options>)
          }
        />
      </Disclosure>

      <Disclosure
        title="Network"
        open={open === "network"}
        onToggle={() => toggle("network")}
        summary={summarise([options.network?.rateLimit, options.network?.cookies && `cookies: ${options.network.cookies}`])}
      >
        <Field label="Speed limit" hint="For example 2M or 500K. Empty means no limit.">
          <Input
            value={options.network?.rateLimit ?? ""}
            placeholder="No limit"
            onChange={(e) =>
              patch({ network: { ...options.network, rateLimit: e.target.value } } as Partial<core.Options>)
            }
          />
        </Field>
        <Field label="Cookies from browser" hint="Needed for private, members-only and age-restricted videos.">
          <Dropdown
            ariaLabel="Cookies from browser"
            value={options.network?.cookies ?? ""}
            options={BROWSERS}
            onChange={(value) => patch({ network: { ...options.network, cookies: value } } as Partial<core.Options>)}
          />
        </Field>
      </Disclosure>

      <Disclosure
        title="Output"
        open={open === "output"}
        onToggle={() => toggle("output")}
        summary={options.output?.template || "Default filename"}
      >
        <Field label="Filename template" hint={<TemplatePreview template={options.output?.template ?? ""} />}>
          <Input
            value={options.output?.template ?? ""}
            placeholder="%(title)s [%(id)s].%(ext)s"
            spellCheck={false}
            onChange={(e) => patch({ output: { ...options.output, template: e.target.value } } as Partial<core.Options>)}
          />
        </Field>
      </Disclosure>
    </Card>
  );
}

/** TemplatePreview shows what a filename template will produce. */
function TemplatePreview({ template }: { template: string }) {
  const sample: Record<string, string> = {
    "%(title)s": "Me at the zoo",
    "%(id)s": "jNQXAC9IVRw",
    "%(ext)s": "mp4",
    "%(uploader)s": "jawed",
    "%(upload_date)s": "20050423",
    "%(playlist_index)s": "01",
  };

  const effective = template || "%(title)s [%(id)s].%(ext)s";
  const rendered = Object.entries(sample).reduce(
    (out, [token, value]) => out.replaceAll(token, value),
    effective,
  );

  return (
    <>
      Preview: <span className="font-mono text-ink-subtle">{rendered}</span>
    </>
  );
}

function summarise(parts: Array<string | undefined | false>): string {
  const kept = parts.filter(Boolean) as string[];
  return kept.length > 0 ? kept.join(" · ") : "Default";
}

function playlistSummary(playlist: core.Playlist | undefined): string {
  if (!playlist) return "All items";
  const { start, end, reverse } = playlist;
  const range = start && end ? `items ${start}–${end}` : start ? `from item ${start}` : end ? `up to item ${end}` : "All items";
  return reverse ? `${range}, reversed` : range;
}
