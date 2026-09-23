import { useEffect, useState } from "react";
import { Icon } from "@lasso/ui";
import { looksLikeURL } from "../format";

/**
 * DropTarget makes the whole window a place to drop a link — from a browser's
 * address bar, a link in a page, or a message.
 *
 * Every drop is cancelled whether or not it is used. The webview's own
 * response to a dropped link or file is to navigate to it, which would replace
 * Lasso with that page in its own window.
 */
export function DropTarget({ onDrop }: { onDrop: (url: string) => void }) {
  const [over, setOver] = useState(false);

  useEffect(() => {
    // dragenter and dragleave fire for every element crossed, so a count of
    // the ones entered is what says whether the pointer is still inside.
    let depth = 0;
    const carriesLink = (e: DragEvent) => {
      const types = e.dataTransfer?.types ?? [];
      return types.includes("text/uri-list") || types.includes("text/plain");
    };

    function enter(e: DragEvent) {
      e.preventDefault();
      if (!carriesLink(e)) return;
      depth++;
      setOver(true);
    }
    function leave(e: DragEvent) {
      if (!carriesLink(e)) return;
      depth = Math.max(0, depth - 1);
      if (depth === 0) setOver(false);
    }
    function overWindow(e: DragEvent) {
      e.preventDefault();
      if (e.dataTransfer) e.dataTransfer.dropEffect = carriesLink(e) ? "copy" : "none";
    }
    function drop(e: DragEvent) {
      e.preventDefault();
      depth = 0;
      setOver(false);

      // uri-list is one link per line, with # lines as comments.
      const list = e.dataTransfer?.getData("text/uri-list") ?? "";
      const text = e.dataTransfer?.getData("text/plain") ?? "";
      const link = [...list.split(/\r?\n/), text]
        .map((line) => line.trim())
        .find((line) => line && !line.startsWith("#") && looksLikeURL(line));
      if (link) onDrop(link);
    }

    window.addEventListener("dragenter", enter);
    window.addEventListener("dragleave", leave);
    window.addEventListener("dragover", overWindow);
    window.addEventListener("drop", drop);
    return () => {
      window.removeEventListener("dragenter", enter);
      window.removeEventListener("dragleave", leave);
      window.removeEventListener("dragover", overWindow);
      window.removeEventListener("drop", drop);
    };
  }, [onDrop]);

  if (!over) return null;
  return (
    <div className="pointer-events-none absolute inset-0 z-20 flex items-center justify-center bg-canvas/85 p-xl">
      <div className="flex flex-col items-center gap-sm rounded-md border-2 border-dashed border-primary px-xxl py-xl">
        <span className="flex size-12 items-center justify-center rounded-pill bg-primary text-on-primary">
          <Icon.LinkIcon className="size-5" strokeWidth={1.75} aria-hidden />
        </span>
        <span className="text-heading text-ink">Drop to open this link</span>
      </div>
    </div>
  );
}
