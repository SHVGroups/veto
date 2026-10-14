import type { ReactNode } from "react";
import { cx } from "@/lib/cx";

/** The page title block: what this page is, and what you can do here. */
export function PageHeader({
  title,
  description,
  actions,
  meta,
}: {
  title: ReactNode;
  description?: ReactNode;
  actions?: ReactNode;
  meta?: ReactNode;
}) {
  return (
    <header className="mb-7 flex flex-wrap items-end justify-between gap-x-8 gap-y-4">
      <div className="min-w-0 max-w-3xl">
        {meta && <div className="mb-2 text-sm text-ink-3">{meta}</div>}
        <h1 className="text-2xl text-ink">{title}</h1>
        {description && <p className="mt-1.5 text-base text-ink-2">{description}</p>}
      </div>
      {actions && <div className="flex flex-wrap items-center gap-2">{actions}</div>}
    </header>
  );
}

/** A surface that holds one piece of the page. */
export function Panel({
  children,
  className,
  padded = true,
  as: As = "section",
}: {
  children: ReactNode;
  className?: string;
  padded?: boolean;
  as?: "section" | "div" | "article";
}) {
  return <As className={cx("min-w-0 rounded-panel bg-surface", padded && "p-5 sm:p-6", className)}>{children}</As>;
}

export function PanelHeader({
  title,
  description,
  actions,
  className,
}: {
  title: ReactNode;
  description?: ReactNode;
  actions?: ReactNode;
  className?: string;
}) {
  return (
    <div className={cx("mb-4 flex items-start justify-between gap-4", className)}>
      <div className="min-w-0">
        <h2 className="text-base text-ink">{title}</h2>
        {description && <p className="mt-0.5 text-sm text-ink-3">{description}</p>}
      </div>
      {actions && <div className="flex shrink-0 items-center gap-2">{actions}</div>}
    </div>
  );
}

/** A figure with its label, read top to bottom: label, number, context. */
export function Stat({
  label,
  value,
  context,
  tone,
  className,
}: {
  label: ReactNode;
  value: ReactNode;
  context?: ReactNode;
  tone?: "proceed" | "safer" | "stop" | "held";
  className?: string;
}) {
  const toneClass = tone ? { proceed: "text-proceed", safer: "text-safer", stop: "text-stop", held: "text-held" }[tone] : "text-ink";
  return (
    <div className={cx("min-w-0", className)}>
      <div className="text-sm text-ink-3">{label}</div>
      <div className={cx("numeral mt-1 text-3xl", toneClass)}>{value}</div>
      {context && <div className="mt-1 text-xs text-ink-3">{context}</div>}
    </div>
  );
}

/** Two-column key/value rows. */
export function Details({ rows, className }: { rows: [ReactNode, ReactNode][]; className?: string }) {
  return (
    <dl className={cx("grid grid-cols-[minmax(7rem,auto)_1fr] gap-x-6 gap-y-2.5 text-sm", className)}>
      {rows.map(([k, v], i) => (
        <div key={i} className="contents">
          <dt className="text-ink-3">{k}</dt>
          <dd className="min-w-0 break-words text-ink">{v}</dd>
        </div>
      ))}
    </dl>
  );
}

export function Kbd({ children }: { children: ReactNode }) {
  return (
    <kbd className="inline-flex h-5 min-w-5 items-center justify-center rounded-[5px] bg-sunken px-1 font-sans text-2xs font-medium text-ink-2 shadow-[inset_0_0_0_1px_var(--line-strong)]">
      {children}
    </kbd>
  );
}

export function Divider({ className }: { className?: string }) {
  return <hr className={cx("border-0 border-t border-line", className)} />;
}
