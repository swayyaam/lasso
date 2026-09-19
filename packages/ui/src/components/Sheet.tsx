import * as Dialog from "@radix-ui/react-dialog";
import type { ReactNode } from "react";
import { cx } from "../cx";

/**
 * Sheet is a panel that covers the canvas rather than opening a second window.
 *
 * This is Radix rather than a positioned div because the visible part of a
 * dialog is the easy part. The rest — trapping focus inside it, restoring focus
 * to whatever opened it, marking the rest of the app inert for screen readers,
 * locking the background from scrolling, closing on Escape and on a click
 * outside but not on a drag that started inside — is a long list that a
 * hand-rolled overlay gets about half of, and the half it misses is the half
 * that only some people notice.
 *
 * The surface is the document's modal treatment: card chrome at 8px with the
 * heavy multi-stop shadow.
 */
export function Sheet({
  open,
  onOpenChange,
  title,
  description,
  footer,
  children,
  className,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  /** Screen-reader description. Rendered visually only if you pass a node. */
  description?: ReactNode;
  footer?: ReactNode;
  children: ReactNode;
  className?: string;
}) {
  return (
    <Dialog.Root open={open} onOpenChange={onOpenChange}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-40 bg-overlay/40 backdrop-blur-sm" />
        <Dialog.Content
          className={cx(
            "fixed inset-0 z-50 m-auto flex h-fit max-h-[calc(100%-var(--spacing-xl))] w-full max-w-sheet flex-col",
            "rounded-md border border-hairline bg-canvas shadow-modal",
            // The frameless window drags by its chrome; a sheet is not chrome.
            "no-drag",
            className,
          )}
        >
          <header className="flex shrink-0 items-center justify-between gap-sm border-b border-hairline px-md py-sm">
            <Dialog.Title className="text-card-title text-ink">{title}</Dialog.Title>
            <Dialog.Close asChild>
              <button
                type="button"
                aria-label="Close"
                className={cx(
                  "no-drag inline-flex size-7 items-center justify-center rounded-sm",
                  "text-ink-subtle transition-colors duration-150 ease-standard",
                  "hover:bg-surface-2 hover:text-ink active:bg-surface-3",
                )}
              >
                <CloseGlyph />
              </button>
            </Dialog.Close>
          </header>

          {description && (
            <Dialog.Description className="shrink-0 px-md pt-sm text-body-sm text-ink-subtle">
              {description}
            </Dialog.Description>
          )}

          <div className="min-h-0 flex-1 overflow-y-auto p-md">{children}</div>

          {footer && (
            <footer className="flex shrink-0 items-center justify-end gap-xs border-t border-hairline px-md py-sm">
              {footer}
            </footer>
          )}
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}

/**
 * The close glyph is drawn rather than imported so the primitive has no icon
 * dependency of its own — every other icon in the app comes from the shared
 * vocabulary, and a dialog's close button is chrome, not an action in it.
 */
function CloseGlyph() {
  return (
    <svg width="14" height="14" viewBox="0 0 14 14" fill="none" aria-hidden>
      <path
        d="M3.5 3.5l7 7M10.5 3.5l-7 7"
        stroke="currentColor"
        strokeWidth="1.5"
        strokeLinecap="round"
      />
    </svg>
  );
}
