"use client";

import { forwardRef, useId, type InputHTMLAttributes, type ReactNode, type SelectHTMLAttributes, type TextareaHTMLAttributes } from "react";
import { cx } from "@/lib/cx";
import { Icon } from "../icons";

const control =
  "w-full rounded-control bg-surface text-ink placeholder:text-ink-3 shadow-[inset_0_0_0_1px_var(--line-strong)] transition-shadow outline-none focus:shadow-[inset_0_0_0_1.5px_var(--held)] disabled:bg-sunken disabled:text-ink-3 aria-[invalid=true]:shadow-[inset_0_0_0_1.5px_var(--stop)]";

export const Input = forwardRef<HTMLInputElement, InputHTMLAttributes<HTMLInputElement>>(function Input({ className, ...rest }, ref) {
  return <input ref={ref} className={cx(control, "h-9 px-3 text-sm", className)} {...rest} />;
});

export const Textarea = forwardRef<HTMLTextAreaElement, TextareaHTMLAttributes<HTMLTextAreaElement>>(function Textarea(
  { className, ...rest },
  ref,
) {
  return <textarea ref={ref} className={cx(control, "min-h-20 px-3 py-2 text-sm leading-relaxed", className)} {...rest} />;
});

export const Select = forwardRef<HTMLSelectElement, SelectHTMLAttributes<HTMLSelectElement>>(function Select(
  { className, children, ...rest },
  ref,
) {
  return (
    <div className={cx("relative", className)}>
      <select ref={ref} className={cx(control, "h-9 appearance-none pr-8 pl-3 text-sm")} {...rest}>
        {children}
      </select>
      <Icon.ChevronDown size={14} className="pointer-events-none absolute top-1/2 right-2.5 -translate-y-1/2 text-ink-3" />
    </div>
  );
});

/** A labelled form control with an optional hint and error. */
export function Field({
  label,
  hint,
  error,
  children,
  className,
  optional,
}: {
  label: ReactNode;
  hint?: ReactNode;
  error?: string | null;
  optional?: boolean;
  className?: string;
  children: (id: string, describedBy: string | undefined) => ReactNode;
}) {
  const id = useId();
  const hintId = hint ? `${id}-hint` : undefined;
  const errId = error ? `${id}-err` : undefined;
  const describedBy = [hintId, errId].filter(Boolean).join(" ") || undefined;
  return (
    <div className={cx("flex flex-col gap-1.5", className)}>
      <label htmlFor={id} className="text-sm font-medium text-ink">
        {label}
        {optional && <span className="ml-1.5 font-normal text-ink-3">Optional</span>}
      </label>
      {children(id, describedBy)}
      {hint && !error && (
        <p id={hintId} className="text-xs text-ink-3">
          {hint}
        </p>
      )}
      {error && (
        <p id={errId} className="text-xs text-stop">
          {error}
        </p>
      )}
    </div>
  );
}

export function Switch({
  checked,
  onChange,
  label,
  description,
  disabled,
}: {
  checked: boolean;
  onChange: (v: boolean) => void;
  label: ReactNode;
  description?: ReactNode;
  disabled?: boolean;
}) {
  const id = useId();
  return (
    <div className="flex items-start justify-between gap-6">
      <div className="min-w-0">
        <label htmlFor={id} className="text-sm font-medium text-ink">
          {label}
        </label>
        {description && <p className="mt-0.5 text-xs text-ink-3">{description}</p>}
      </div>
      <button
        id={id}
        type="button"
        role="switch"
        aria-checked={checked}
        disabled={disabled}
        onClick={() => onChange(!checked)}
        className={cx(
          "relative mt-0.5 inline-flex h-5 w-9 shrink-0 items-center rounded-full transition-colors disabled:opacity-45",
          checked ? "bg-ink" : "bg-line-strong",
        )}
      >
        <span
          className={cx(
            "inline-block size-4 rounded-full bg-surface shadow-sm transition-transform duration-150",
            checked ? "translate-x-[18px]" : "translate-x-0.5",
          )}
        />
      </button>
    </div>
  );
}

export function Checkbox({
  checked,
  onChange,
  label,
  disabled,
}: {
  checked: boolean;
  onChange: (v: boolean) => void;
  label: ReactNode;
  disabled?: boolean;
}) {
  return (
    <label className={cx("inline-flex items-center gap-2.5 text-sm text-ink", disabled && "opacity-50")}>
      <input
        type="checkbox"
        checked={checked}
        disabled={disabled}
        onChange={(e) => onChange(e.target.checked)}
        className="size-4 rounded-[5px] accent-[var(--ink)]"
      />
      {label}
    </label>
  );
}

/** A compact choice between a few values, such as a time range. */
export function Segmented<T extends string>({
  value,
  options,
  onChange,
  label,
  size = "md",
}: {
  value: T;
  options: { value: T; label: ReactNode }[];
  onChange: (v: T) => void;
  label: string;
  size?: "sm" | "md";
}) {
  return (
    <div role="radiogroup" aria-label={label} className="inline-flex rounded-[11px] bg-sunken p-0.5 shadow-[inset_0_0_0_1px_var(--line)]">
      {options.map((o) => {
        const on = o.value === value;
        return (
          <button
            key={o.value}
            type="button"
            role="radio"
            aria-checked={on}
            onClick={() => onChange(o.value)}
            className={cx(
              "rounded-[9px] font-medium transition-colors",
              size === "sm" ? "h-7 px-2.5 text-xs" : "h-8 px-3 text-sm",
              on ? "bg-surface text-ink shadow-[0_1px_2px_rgb(0_0_0/0.08),inset_0_0_0_1px_var(--line)]" : "text-ink-3 hover:text-ink",
            )}
          >
            {o.label}
          </button>
        );
      })}
    </div>
  );
}

/** A secret shown once, with a copy control and a warning to store it. */
export function SecretReveal({ label, secret, note }: { label: string; secret: string; note?: ReactNode }) {
  return (
    <div className="rounded-[14px] bg-sunken p-4 shadow-[inset_0_0_0_1px_var(--line)]">
      <p className="text-sm font-medium text-ink">{label}</p>
      <p className="mt-1 text-xs text-ink-3">{note ?? "Copy it now. Veto stores only a hash and cannot show it again."}</p>
      <div className="mt-3 flex items-center gap-2">
        <code className="min-w-0 flex-1 truncate rounded-[9px] bg-surface px-3 py-2 text-xs text-ink shadow-[inset_0_0_0_1px_var(--line)] select-all">
          {secret}
        </code>
        <CopyInline value={secret} />
      </div>
    </div>
  );
}

function CopyInline({ value }: { value: string }) {
  return (
    <button
      type="button"
      onClick={() => void navigator.clipboard.writeText(value)}
      className="inline-flex h-9 items-center gap-1.5 rounded-control bg-ink px-3 text-sm font-medium text-surface hover:opacity-90"
    >
      <Icon.Copy size={15} />
      Copy
    </button>
  );
}
