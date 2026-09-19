import { useState } from "react";
import { useBinaryStatus, useHistory, usePresets, useQueue, useSettings } from "./hooks/useBackend";
import { Composer } from "./components/Composer";
import { FirstRun, InstallingOverlay } from "./components/FirstRun";
import { ActivityPane } from "./components/ActivityPane";
import { DoctorProvider } from "./components/DoctorPanel";
import { SettingsSheet } from "./components/SettingsSheet";
import { TitleBar } from "./components/TitleBar";

/**
 * App is the window: a header that doubles as the drag region, then two panes.
 *
 * Composer and queue sit side by side so the next link can be pasted while
 * downloads run. Below roughly 900px they stack, which is the narrowest the
 * window is allowed to get.
 */
export function App() {
  const status = useBinaryStatus();
  const items = useQueue();
  const { entries } = useHistory();
  const { presets, refresh } = usePresets();
  const { settings, save, error: saveError } = useSettings();

  const [settingsOpen, setSettingsOpen] = useState(false);
  const ready = status?.ready ?? false;
  const starting = status === null;

  return (
    <DoctorProvider>
      <div className="relative flex h-full flex-col bg-canvas">
      <TitleBar
        onOpenSettings={() => setSettingsOpen(true)}
        ytDlpVersion={status?.versions?.["yt-dlp"]}
      />

      {starting && <InstallingOverlay />}

      {!starting && !ready && (
        <FirstRun status={status} onOpenSettings={() => setSettingsOpen(true)} />
      )}

      {!starting && ready && (
        <main className="flex min-h-0 flex-1 flex-col lg:flex-row">
          {/* min-w-0 on both panes is load-bearing: without it a long filename
              in a queue row sets the pane's minimum width and the split drifts
              away from 60/40 as soon as something finishes downloading. */}
          <div className="flex min-h-0 min-w-0 basis-3/5 flex-col border-hairline lg:border-r">
            <Composer presets={presets} onPresetsChanged={refresh} disabled={!ready} />
          </div>
          <div className="flex min-h-0 min-w-0 basis-2/5 flex-col border-t border-hairline lg:border-t-0">
            <ActivityPane items={items} entries={entries} />
          </div>
        </main>
      )}

      {settingsOpen && (
        <SettingsSheet
          settings={settings}
          status={status}
          saveError={saveError}
          onSave={save}
          onClose={() => setSettingsOpen(false)}
        />
        )}
      </div>
    </DoctorProvider>
  );
}
