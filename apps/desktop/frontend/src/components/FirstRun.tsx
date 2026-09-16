import { Banner, Button, Card, Details, LoadingState, MonoBlock } from "@lasso/ui";
import { binaries } from "../bindings";

/**
 * FirstRun covers the window while the helper programs are being installed or
 * when they could not be started.
 *
 * The first launch copies about 330 MB out of the bundle and lets macOS scan
 * it, which takes several seconds. Without this the window would sit empty and
 * look broken.
 */
export function FirstRun({ status, onOpenSettings }: { status: binaries.Status | null; onOpenSettings: () => void }) {
  if (!status) {
    return (
      <Centre>
        <LoadingState label="Starting Lasso…" />
      </Centre>
    );
  }

  if (!status.ready) {
    return (
      <Centre>
        <Card className="flex w-full max-w-dialog flex-col gap-sm">
          <h2 className="text-card-title text-ink">Lasso cannot download yet</h2>
          <p className="text-body-sm text-ink-muted">
            The helper programs Lasso needs did not start. Downloading is unavailable until this is fixed.
          </p>

          {status.problems?.map((problem, i) => (
            <Banner key={i} title={problem.message} tone="danger">
              {problem.detail && (
                <Details summary="Details">
                  <MonoBlock text={problem.detail} maxHeight="12rem" copyable />
                </Details>
              )}
            </Banner>
          ))}

          <div className="flex justify-end gap-xs pt-xs">
            <Button onClick={onOpenSettings}>Open Settings</Button>
          </div>
        </Card>
      </Centre>
    );
  }

  return null;
}

/** InstallingOverlay is shown while first-run installation is in progress. */
export function InstallingOverlay() {
  return (
    <Centre>
      <Card className="flex w-full max-w-dialog flex-col items-center gap-xs py-lg text-center">
        <LoadingState label="Setting up Lasso…" className="py-xs" />
        <p className="text-caption text-ink-tertiary">
          Installing the bundled copies of yt-dlp and ffmpeg. This happens once.
        </p>
      </Card>
    </Centre>
  );
}

function Centre({ children }: { children: React.ReactNode }) {
  return <div className="flex flex-1 items-center justify-center p-lg">{children}</div>;
}
