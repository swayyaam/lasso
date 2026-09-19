import type { ReactNode, SelectHTMLAttributes, InputHTMLAttributes } from "react";
import { cx } from "../cx";

/** Field pairs a label with a control and optional help text. */
export function Field({
  label,
  hint,
  htmlFor,
  children,
  className,
}: {
  label: string;
  hint?: ReactNode;
  htmlFor?: string;
  children: ReactNode;
  className?: string;
}) {
  return (
    <div className={cx("flex flex-col gap-xxs", className)}>
      <label htmlFor={htmlFor} className="text-caption font-medium text-ink-muted">
        {label}
      </label>
      {children}
      {hint && <p className="text-caption text-ink-tertiary">{hint}</p>}
    </div>
  );
}

/** Select is a native dropdown styled to sit on the surface ladder. */
export function Select({
  className,
  children,
  ...rest
}: SelectHTMLAttributes<HTMLSelectElement>) {
  return (
    <select
      className={cx(
        "no-drag h-9 w-full rounded-sm border border-hairline bg-canvas px-xs",
        "text-body-sm text-ink transition-colors duration-150 ease-standard",
        "hover:border-hairline-strong focus:border-primary focus:outline-none",
        "disabled:bg-surface-2 disabled:text-ink-faint",
        className,
      )}
      {...rest}
    >
      {children}
    </select>
  );
}

/** Checkbox is a labelled toggle sized for a dense settings list. */
export function Checkbox({
  label,
  hint,
  className,
  ...rest
}: { label: string; hint?: string } & InputHTMLAttributes<HTMLInputElement>) {
  return (
    <label className={cx("no-drag flex cursor-pointer items-start gap-xs py-0.5", className)}>
      <input
        type="checkbox"
        className={cx(
          "mt-0.5 size-4 shrink-0 cursor-pointer appearance-none rounded-xs",
          "border border-hairline-strong bg-canvas transition-colors duration-150",
          "hover:border-primary",
          "checked:border-primary checked:bg-primary",
          // The tick is drawn with a border rather than an SVG so it inherits
          // the token colours and needs no asset.
          "checked:after:ml-[4px] checked:after:mt-[0.5px] checked:after:block checked:after:h-[8px] checked:after:w-[4px]",
          "checked:after:rotate-45 checked:after:border-b-2 checked:after:border-r-2 checked:after:border-on-primary",
          "disabled:cursor-not-allowed disabled:border-hairline",
        )}
        {...rest}
      />
      <span className="min-w-0">
        <span className="block text-body-sm text-ink">{label}</span>
        {hint && <span className="block text-caption text-ink-tertiary">{hint}</span>}
      </span>
    </label>
  );
}
