import { useEffect, useState } from "react";
import { useBinaryStatus, useHistory, useLatest, usePresets, useQueue, useSettings } from "./hooks/useBackend";
import { forgetResolved, useLink } from "./hooks/useLink";
import { looksLikeURL } from "./format";
import { FirstRun, InstallingOverlay } from "./components/FirstRun";
import { DoctorProvider } from "./components/DoctorPanel";
import { SettingsSheet } from "./components/SettingsSheet";
import { TitleBar } from "./components/TitleBar";
import { ChooseScreen } from "./screens/ChooseScreen";
import { DownloadsScreen } from "./screens/DownloadsScreen";
import { HistoryScreen } from "./screens/HistoryScreen";
import { ReadyScreen } from "./screens/ReadyScreen";

const MOVING = new Set(["queued", "fetching", "downloading", "post-processing"]);

/**
 * App is the window: a header that doubles as the drag region, then one column
 * showing one of three moments.
 *
 *  - Ready: nothing is happening, so the screen is a place to paste.
 *  - Choose: a link is being looked at, and the screen is the decision.
 *  - Downloading: the queue, with a field for the next link.
 *
 * History is a place of its own, reached from the title bar or from Ready's
 * "Show all". Pasting a link anywhere, on any of them, goes to Choose.
 */
export function App() {
  const status = useBinaryStatus();
  const items = useQueue();
  const { entries } = useHistory();
  const { presets, refresh } = usePresets();
  const { settings, save, error: saveError } = useSettings();
  const link = useLink();

  const [settingsOpen, setSettingsOpen] = useState(false);
  const [place, setPlace] = useState<"home" | "history">("home");

  const ready = status?.ready ?? false;
  const starting = status === null;
  const moving = items.filter((i) => MOVING.has(i.state)).length;
  const paused = items.filter((i) => i.state === "paused").length;
  const activity = moving > 0 ? `${moving} downloading` : paused > 0 ? `${paused} paused` : "";

  function open(url: string) {
    setPlace("home");
    void link.resolve(url);
  }

  // Cmd+V anywhere resolves a link, so the common case never requires aiming
  // at a field first. A paste into some other field is left alone.
  const latest = useLatest({ ready, settingsOpen, open });
  useEffect(() => {
    function onPaste(event: ClipboardEvent) {
      const { ready, settingsOpen, open } = latest.current;
      if (!ready || settingsOpen) return;
      const target = event.target as HTMLElement | null;
      if (target?.tagName === "INPUT" || target?.tagName === "TEXTAREA") return;

      const text = event.clipboardData?.getData("text") ?? "";
      if (!looksLikeURL(text)) return;
      event.preventDefault();
      open(text.trim());
    }
    window.addEventListener("paste", onPaste);
    return () => window.removeEventListener("paste", onPaste);
  }, [latest]);

  const home = link.open ? "choose" : items.length > 0 ? "downloads" : "ready";
  // The indicator only earns its place when the downloads are not on screen.
  const offscreen = place === "history" || home === "choose";

  return (
    <DoctorProvider>
      <div className="relative flex h-full flex-col bg-canvas">
        <TitleBar
          onOpenSettings={() => setSettingsOpen(true)}
          onOpenHistory={ready && place !== "history" ? () => setPlace("history") : undefined}
          activity={offscreen ? activity : ""}
          busy={moving > 0}
          onShowDownloads={() => {
            link.clear();
            setPlace("home");
          }}
        />

        {starting && <InstallingOverlay />}

        {!starting && !ready && <FirstRun status={status} onOpenSettings={() => setSettingsOpen(true)} />}

        {!starting && ready && (
          <main className="flex min-h-0 flex-1 flex-col">
            {place === "history" ? (
              <HistoryScreen entries={entries} onBack={() => setPlace("home")} onAgain={() => setPlace("home")} />
            ) : home === "choose" ? (
              <ChooseScreen
                link={link}
                presets={presets}
                onPresetsChanged={refresh}
                disabled={!ready}
                onQueued={link.clear}
              />
            ) : home === "downloads" ? (
              <DownloadsScreen items={items} onSubmit={open} disabled={!ready} />
            ) : (
              <div className="min-h-0 flex-1 overflow-y-auto">
                <ReadyScreen
                  entries={entries}
                  onSubmit={open}
                  onShowHistory={() => setPlace("history")}
                  disabled={!ready}
                />
              </div>
            )}
          </main>
        )}

        {settingsOpen && (
          <SettingsSheet
            settings={settings}
            status={status}
            presets={presets}
            onPresetsChanged={refresh}
            saveError={saveError}
            onSave={async (next) => {
              const ok = await save(next);
              // Cookies change what a site offers, so an answer from before
              // the change is no longer the answer.
              if (ok) forgetResolved();
              return ok;
            }}
            onClose={() => setSettingsOpen(false)}
          />
        )}
      </div>
    </DoctorProvider>
  );
}
