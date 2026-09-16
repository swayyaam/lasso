import type { InputHTMLAttributes, ReactNode, Ref } from "react";
import { cx } from "../cx";

/**
 * Input is the `text-input` component from design.md: a surface-1 field with
 * an 8px radius. The focus ring comes from the global :focus-visible rule
 * rather than being restated here.
 */
export function Input({
  className,
  invalid,
  leading,
  trailing,
  ref,
  ...rest
}: {
  invalid?: boolean;
  leading?: ReactNode;
  trailing?: ReactNode;
  /** Forwarded to the inner input. React 19 takes ref as an ordinary prop. */
  ref?: Ref<HTMLInputElement>;
} & InputHTMLAttributes<HTMLInputElement>) {
  return (
    <div
      className={cx(
        "no-drag flex items-center gap-xs rounded-md border bg-surface-1 px-sm",
        "transition-colors duration-150",
        invalid ? "border-danger" : "border-hairline focus-within:border-hairline-strong",
        className,
      )}
    >
      {leading && <span className="shrink-0 text-ink-subtle">{leading}</span>}
      <input
        ref={ref}
        className={cx(
          "h-9 min-w-0 flex-1 bg-transparent text-body-sm text-ink",
          "placeholder:text-ink-tertiary focus:outline-none",
        )}
        {...rest}
      />
      {trailing && <span className="shrink-0">{trailing}</span>}
    </div>
  );
}
