"use client";

import type { ReactNode } from "react";
import { ApiError, errorMessage } from "@/lib/api";
import { cx } from "@/lib/cx";
import { Icon } from "../icons";
import { Button } from "./button";

/** What to show when there is nothing yet, and what to do about it. */
export function EmptyState({
  art,
  title,
  children,
  action,
  className,
}: {
  art?: ReactNode;
  title: ReactNode;
  children?: ReactNode;
  action?: ReactNode;
  className?: string;
}) {
  return (
    <div className={cx("flex flex-col items-center px-6 py-14 text-center", className)}>
      {art && <div className="mb-5 w-40 text-ink-3">{art}</div>}
      <h3 className="text-lg text-ink">{title}</h3>
      {children && <div className="mt-1.5 max-w-md text-sm text-ink-2">{children}</div>}
      {action && <div className="mt-5">{action}</div>}
    </div>
  );
}

/** A failed request: what went wrong, and a way to try again. */
export function ErrorState({ error, onRetry, className }: { error: unknown; onRetry?: () => void; className?: string }) {
  const requestId = error instanceof ApiError ? error.requestId : undefined;
  return (
    <div className={cx("flex items-start gap-3 rounded-[14px] bg-stop-wash p-4 text-sm", className)} role="alert">
      <Icon.Alert size={18} className="mt-0.5 shrink-0 text-stop" />
      <div className="min-w-0 flex-1">
        <p className="font-medium text-ink">{errorMessage(error)}</p>
        {requestId && <p className="mt-0.5 text-xs text-ink-3">Request {requestId}</p>}
      </div>
      {onRetry && (
        <Button size="sm" variant="secondary" onClick={onRetry} icon={<Icon.Refresh size={14} />}>
          Try again
        </Button>
      )}
    </div>
  );
}

export function Notice({
  tone = "neutral",
  title,
  children,
  action,
  className,
}: {
  tone?: "neutral" | "held" | "safer" | "stop" | "proceed";
  title?: ReactNode;
  children?: ReactNode;
  action?: ReactNode;
  className?: string;
}) {
  const wash = {
    neutral: "bg-sunken shadow-[inset_0_0_0_1px_var(--line)]",
    held: "bg-held-wash",
    safer: "bg-safer-wash",
    stop: "bg-stop-wash",
    proceed: "bg-proceed-wash",
  }[tone];
  const iconTone = { neutral: "text-ink-3", held: "text-held", safer: "text-safer", stop: "text-stop", proceed: "text-proceed" }[tone];
  return (
    <div className={cx("flex items-start gap-3 rounded-[14px] p-4 text-sm", wash, className)}>
      <Icon.Info size={18} className={cx("mt-0.5 shrink-0", iconTone)} />
      <div className="min-w-0 flex-1 text-ink-2">
        {title && <p className="font-medium text-ink">{title}</p>}
        {children && <div className={title ? "mt-0.5" : undefined}>{children}</div>}
      </div>
      {action && <div className="shrink-0">{action}</div>}
    </div>
  );
}

export function Skeleton({ className }: { className?: string }) {
  return <div className={cx("animate-pulse rounded-[8px] bg-hover", className)} />;
}

/** A panel-sized placeholder while data loads. */
export function LoadingPanel({ rows = 4, className }: { rows?: number; className?: string }) {
  return (
    <div className={cx("space-y-3", className)} aria-busy="true" aria-label="Loading">
      <Skeleton className="h-5 w-40" />
      {Array.from({ length: rows }, (_, i) => (
        <Skeleton key={i} className="h-9 w-full" />
      ))}
    </div>
  );
}
