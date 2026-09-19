import { useEffect, useState } from "react";
import { Banner, Button, Card, Chip, Details, Dropdown, Eyebrow, Icon, LoadingState, MonoBlock, cx } from "@lasso/ui";
import { api, binaries, main } from "../bindings";
import { BROWSERS } from "./AdvancedDrawer";
import { useDoctor } from "./DoctorPanel";
import { UpdatePanel } from "./UpdatePanel";

/**
 * SettingsSheet covers the canvas rather than opening a second window.
 *
 * Every row is a label and its explanation on the left, the control on the
 * right, at a fixed panel width. Controls are given explicit widths so that a
 * long helper sentence can never squeeze one into a sliver.
 */
export function SettingsSheet({
  settings,
  status,
  onSave,
  onClose,
  saveError,
}: {
  settings: main.Settings | null;
  status: binaries.Status | null;
  onSave: (next: main.Settings) => Promise<boolean>;
  onClose: () => void;
  saveError: string;
}) {
  const [draft, setDraft] = useState<main.Settings | null>(settings);
  const [updating, setUpdating] = useState(false);
  const [updateResult, setUpdateResult] = useState<binaries.UpdateResult | null>(null);
  const [updateError, setUpdateError] = useState("");
  const [saved, setSaved] = useState(false);
  const openDoctor = useDoctor();

  useEffect(() => setDraft(settings), [settings]);

  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.key === "Escape") onClose();
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);

  async function chooseFolder() {
    const folder = await api.ChooseFolder();
    if (folder && draft) setDraft({ ...draft, downloadFolder: folder });
  }

  async function save() {
    if (!draft) return;
    if (await onSave(draft)) {
      setSaved(true);
      setTimeout(() => setSaved(false), 1600);
    }
  }

  async function updateYtDlp() {
    setUpdating(true);
    setUpdateError("");
    setUpdateResult(null);
    try {
      setUpdateResult(await api.UpdateYtDlp());
    } catch (e) {
      setUpdateError(String(e).replace(/^Error:\s*/, ""));
    } finally {
      setUpdating(false);
    }
  }

  return (
    <div
      className="absolute inset-0 z-10 flex justify-center overflow-y-auto bg-overlay/70 p-lg backdrop-blur-sm"
      onClick={(e) => e.target === e.currentTarget && onClose()}
    >
      <div className="h-fit w-full max-w-sheet">
        <Card className="flex flex-col">
          <div className="flex items-center justify-between pb-md">
            <h2 className="text-card-title text-ink">Settings</h2>
            <Button variant="tertiary" size="sm" onClick={onClose}>
              Done
            </Button>
          </div>

          {!draft ? (
            <LoadingState label="Loading settings…" />
          ) : (
            <>
              <Row
                label="Download folder"
                hint="Where finished files are saved."
                stacked
                control={
                  <div className="flex w-full items-center gap-xs">
                    <span
                      title={draft.downloadFolder}
                      className="min-w-0 flex-1 rounded-md border border-hairline bg-surface-2 px-xs py-1 text-caption text-ink-muted"
                    >
                      <MiddleTruncate text={draft.downloadFolder} />
                    </span>
                    <Button
                      size="sm"
                      onClick={chooseFolder}
                      icon={<Icon.Folder className="size-3.5" strokeWidth={1.75} aria-hidden />}
                    >
                      Choose…
                    </Button>
                  </div>
                }
              />

              <Row
                label="Downloads at once"
                hint="More is not always faster: sites start rate-limiting, and everything slows down."
                control={
                  <div className="flex gap-xxs">
                    {[1, 2, 3, 4].map((n) => (
                      <Chip
                        key={n}
                        selected={draft.concurrency === n}
                        onClick={() => setDraft({ ...draft, concurrency: n })}
                      >
                        {n}
                      </Chip>
                    ))}
                  </div>
                }
              />

              <Row
                label="Cookies from browser"
                hint={
                  draft.cookies
                    ? "macOS may ask for your keychain password the first time, and it can take a minute."
                    : "Needed for private, members-only and age-restricted videos."
                }
                control={
                  <Dropdown
                    className="w-48"
                    ariaLabel="Cookies from browser"
                    value={draft.cookies ?? ""}
                    options={BROWSERS}
                    onChange={(value) => setDraft({ ...draft, cookies: value })}
                  />
                }
              />

              {saveError && (
                <div className="pt-sm">
                  <Banner title="Could not save" tone="danger">
                    {saveError}
                  </Banner>
                </div>
              )}

              <div className="flex justify-end pt-md">
                <Button variant="primary" onClick={save}>
                  {saved ? "Saved" : "Save"}
                </Button>
              </div>
            </>
          )}

          <div className="mt-md flex flex-col gap-sm border-t border-hairline pt-md">
            <Eyebrow>Lasso</Eyebrow>
            <UpdatePanel />
          </div>

          <div className="mt-md flex flex-col gap-sm border-t border-hairline pt-md">
            <Eyebrow>Helper programs</Eyebrow>

            {Object.entries(status?.versions ?? {}).map(([name, version]) => (
              <div key={name} className="flex items-center justify-between gap-sm">
                <span className="text-body-sm whitespace-nowrap text-ink-muted">{name}</span>
                <span className="truncate text-caption text-ink-tertiary tabular-nums" title={String(version)}>
                  {shortVersion(String(version))}
                </span>
              </div>
            ))}

            <Row
              label="Diagnostics"
              hint="Checks the helper programs, the download folder, free space and cookies — and repairs what it can."
              control={
                <Button
                  onClick={openDoctor}
                  icon={<Icon.Doctor className="size-3.5" strokeWidth={1.75} aria-hidden />}
                >
                  Run the doctor
                </Button>
              }
            />

            <Row
              label="yt-dlp updates"
              hint={
                updating
                  ? "Contacting the yt-dlp release server…"
                  : updateError || updateMessage(updateResult) || "Sites change often; updating usually fixes a broken download."
              }
              hintTone={updateError ? "danger" : "normal"}
              control={
                <Button
                  onClick={updateYtDlp}
                  busy={updating}
                  icon={<Icon.Recheck className="size-3.5" strokeWidth={1.75} aria-hidden />}
                >
                  {updating ? "Checking…" : "Update yt-dlp"}
                </Button>
              }
            />

            {status?.problems?.map((problem, i) => (
              <Banner key={i} title={problem.message} tone="danger">
                {problem.detail && (
                  <Details summary="Details">
                    <MonoBlock text={problem.detail} maxHeight="10rem" />
                  </Details>
                )}
              </Banner>
            ))}

            {updateResult?.output && (
              <Details summary="Update log">
                <MonoBlock text={updateResult.output} maxHeight="10rem" />
              </Details>
            )}
          </div>
        </Card>
      </div>
    </div>
  );
}

/**
 * Row is one setting: label and explanation on the left, control on the right.
 *
 * The label column is given a floor and the control column a ceiling, so a
 * long sentence cannot squeeze the control down to nothing — which is exactly
 * what happened when both sides were free to shrink.
 */
function Row({
  label,
  hint,
  control,
  hintTone = "normal",
  stacked = false,
}: {
  label: string;
  hint?: string;
  control: React.ReactNode;
  hintTone?: "normal" | "danger";
  /** Puts the control on its own full-width line, for anything that needs room. */
  stacked?: boolean;
}) {
  const caption = hint && (
    <span className={cx("text-caption", hintTone === "danger" ? "text-danger-strong" : "text-ink-tertiary")}>
      {hint}
    </span>
  );

  if (stacked) {
    return (
      <div className="flex flex-col gap-xs border-b border-hairline py-sm last:border-b-0">
        <div className="flex flex-col gap-0.5">
          <span className="text-body-sm text-ink">{label}</span>
          {caption}
        </div>
        {control}
      </div>
    );
  }

  return (
    <div className="flex items-start justify-between gap-md border-b border-hairline py-sm last:border-b-0">
      {/* The label column has a floor and the control column a ceiling, so a
          long sentence cannot squeeze a control down to a sliver. */}
      <div className="flex min-w-40 flex-1 flex-col gap-0.5">
        <span className="text-body-sm text-ink">{label}</span>
        {caption}
      </div>
      <div className="flex w-52 shrink-0 justify-end">{control}</div>
    </div>
  );
}

/**
 * MiddleTruncate keeps both ends of a path readable.
 *
 * A download folder is usually distinguished by its last component, which a
 * plain end-truncation would be the first thing to throw away.
 */
function MiddleTruncate({ text, keepEnd = 18 }: { text: string; keepEnd?: number }) {
  if (text.length <= keepEnd + 12) return <span className="block truncate">{text}</span>;

  const end = text.slice(-keepEnd);
  const start = text.slice(0, text.length - keepEnd);

  return (
    <span className="flex min-w-0">
      <span className="min-w-0 truncate">{start}</span>
      <span className="shrink-0">{end}</span>
    </span>
  );
}

function updateMessage(result: binaries.UpdateResult | null): string {
  if (!result) return "";
  if (result.updated) return `Updated to ${result.versionAfter}.`;
  return `Already up to date${result.versionAfter ? ` (${result.versionAfter})` : ""}.`;
}

/**
 * shortVersion pulls the version number out of whatever each program prints.
 *
 * The four helpers disagree completely: ffmpeg says "ffmpeg version 9.0.1-...",
 * deno says "deno 2.9.6 (stable, ...)", and yt-dlp prints the bare number. So
 * this looks for the first token that is actually a version rather than
 * trusting any one program's phrasing.
 */
function shortVersion(version: string): string {
  const match = version.match(/\b\d+\.\d+(?:\.\d+)?\b/);
  if (match) return match[0];
  return version.split(" ")[0];
}
