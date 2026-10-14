"use client";

import { cx } from "@/lib/cx";
import { formatDuration } from "@/lib/format";
import { useNow } from "@/lib/use-now";

/**
 * Time left to decide a held action. When the deadline passes the action
 * expires and is stopped, so the last tenth of the window turns red.
 */
export function Deadline({
  receivedAt,
  deadline,
  bar = false,
  className,
}: {
  receivedAt: string;
  deadline: string | null;
  bar?: boolean;
  className?: string;
}) {
  const now = useNow(1000);
  if (!deadline) return null;
  const end = new Date(deadline).getTime();
  const start = new Date(receivedAt).getTime();
  const left = Math.max(0, (end - now) / 1000);
  const frac = end > start ? Math.min(1, Math.max(0, (end - now) / (end - start))) : 0;
  const urgent = frac < 0.1;
  const text = left <= 0 ? "Expiring" : `${formatDuration(left)} left`;
  if (!bar) return <span className={cx("figures", urgent ? "text-stop" : "text-ink-3", className)}>{text}</span>;
  return (
    <div className={className}>
      <div className="flex items-baseline justify-between text-xs">
        <span className="text-ink-3">Time to decide</span>
        <span className={cx("figures font-medium", urgent ? "text-stop" : "text-ink")}>{text}</span>
      </div>
      <div className="mt-1.5 h-1 overflow-hidden rounded-full bg-hover">
        <div
          className={cx("h-full rounded-full transition-[width] duration-1000 ease-linear", urgent ? "bg-stop" : "bg-held")}
          style={{ width: `${frac * 100}%` }}
        />
      </div>
    </div>
  );
}
