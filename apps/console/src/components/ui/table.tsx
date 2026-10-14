"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import type { Route } from "next";
import { useRef, useState, useEffect, type ReactNode, type ThHTMLAttributes, type TdHTMLAttributes } from "react";
import { cx } from "@/lib/cx";

export function Table({ children, className }: { children: ReactNode; className?: string }) {
  return (
    <div className={cx("scroll-quiet -mx-1 overflow-x-auto px-1", className)}>
      <table className="w-full border-separate border-spacing-0 text-sm">{children}</table>
    </div>
  );
}

export function Th({ className, children, ...rest }: ThHTMLAttributes<HTMLTableCellElement>) {
  return (
    <th
      className={cx("border-b border-line py-2.5 pr-4 text-left text-xs font-medium whitespace-nowrap text-ink-3 first:pl-1", className)}
      {...rest}
    >
      {children}
    </th>
  );
}

export function Td({ className, children, ...rest }: TdHTMLAttributes<HTMLTableCellElement>) {
  return (
    <td className={cx("border-b border-line py-3 pr-4 align-middle first:pl-1", className)} {...rest}>
      {children}
    </td>
  );
}

/**
 * A table row that opens a page when clicked anywhere. Put a real link to
 * the same page in the row's main cell for keyboard and new-tab use.
 */
export function LinkRow({ href, children, className }: { href: string; children: ReactNode; className?: string }) {
  const router = useRouter();
  return (
    <tr
      onClick={(e) => {
        if ((e.target as HTMLElement).closest("a,button,input,select,textarea")) return;
        if (e.metaKey || e.ctrlKey) window.open(href, "_blank", "noopener");
        else router.push(href as Route);
      }}
      className={cx("group cursor-pointer transition-colors hover:bg-sunken", className)}
    >
      {children}
    </tr>
  );
}

/** The main cell link of a LinkRow. */
export function RowLink({ href, children, className }: { href: string; children: ReactNode; className?: string }) {
  return (
    <Link href={href as Route} className={cx("font-medium text-ink outline-none focus-visible:underline", className)}>
      {children}
    </Link>
  );
}

/** Tabs that switch content in place. */
export function Tabs<T extends string>({
  tabs,
  value,
  onChange,
  className,
}: {
  tabs: { value: T; label: ReactNode; count?: number }[];
  value: T;
  onChange: (v: T) => void;
  className?: string;
}) {
  return (
    <div role="tablist" className={cx("flex gap-1 border-b border-line", className)}>
      {tabs.map((t) => {
        const on = t.value === value;
        return (
          <button
            key={t.value}
            role="tab"
            type="button"
            aria-selected={on}
            onClick={() => onChange(t.value)}
            className={cx(
              "relative -mb-px inline-flex h-10 items-center gap-2 px-3 text-sm font-medium transition-colors",
              on ? "text-ink" : "text-ink-3 hover:text-ink",
            )}
          >
            {t.label}
            {t.count !== undefined && <span className="figures text-xs text-ink-3">{t.count}</span>}
            {on && <span className="absolute inset-x-2 bottom-0 h-[2px] rounded-full bg-ink" />}
          </button>
        );
      })}
    </div>
  );
}

/** A small popover menu opened from a trigger button. */
export function Menu({
  trigger,
  children,
  align = "end",
}: {
  trigger: (props: { onClick: () => void; "aria-expanded": boolean; "aria-haspopup": "menu" }) => ReactNode;
  children: (close: () => void) => ReactNode;
  align?: "start" | "end";
}) {
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    function onDown(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false);
    }
    function onKey(e: KeyboardEvent) {
      if (e.key === "Escape") setOpen(false);
    }
    document.addEventListener("mousedown", onDown);
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("mousedown", onDown);
      document.removeEventListener("keydown", onKey);
    };
  }, [open]);

  return (
    <div ref={ref} className="relative inline-block">
      {trigger({ onClick: () => setOpen((o) => !o), "aria-expanded": open, "aria-haspopup": "menu" })}
      {open && (
        <div
          role="menu"
          className={cx(
            "absolute top-full z-40 mt-1.5 min-w-48 animate-[veto-arrive_140ms_ease-out] rounded-[14px] bg-surface p-1.5 shadow-pop",
            align === "end" ? "right-0" : "left-0",
          )}
        >
          {children(() => setOpen(false))}
        </div>
      )}
    </div>
  );
}

export function MenuItem({
  onSelect,
  children,
  tone,
  icon,
}: {
  onSelect: () => void;
  children: ReactNode;
  tone?: "danger";
  icon?: ReactNode;
}) {
  return (
    <button
      type="button"
      role="menuitem"
      onClick={onSelect}
      className={cx(
        "flex w-full items-center gap-2.5 rounded-[9px] px-2.5 py-2 text-left text-sm hover:bg-hover",
        tone === "danger" ? "text-stop" : "text-ink",
      )}
    >
      {icon && <span className="text-ink-3">{icon}</span>}
      {children}
    </button>
  );
}
