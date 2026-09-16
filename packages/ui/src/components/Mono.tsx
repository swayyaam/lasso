import { useState } from "react";
import { Button } from "./Button";
import { cx } from "../cx";

/**
 * MonoBlock renders a command or a log in the mono style.
 *
 * design.md keeps mono inside product surfaces rather than on chrome, which is
 * exactly what this is: the generated command and yt-dlp's raw output.
 */
export function MonoBlock({
  text,
  className,
  copyable = false,
  maxHeight = "18rem",
}: {
  text: string;
  className?: string;
  copyable?: boolean;
  maxHeight?: string;
}) {
  const [copied, setCopied] = useState(false);

  async function copy() {
    try {
      await navigator.clipboard.writeText(text);
      setCopied(true);
      setTimeout(() => setCopied(false), 1600);
    } catch {
      // Clipboard access can be refused; failing silently is better than an
      // alert for something this incidental.
    }
  }

  return (
    <div className={cx("relative rounded-xl border border-hairline bg-surface-1", className)}>
      <pre
        data-selectable
        className="overflow-auto p-sm pr-16 font-mono text-mono whitespace-pre-wrap break-all text-ink-muted"
        style={{ maxHeight }}
      >
        {text}
      </pre>
      {copyable && (
        <div className="absolute right-2 top-2">
          <Button size="sm" onClick={copy}>
            {copied ? "Copied" : "Copy"}
          </Button>
        </div>
      )}
    </div>
  );
}

/**
 * Details is the raw-log toggle kept on every failure: a plain-language
 * message stays visible, and yt-dlp's own output is one click away.
 */
export function Details({ summary, children }: { summary: string; children: React.ReactNode }) {
  return (
    <details className="group">
      <summary
        className={cx(
          "no-drag inline-flex cursor-pointer list-none items-center gap-xxs",
          "text-caption text-ink-subtle hover:text-ink-muted",
        )}
      >
        <span className="inline-block transition-transform duration-150 group-open:rotate-90">›</span>
        {summary}
      </summary>
      <div className="mt-xs">{children}</div>
    </details>
  );
}
