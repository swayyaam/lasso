import * as RadixSelect from "@radix-ui/react-select";
import { Check, ChevronDown } from "lucide-react";
import type { ReactNode } from "react";
import { cx } from "../cx";

export type DropdownOption = { value: string; label: string; hint?: string };

/**
 * Dropdown replaces the native <select>.
 *
 * A native select renders its popup with the operating system's own chrome,
 * which on a white canvas means a control that follows neither the token
 * colours nor the 4px shape system — and, with the webview set to light, one
 * that still looked out of place against every other control.
 *
 * Radix is worth the dependency here specifically: a listbox is the one
 * pattern where hand-rolling means re-implementing typeahead, roving focus,
 * scroll-into-view and dismissal, and getting any of them wrong makes the
 * control unusable by keyboard rather than merely unattractive.
 *
 * It keeps the native `value` / `onChange` shape so callers read the same as
 * they did with a <select>.
 */
export function Dropdown({
  value,
  onChange,
  options,
  placeholder = "Choose…",
  disabled,
  ariaLabel,
  className,
}: {
  value: string;
  onChange: (value: string) => void;
  options: DropdownOption[];
  placeholder?: string;
  disabled?: boolean;
  ariaLabel?: string;
  className?: string;
}) {
  return (
    <RadixSelect.Root value={value} onValueChange={onChange} disabled={disabled}>
      <RadixSelect.Trigger
        aria-label={ariaLabel}
        className={cx(
          "no-drag flex h-9 w-full items-center justify-between gap-xs rounded-sm border px-xs",
          "border-hairline bg-canvas text-body-sm text-ink",
          "transition-colors duration-150 ease-standard",
          "hover:border-hairline-strong data-[state=open]:border-primary",
          "disabled:pointer-events-none disabled:bg-surface-2 disabled:text-ink-faint",
          className,
        )}
      >
        {/* Radix shows the placeholder whenever the value is "", which is
            exactly the value an "Automatic" or "None" option has — so those
            read as "Select" rather than as themselves. The matching option's
            own label stands in for the placeholder instead. */}
        <RadixSelect.Value placeholder={options.find((o) => o.value === value)?.label ?? placeholder} />
        <RadixSelect.Icon asChild>
          <ChevronDown className="size-4 shrink-0 text-ink-tertiary" aria-hidden />
        </RadixSelect.Icon>
      </RadixSelect.Trigger>

      <RadixSelect.Portal>
        <RadixSelect.Content
          position="popper"
          sideOffset={4}
          className={cx(
            "z-50 max-h-64 min-w-[var(--radix-select-trigger-width)] overflow-hidden",
            "rounded-md border border-hairline bg-canvas shadow-layered-strong",
          )}
        >
          <RadixSelect.Viewport className="p-xxs">
            {options.map((option) => (
              <RadixSelect.Item
                key={option.value}
                value={option.value}
                className={cx(
                  "relative flex cursor-pointer select-none items-center gap-xs rounded-sm",
                  "py-xxs pl-xs pr-xl text-body-sm text-ink outline-none",
                  "data-[highlighted]:bg-surface-2",
                  "data-[state=checked]:font-medium",
                )}
              >
                <span className="min-w-0 flex-1">
                  <RadixSelect.ItemText>{option.label}</RadixSelect.ItemText>
                  {option.hint && (
                    <span className="block text-caption text-ink-tertiary">{option.hint}</span>
                  )}
                </span>
                <RadixSelect.ItemIndicator className="absolute right-xs">
                  <Check className="size-3.5 text-ink" aria-hidden />
                </RadixSelect.ItemIndicator>
              </RadixSelect.Item>
            ))}
          </RadixSelect.Viewport>
        </RadixSelect.Content>
      </RadixSelect.Portal>
    </RadixSelect.Root>
  );
}

/** DropdownGroupLabel heads a section inside a dropdown. */
export function DropdownGroupLabel({ children }: { children: ReactNode }) {
  return (
    <div className="px-xs py-xxs text-eyebrow font-medium tracking-wide text-ink-tertiary uppercase">
      {children}
    </div>
  );
}
