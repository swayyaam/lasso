import { createContext, useCallback, useContext, useEffect, useState } from "react";
import type { ReactNode } from "react";
import { Button, Card, Details, Icon, MonoBlock, Sheet, cx } from "@lasso/ui";
import { api, doctor } from "../bindings";

/**
 * DoctorPanel is the diagnostics screen.
 *
 * The failures that stop a download are rarely about the link — a quarantined
 * binary, a folder that was renamed, a cookie jar macOS will not open — and
 * they all reach the user as the same thing: a download that failed. This is
 * where they are told apart.
 *
 * Every check answers the same three things in the same order: what was found,
 * what to do about it, and the raw evidence behind a toggle. Where Lasso can
 * do the thing itself, the remedy is a button rather than a paragraph.
 */
export function DoctorPanel({ onClose }: { onClose?: () => void }) {
  const [report, setReport] = useState<doctor.Report | null>(null);
  const [running, setRunning] = useState(false);
  const [error, setError] = useState("");

  const run = useCallback(async () => {
    setRunning(true);
    setError("");
    try {
      setReport(await api.Diagnose());
    } catch (e) {
      setError(clean(e));
    } finally {
      setRunning(false);
    }
  }, []);

  // Runs on open. Diagnosis has no side effects, so there is no reason to make
  // the user press a button to find out what is wrong.
  useEffect(() => {
    void run();
  }, [run]);

  return (
    <div className="flex flex-col gap-sm">
      <div className="flex items-center gap-sm">
        <Verdict report={report} running={running} />
        <div className="flex-1" />
        <Button
          size="sm"
          variant="secondary"
          busy={running}
          onClick={() => void run()}
          icon={<Icon.Recheck className="size-3.5" strokeWidth={1.75} aria-hidden />}
        >
          Re-check
        </Button>
        {onClose && (
          <Button size="sm" variant="tertiary" onClick={onClose}>
            Done
          </Button>
        )}
      </div>

      {error && (
        <Card className="border-danger/30 bg-danger-surface">
          <p className="text-body-sm text-danger-strong">{error}</p>
        </Card>
      )}

      <div className="flex flex-col gap-xs">
        {report?.checks?.map((check) => (
          <CheckRow key={check.id} check={check} onFixed={() => void run()} />
        ))}
      </div>
    </div>
  );
}

/** Verdict is the one-line answer, so the panel is readable without reading it. */
function Verdict({ report, running }: { report: doctor.Report | null; running: boolean }) {
  if (running && !report) {
    return <p className="text-body-sm text-ink-subtle">Checking…</p>;
  }
  if (!report) return <span />;

  // The worst check sets the tone. "Healthy" only means nothing failed, so it
  // said "1 warning worth looking at" beside a green tick.
  const checks = report.checks ?? [];
  const worst = checks.some((c) => c.status === "fail")
    ? "fail"
    : checks.some((c) => c.status === "warn")
      ? "warn"
      : "ok";
  const { text, Glyph } = TONE[worst];
  return (
    <p className={cx("flex items-center gap-xs text-body-sm font-medium", text)}>
      <Glyph className="size-4" strokeWidth={1.75} aria-hidden />
      {report.summary}
    </p>
  );
}

const TONE: Record<string, { text: string; surface: string; Glyph: typeof Icon.Ok }> = {
  ok: { text: "text-success-strong", surface: "border-hairline", Glyph: Icon.Ok },
  warn: { text: "text-warning-strong", surface: "border-hairline", Glyph: Icon.Warn },
  fail: { text: "text-danger-strong", surface: "border-danger/30", Glyph: Icon.Fail },
};

function CheckRow({ check, onFixed }: { check: doctor.Check; onFixed: () => void }) {
  const [fixing, setFixing] = useState(false);
  const [outcome, setOutcome] = useState("");
  const [failure, setFailure] = useState("");

  const tone = TONE[check.status] ?? TONE.ok;

  async function fix() {
    setFixing(true);
    setOutcome("");
    setFailure("");
    try {
      setOutcome((await api.ApplyFix(check.id)) || "Done.");
      // Re-run rather than assume: a fix that did not take should not leave
      // the row claiming it did.
      onFixed();
    } catch (e) {
      setFailure(clean(e));
    } finally {
      setFixing(false);
    }
  }

  return (
    <Card className={cx("flex flex-col gap-xxs", tone.surface)}>
      <div className="flex items-start gap-xs">
        <tone.Glyph className={cx("mt-0.5 size-4 shrink-0", tone.text)} strokeWidth={1.75} aria-hidden />

        <div className="min-w-0 flex-1">
          <p className="text-body-sm font-medium text-ink">{check.title}</p>
          <p className="text-caption text-ink-subtle">{check.summary}</p>
        </div>

        {check.fixable && (
          <Button
            size="sm"
            variant="secondary"
            busy={fixing}
            onClick={() => void fix()}
            icon={<Icon.Fix className="size-3.5" strokeWidth={1.75} aria-hidden />}
          >
            {check.fixLabel || "Fix"}
          </Button>
        )}
      </div>

      {check.remedy && <p className="pl-lg text-caption text-ink-muted">{check.remedy}</p>}

      {outcome && (
        <p className="pl-lg text-caption text-success-strong">{outcome}</p>
      )}
      {failure && <p className="pl-lg text-caption text-danger-strong">{failure}</p>}

      {check.detail && (
        <div className="pl-lg">
          <Details summary="Details">
            <MonoBlock text={check.detail} maxHeight="10rem" copyable />
          </Details>
        </div>
      )}
    </Card>
  );
}

function clean(e: unknown): string {
  return String(e).replace(/^Error:\s*/, "").trim() || "Something went wrong.";
}

/**
 * The doctor is opened from two places that are nowhere near each other in the
 * tree: the Settings sheet, and a failed row inside the virtualised queue. A
 * context carries the opener rather than threading a callback down through the
 * activity pane and the queue to a row.
 */
const DoctorContext = createContext<() => void>(() => {});

/** useDoctor returns a function that opens the diagnostics sheet. */
export function useDoctor() {
  return useContext(DoctorContext);
}

/**
 * useSuggestsDoctor asks the backend whether a failure is worth offering
 * diagnostics for. The rule lives in Go so it has one definition; the answer
 * per kind is remembered, since it cannot change while the app runs.
 */
const suggestions = new Map<string, boolean>();
export function useSuggestsDoctor(kind: string): boolean {
  const [suggests, setSuggests] = useState(() => suggestions.get(kind) ?? false);
  useEffect(() => {
    if (!kind) return;
    if (suggestions.has(kind)) {
      setSuggests(suggestions.get(kind)!);
      return;
    }
    let cancelled = false;
    api.ShouldSuggestDoctor(kind).then((answer) => {
      suggestions.set(kind, answer);
      if (!cancelled) setSuggests(answer);
    });
    return () => {
      cancelled = true;
    };
  }, [kind]);
  return suggests;
}

/**
 * DoctorProvider owns the sheet and hands its opener down.
 *
 * It renders the sheet itself, so a caller only ever has to say "open it".
 */
export function DoctorProvider({ children }: { children: ReactNode }) {
  const [open, setOpen] = useState(false);
  const openDoctor = useCallback(() => setOpen(true), []);

  return (
    <DoctorContext.Provider value={openDoctor}>
      {children}
      <Sheet
        open={open}
        onOpenChange={setOpen}
        title="Diagnostics"
        description="What Lasso can see about its own setup. Most download failures are one of these rather than a problem with the link."
      >
        {/* Mounted only while open, so every opening runs a fresh check
            rather than showing whatever was true last time. */}
        {open && <DoctorPanel />}
      </Sheet>
    </DoctorContext.Provider>
  );
}
