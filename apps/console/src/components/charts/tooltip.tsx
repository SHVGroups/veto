import type { ReactNode } from "react";

/** The hover card for charts, positioned by the chart inside its own box. */
export function ChartTooltip({ x, y, width, children }: { x: number; y: number; width: number; children: ReactNode }) {
  const flip = x > width - 200;
  return (
    <div
      className="pointer-events-none absolute z-10 min-w-40 rounded-[12px] bg-surface px-3 py-2.5 text-xs shadow-pop"
      style={{ left: flip ? undefined : x + 14, right: flip ? width - x + 14 : undefined, top: Math.max(0, y - 10) }}
    >
      {children}
    </div>
  );
}

export function TooltipRow({ swatch, label, value }: { swatch?: string; label: ReactNode; value: ReactNode }) {
  return (
    <div className="flex items-center justify-between gap-5 py-0.5">
      <span className="flex items-center gap-2 text-ink-2">
        {swatch && <span className="size-2 rounded-[3px]" style={{ background: swatch }} />}
        {label}
      </span>
      <span className="figures font-medium text-ink">{value}</span>
    </div>
  );
}
