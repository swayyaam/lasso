import { useCallback, useEffect, useState } from "react";
import { Button, Details, Icon, MonoBlock, cx } from "@lasso/ui";
import { api, updater } from "../bindings";
import { formatBytes } from "../format";

type Phase = "idle" | "checking" | "installing" | "installed";

/**
 * UpdatePanel updates Lasso itself.
 *
 * Doing it in the app matters more here than convenience: Lasso is not
 * notarised, so a copy downloaded through a browser is quarantined and has to
 * be let past Gatekeeper by hand every single time. An update the app installs
 * is never quarantined, so from the second version onwards there is nothing to
 * get past.
 *
 * The check runs on open and downloads nothing. Installing is always a
 * deliberate press — replacing the application someone is using is not
 * something to do because a timer went off.
 */
export function UpdatePanel() {
  const [currentVersion, setCurrentVersion] = useState("");
  const [update, setUpdate] = useState<updater.Update | null>(null);
  const [phase, setPhase] = useState<Phase>("idle");
  const [error, setError] = useState("");
  const [log, setLog] = useState("");

  // force is false on open, so reopening Settings reuses the cached answer
  // rather than spending the hour's allowance of unauthenticated calls.
  const check = useCallback(async (force: boolean) => {
    setPhase("checking");
    setError("");
    try {
      setUpdate(await api.CheckForUpdate(force));
    } catch (e) {
      setError(clean(e));
    } finally {
      setPhase("idle");
    }
  }, []);

  useEffect(() => {
    void check(false);
    api.AppVersion().then(setCurrentVersion).catch(() => setCurrentVersion(""));
  }, [check]);

  async function install() {
    setPhase("installing");
    setError("");
    try {
      const result = await api.InstallUpdate();
      setLog(result.output ?? "");
      setPhase(result.needsRestart ? "installed" : "idle");
      if (!result.needsRestart) await check(true);
    } catch (e) {
      setError(clean(e));
      setPhase("idle");
    }
  }

  // The app quits as part of this, so there is nothing to do afterwards and
  // nothing to show if it works.
  async function restart() {
    setError("");
    try {
      await api.RestartToFinish();
    } catch (e) {
      setError(clean(e));
    }
  }

  if (phase === "installed") {
    return (
      <div className="flex flex-col gap-xs">
        <p className="flex items-center gap-xs text-body-sm font-medium text-success-strong">
          <Icon.Ok className="size-4" strokeWidth={1.75} aria-hidden />
          Version {update?.version} is installed.
        </p>
        <p className="text-caption text-ink-subtle">
          Lasso needs to restart to start running it.
        </p>
        <div className="flex items-center gap-xs">
          <Button
            variant="primary"
            onClick={() => void restart()}
            icon={<Icon.Recheck className="size-3.5" strokeWidth={1.75} aria-hidden />}
          >
            Restart now
          </Button>
          {log && (
            <Details summary="What it did">
              <MonoBlock text={log} maxHeight="10rem" copyable />
            </Details>
          )}
        </div>
        {error && <p className="text-caption text-danger-strong">{error}</p>}
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-xs">
      <div className="flex items-center gap-sm">
        <Status phase={phase} update={update} currentVersion={currentVersion} error={error} />
        <div className="flex-1" />

        {update?.available ? (
          <Button
            variant="primary"
            busy={phase === "installing"}
            onClick={() => void install()}
            icon={<Icon.Download className="size-3.5" strokeWidth={1.75} aria-hidden />}
          >
            {phase === "installing" ? "Installing…" : `Update to ${update.version}`}
          </Button>
        ) : (
          <Button
            busy={phase === "checking"}
            onClick={() => void check(true)}
            icon={<Icon.Recheck className="size-3.5" strokeWidth={1.75} aria-hidden />}
          >
            Check again
          </Button>
        )}
      </div>

      {update?.available && update.notes && (
        <Details summary={`What's new in ${update.version}`}>
          <MonoBlock text={update.notes} maxHeight="12rem" />
        </Details>
      )}
    </div>
  );
}

function Status({
  phase,
  update,
  currentVersion,
  error,
}: {
  phase: Phase;
  update: updater.Update | null;
  currentVersion: string;
  error: string;
}) {
  if (error) {
    return <p className="max-w-note text-caption text-danger-strong">{error}</p>;
  }
  if (phase === "checking" && !update) {
    return <p className="text-caption text-ink-subtle">Checking for updates…</p>;
  }
  if (phase === "installing") {
    return (
      <p className="text-caption text-ink-subtle">
        Downloading {formatBytes(update?.bytes ?? 0) || "the update"} and replacing Lasso…
      </p>
    );
  }
  if (update?.available) {
    return (
      <p className={cx("text-caption text-ink-muted")}>
        Version {update.version} is available
        {update.bytes > 0 && ` — ${formatBytes(update.bytes)} to download`}.
      </p>
    );
  }
  return (
    <p className="text-caption text-ink-tertiary">
      {currentVersion ? `Lasso ${currentVersion} is up to date.` : "Lasso is up to date."}
    </p>
  );
}

function clean(e: unknown): string {
  return String(e).replace(/^Error:\s*/, "").trim() || "Something went wrong.";
}
