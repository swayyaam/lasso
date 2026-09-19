import type { InputHTMLAttributes, ReactNode, Ref } from "react";
import { cx } from "../cx";

/**
 * Input is the `text-input` component: canvas fill, hairline border, 4px
 * corners.
 *
 * The focus ring comes from the global :focus-visible rule, but a text field
 * is focused by clicking into it rather than by tabbing, so it also darkens
 * its own border on focus-within — otherwise the one control you are actually
 * typing into is the one with no state.
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
        "no-drag flex items-center gap-xs rounded-sm border bg-canvas px-sm",
        "transition-colors duration-150 ease-standard",
        invalid
          ? "border-danger"
          : "border-hairline hover:border-hairline-strong focus-within:border-primary",
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
