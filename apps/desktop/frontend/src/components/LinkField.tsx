import { useState } from "react";
import type { Ref } from "react";
import { Button, Icon, cx } from "@lasso/ui";
import { looksLikeURL } from "../format";

/**
 * LinkField is where a link goes in.
 *
 * Large on Ready, where pasting is the only thing to do; compact above the
 * downloads, where it is the way to add another. Either way a paste resolves
 * at once — there is nothing to confirm about a link — and Enter does the same
 * for one typed by hand.
 */
export function LinkField({
  size,
  onSubmit,
  disabled,
  placeholder,
  autoFocus,
  inputRef,
}: {
  size: "large" | "compact";
  onSubmit: (url: string) => void;
  disabled?: boolean;
  placeholder: string;
  autoFocus?: boolean;
  inputRef?: Ref<HTMLInputElement>;
}) {
  const [value, setValue] = useState("");
  const large = size === "large";

  function submit(text: string) {
    const trimmed = text.trim();
    if (!trimmed) return;
    onSubmit(trimmed);
    // The link moves on to the Choose screen; the field is for the next one.
    setValue("");
  }

  return (
    <div
      className={cx(
        "no-drag flex items-center rounded-sm bg-canvas",
        "transition-[border-color,box-shadow] duration-150 ease-standard",
        large
          ? "h-16 gap-sm border border-primary pr-sm pl-md shadow-layered focus-within:shadow-layered-strong"
          : "h-11 gap-xs border border-hairline pr-xs pl-sm hover:border-hairline-strong focus-within:border-primary",
        disabled && "pointer-events-none opacity-60",
      )}
    >
      <Icon.LinkIcon
        className={cx("shrink-0 text-ink-subtle", large ? "size-5" : "size-4")}
        strokeWidth={1.75}
        aria-hidden
      />
      <input
        ref={inputRef}
        type="text"
        inputMode="url"
        value={value}
        autoFocus={autoFocus}
        disabled={disabled}
        spellCheck={false}
        autoCapitalize="off"
        autoCorrect="off"
        aria-label="Link to download"
        placeholder={placeholder}
        onChange={(e) => setValue(e.target.value)}
        onPaste={(e) => {
          // A paste that is a link is the whole interaction; anything else is
          // left to land in the field for editing.
          const text = e.clipboardData.getData("text");
          if (!looksLikeURL(text)) return;
          e.preventDefault();
          // Stop the window's own paste handler resolving it a second time.
          e.stopPropagation();
          submit(text);
        }}
        onKeyDown={(e) => {
          if (e.key === "Enter") submit(value);
        }}
        className={cx(
          "h-full min-w-0 flex-1 bg-transparent text-ink outline-none placeholder:text-ink-tertiary",
          large ? "text-body" : "text-body-sm",
        )}
      />
      {value.trim() ? (
        <Button variant="primary" size="sm" onClick={() => submit(value)}>
          Continue
        </Button>
      ) : (
        <kbd
          className="shrink-0 rounded-sm border border-hairline bg-surface-1 px-xs py-0.5 font-sans text-caption text-ink-muted"
          title="Paste a link anywhere in the window"
        >
          ⌘V
        </kbd>
      )}
    </div>
  );
}
