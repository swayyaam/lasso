import { useEffect, useState } from "react";
import { Banner, Button, Card, Chip, Details, Dropdown, Eyebrow, Icon, Input, LoadingState, MonoBlock, cx } from "@lasso/ui";
import { api, binaries, main, presets as presetModels } from "../bindings";
import { cleanError } from "../hooks/useLink";
import { BROWSERS } from "./AdvancedDrawer";
import { useDoctor } from "./DoctorPanel";
import { UpdatePanel } from "./UpdatePanel";

type Tab = "preferences" | "about";

/** In the order System Settings lists them. Auto is the empty setting. */
const APPEARANCES = [
  { value: "light", label: "Light" },
  { value: "dark", label: "Dark" },
  { value: "", label: "Auto" },
];

/**
 * SettingsSheet covers the canvas rather than opening a second window.
 *
 * Two tabs, because they are two different visits: Preferences is how Lasso
 * should behave, and About & diagnostics is what it is running and whether
 * that is healthy. Helper versions used to sit among the preferences, where
 * they were the first thing read and the last thing needed.
 *
 * Every row is a label and its explanation on the left, the control on the
 * right, at a fixed panel width. Controls are given explicit widths so that a
 * long helper sentence can never squeeze one into a sliver.
 */
export function SettingsSheet({
  settings,
  status,
  presets,
  onPresetsChanged,
  onSave,
  onClose,
  saveError,
}: {
  settings: main.Settings | null;
  status: binaries.Status | null;
  presets: presetModels.Preset[] | null;
  onPresetsChanged: () => void;
  onSave: (next: main.Settings) => Promise<boolean>;
  onClose: () => void;
  saveError: string;
}) {
  const [tab, setTab] = useState<Tab>("preferences");
  const [draft, setDraft] = useState<main.Settings | null>(settings);
  const [updating, setUpdating] = useState(false);
  const [updateResult, setUpdateResult] = useState<binaries.UpdateResult | null>(null);
  const [updateError, setUpdateError] = useState("");
  const [confirmDiscard, setConfirmDiscard] = useState(false);
  const openDoctor = useDoctor();

  // Shallow and key-driven rather than field-by-field, so a new setting is
  // covered the day it is added rather than the day someone remembers this.
  const dirty =
    draft != null &&
    settings != null &&
    (Object.keys({ ...settings, ...draft }) as (keyof main.Settings)[]).some(
      (k) => draft[k] !== settings[k],
    );

  useEffect(() => setDraft(settings), [settings]);

  // No dependency array on purpose: the handler closes over `dirty`, and one
  // registered once would go on believing whatever `dirty` was when the sheet
  // opened — which is false, so Escape would resume discarding silently.
  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.key === "Escape") requestClose();
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  });

  /**
   * requestClose handles the two ways out that do not commit: Escape and a
   * click on the backdrop. Neither is a deliberate "apply", so an unsaved
   * draft asks rather than evaporating.
   */
  function requestClose() {
    if (dirty) {
      setConfirmDiscard(true);
      return;
    }
    onClose();
  }

  /**
   * done is the commit. The button says Done, so it applies what is on screen
   * — a sheet whose confirming button silently discarded the edits above it
   * is the bug this replaces. A refused save keeps the sheet open, with the
   * reason under the rows it came from.
   */
  async function done() {
    if (draft && dirty && !(await onSave(draft))) {
      // Let the refusal be the only thing asking for attention.
      setConfirmDiscard(false);
      return;
    }
    onClose();
  }

  async function chooseFolder() {
    const folder = await api.ChooseFolder();
    if (folder && draft) setDraft({ ...draft, downloadFolder: folder });
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
      onClick={(e) => e.target === e.currentTarget && requestClose()}
    >
      <div className="h-fit w-full max-w-sheet">
        <Card className="flex flex-col">
          <div className="flex items-center justify-between pb-sm">
            <h2 className="text-heading text-ink">Settings</h2>
            <Button
              variant={dirty ? "primary" : "tertiary"}
              size="sm"
              onClick={() => void done()}
            >
              Done
            </Button>
          </div>

          <div className="mb-sm flex gap-md border-b border-hairline" role="tablist">
            <TabButton selected={tab === "preferences"} onClick={() => setTab("preferences")}>
              Preferences
            </TabButton>
            <TabButton selected={tab === "about"} onClick={() => setTab("about")}>
              About &amp; diagnostics
            </TabButton>
          </div>

          {confirmDiscard && (
            <div className="pb-sm">
              <Banner title="You have unsaved changes" tone="neutral">
                <div className="flex items-center gap-xs pt-xs">
                  <Button size="sm" variant="primary" onClick={() => void done()}>
                    Save and close
                  </Button>
                  <Button size="sm" onClick={onClose}>
                    Discard
                  </Button>
                  <Button size="sm" variant="tertiary" onClick={() => setConfirmDiscard(false)}>
                    Keep editing
                  </Button>
                </div>
              </Banner>
            </div>
          )}

          {tab === "preferences" && (!draft ? (
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
                label="Appearance"
                hint="Auto follows your Mac, and changes when it does."
                control={
                  <div className="flex gap-xxs">
                    {APPEARANCES.map(({ value, label }) => (
                      <Chip
                        key={label}
                        selected={(draft.appearance ?? "") === value}
                        onClick={() => setDraft({ ...draft, appearance: value })}
                      >
                        {label}
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

              <SavedChoices presets={presets} onChanged={onPresetsChanged} />
            </>
          ))}

          {tab === "about" && (
          <>
          <div className="flex flex-col gap-sm pt-xs">
            <Eyebrow>Lasso</Eyebrow>
            <UpdatePanel />
          </div>

          <div className="mt-md flex flex-col gap-sm border-t border-hairline pt-md">
            <Eyebrow>Diagnostics</Eyebrow>

            <Row
              label="Check Lasso's setup"
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

            <Details summary="Helper program versions">
              <div className="flex flex-col gap-xxs">
                {Object.entries(status?.versions ?? {}).map(([name, version]) => (
                  <div key={name} className="flex items-center justify-between gap-sm">
                    <span className="text-body-sm whitespace-nowrap text-ink-muted">{name}</span>
                    <span className="truncate text-caption text-ink-tertiary tabular-nums" title={String(version)}>
                      {shortVersion(String(version))}
                    </span>
                  </div>
                ))}
              </div>
            </Details>

            <nav className="flex flex-wrap items-center gap-x-sm gap-y-xxs pt-xs text-body-sm" aria-label="Documents">
              {/* By name, never by address: the backend owns the URLs. */}
              {(
                [
                  ["privacy", "Privacy policy"],
                  ["terms", "Terms of use"],
                  ["licence", "Licence (GPL-3.0)"],
                  ["notices", "Third-party notices"],
                ] as const
              ).map(([name, label]) => (
                <button
                  key={name}
                  type="button"
                  onClick={() => void api.OpenDocument(name)}
                  className="no-drag rounded-sm font-medium text-info-strong hover:text-ink"
                >
                  {label}
                </button>
              ))}
            </nav>
          </div>
          </>
          )}
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

/** TabButton is a text tab with the primary underline, like the old pane tabs. */
function TabButton({
  selected,
  onClick,
  children,
}: {
  selected: boolean;
  onClick: () => void;
  children: React.ReactNode;
}) {
  return (
    <button
      type="button"
      role="tab"
      aria-selected={selected}
      onClick={onClick}
      className={cx(
        "no-drag -mb-px h-9 border-b-2 text-body-sm font-medium",
        "transition-[color,border-color] duration-150 ease-standard",
        selected ? "border-primary text-ink" : "border-transparent text-ink-subtle hover:text-ink",
      )}
    >
      {children}
    </button>
  );
}

/**
 * SavedChoices is where saved choices are renamed and deleted. They are made
 * on the Choose screen, with "Save these…", where the choices are; managing
 * them is occasional enough to live here. Changes apply at once rather than
 * waiting for Done — there is nothing to review about a deletion.
 */
function SavedChoices({
  presets,
  onChanged,
}: {
  presets: presetModels.Preset[] | null;
  onChanged: () => void;
}) {
  const own = (presets ?? []).filter((p) => !p.builtIn);
  const [renaming, setRenaming] = useState<string | null>(null);
  const [name, setName] = useState("");
  const [error, setError] = useState("");

  async function act(action: () => Promise<unknown>) {
    setError("");
    try {
      await action();
      onChanged();
    } catch (e) {
      setError(cleanError(e));
    }
  }

  return (
    <div className="flex flex-col gap-xs pt-sm">
      <div className="flex flex-col gap-0.5">
        <span className="text-body-sm text-ink">Saved choices</span>
        <span className="text-caption text-ink-tertiary">
          {own.length === 0
            ? "None of your own yet. On the Choose screen, “Save these…” keeps the current choices under a name."
            : "Yours, as they appear under Saved choices. The built-in ones cannot be changed."}
        </span>
      </div>

      {own.map((preset) =>
        renaming === preset.id ? (
          <div key={preset.id} className="flex items-center gap-xs">
            <Input
              autoFocus
              value={name}
              aria-label={`New name for ${preset.name}`}
              className="flex-1"
              onChange={(e) => setName(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter" && name.trim()) void act(() => api.RenamePreset(preset.id, name.trim())).then(() => setRenaming(null));
                if (e.key === "Escape") {
                  e.stopPropagation();
                  setRenaming(null);
                }
              }}
            />
            <Button
              size="sm"
              variant="primary"
              disabled={!name.trim()}
              onClick={() => void act(() => api.RenamePreset(preset.id, name.trim())).then(() => setRenaming(null))}
            >
              Rename
            </Button>
            <Button size="sm" variant="tertiary" onClick={() => setRenaming(null)}>
              Cancel
            </Button>
          </div>
        ) : (
          <div key={preset.id} className="flex items-center gap-xs">
            <span className="min-w-0 flex-1 truncate text-body-sm text-ink-muted">{preset.name}</span>
            <Button
              size="sm"
              variant="tertiary"
              onClick={() => {
                setName(preset.name);
                setRenaming(preset.id);
              }}
            >
              Rename
            </Button>
            <Button size="sm" variant="tertiary" onClick={() => void act(() => api.DeletePreset(preset.id))}>
              Delete
            </Button>
          </div>
        ),
      )}

      {error && <p className="text-caption text-danger-strong">{error}</p>}
    </div>
  );
}
