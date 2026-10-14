"use client";

import { max } from "d3-array";
import { scaleBand, scaleLinear } from "d3-scale";
import { line, curveMonotoneX } from "d3-shape";
import { useState } from "react";
import { formatCount, formatMs } from "@/lib/format";
import type { LatencyPoint, SeriesPoint } from "@/lib/types";
import { ChartTooltip, TooltipRow } from "./tooltip";
import { useWidth } from "./use-size";

const bucketLabel = (iso: string, bucketSeconds: number) => {
  const d = new Date(iso);
  if (bucketSeconds >= 86_400) return d.toLocaleDateString("en-US", { month: "short", day: "numeric" });
  if (bucketSeconds >= 3600) return d.toLocaleString("en-US", { month: "short", day: "numeric", hour: "numeric" });
  return d.toLocaleTimeString("en-US", { hour: "numeric", minute: "2-digit" });
};

const tickLabel = (iso: string, bucketSeconds: number) => {
  const d = new Date(iso);
  if (bucketSeconds >= 6 * 3600) return d.toLocaleDateString("en-US", { month: "short", day: "numeric" });
  return d.toLocaleTimeString("en-US", { hour: "numeric", minute: bucketSeconds < 3600 ? "2-digit" : undefined });
};

const SERIES = [
  { key: "auto_approved", label: "Ran without a person", color: "var(--chart-1)" },
  { key: "human_approved", label: "Approved by a reviewer", color: "var(--held)" },
  { key: "denied", label: "Stopped", color: "var(--stop)" },
  { key: "expired", label: "Expired", color: "color-mix(in srgb, var(--stop) 45%, var(--surface))" },
  { key: "pending", label: "Waiting", color: "color-mix(in srgb, var(--held) 35%, var(--surface))" },
] as const;

export const volumeLegend = SERIES.map(({ label, color }) => ({ label, color }));

/** Actions per time bucket, stacked by how each one ended. */
export function VolumeChart({ series, bucketSeconds, height = 220 }: { series: SeriesPoint[]; bucketSeconds: number; height?: number }) {
  const [ref, width] = useWidth<HTMLDivElement>();
  const [hover, setHover] = useState<number | null>(null);
  const m = { top: 8, right: 4, bottom: 24, left: 36 };
  const innerW = Math.max(10, width - m.left - m.right);
  const innerH = height - m.top - m.bottom;

  const totals = series.map((p) => SERIES.reduce((s, k) => s + p[k.key], 0));
  const yMax = Math.max(4, max(totals) ?? 0);
  const x = scaleBand<number>()
    .domain(series.map((_, i) => i))
    .range([0, innerW])
    .paddingInner(series.length > 60 ? 0.15 : 0.28)
    .paddingOuter(0.1);
  const y = scaleLinear().domain([0, yMax]).nice(4).range([innerH, 0]);
  const ticks = y.ticks(4);
  const labelEvery = Math.max(1, Math.ceil(series.length / Math.max(2, Math.floor(innerW / 90))));
  const bw = x.bandwidth();
  const point = hover !== null ? series[hover] : undefined;

  return (
    <div ref={ref} className="relative" onMouseLeave={() => setHover(null)}>
      <svg width={width} height={height} role="img" aria-label="Actions over time by result">
        <g transform={`translate(${m.left},${m.top})`}>
          {ticks.map((t) => (
            <g key={t} transform={`translate(0,${y(t)})`}>
              <line x1={0} x2={innerW} stroke="var(--line)" />
              <text x={-10} dy="0.32em" textAnchor="end" className="figures fill-ink-3 text-[11px]">
                {formatCount(t)}
              </text>
            </g>
          ))}
          {series.map((p, i) => {
            let acc = 0;
            const x0 = x(i) ?? 0;
            return (
              <g key={p.bucket} opacity={hover === null || hover === i ? 1 : 0.45}>
                {SERIES.map((s) => {
                  const v = p[s.key];
                  if (!v) return null;
                  const y1 = y(acc + v);
                  const h = y(acc) - y1;
                  acc += v;
                  return <rect key={s.key} x={x0} y={y1} width={bw} height={Math.max(0, h - 1)} rx={Math.min(2, bw / 4)} fill={s.color} />;
                })}
                <rect
                  x={x0 - (x.step() - bw) / 2}
                  y={0}
                  width={x.step()}
                  height={innerH}
                  fill="transparent"
                  onMouseEnter={() => setHover(i)}
                />
              </g>
            );
          })}
          {series.map((p, i) =>
            i % labelEvery === 0 ? (
              <text key={p.bucket} x={(x(i) ?? 0) + bw / 2} y={innerH + 17} textAnchor="middle" className="fill-ink-3 text-[11px]">
                {tickLabel(p.bucket, bucketSeconds)}
              </text>
            ) : null,
          )}
        </g>
      </svg>
      {point && hover !== null && (
        <ChartTooltip x={m.left + (x(hover) ?? 0) + bw} y={m.top + 10} width={width}>
          <div className="mb-1.5 font-medium text-ink">{bucketLabel(point.bucket, bucketSeconds)}</div>
          {SERIES.map((s) => (
            <TooltipRow key={s.key} swatch={s.color} label={s.label} value={formatCount(point[s.key])} />
          ))}
          <div className="mt-1.5 border-t border-line pt-1.5">
            <TooltipRow label="Held for review" value={formatCount(point.held)} />
          </div>
        </ChartTooltip>
      )}
    </div>
  );
}

/** Decision-model latency per bucket: median and 95th percentile. */
export function LatencyChart({ points, bucketSeconds, height = 180 }: { points: LatencyPoint[]; bucketSeconds: number; height?: number }) {
  const [ref, width] = useWidth<HTMLDivElement>();
  const [hover, setHover] = useState<number | null>(null);
  const m = { top: 10, right: 6, bottom: 24, left: 56 };
  const innerW = Math.max(10, width - m.left - m.right);
  const innerH = height - m.top - m.bottom;
  const yMax = Math.max(50, max(points, (p) => p.p95 ?? 0) ?? 0);
  const x = scaleLinear()
    .domain([0, Math.max(1, points.length - 1)])
    .range([0, innerW]);
  const y = scaleLinear().domain([0, yMax]).nice(4).range([innerH, 0]);
  const mk = (k: "p50" | "p95") =>
    line<LatencyPoint>()
      .defined((p) => p[k] !== null)
      .x((_, i) => x(i))
      .y((p) => y(p[k] ?? 0))
      .curve(curveMonotoneX)(points) ?? "";
  const labelEvery = Math.max(1, Math.ceil(points.length / Math.max(2, Math.floor(innerW / 90))));
  const p = hover !== null ? points[hover] : undefined;

  return (
    <div
      ref={ref}
      className="relative"
      onMouseLeave={() => setHover(null)}
      onMouseMove={(e) => {
        const r = e.currentTarget.getBoundingClientRect();
        const i = Math.round(x.invert(e.clientX - r.left - m.left));
        setHover(i >= 0 && i < points.length ? i : null);
      }}
    >
      <svg width={width} height={height} role="img" aria-label="Decision model latency over time">
        <g transform={`translate(${m.left},${m.top})`}>
          {y.ticks(4).map((t) => (
            <g key={t} transform={`translate(0,${y(t)})`}>
              <line x1={0} x2={innerW} stroke="var(--line)" />
              <text x={-10} dy="0.32em" textAnchor="end" className="figures fill-ink-3 text-[11px]">
                {t} ms
              </text>
            </g>
          ))}
          <path d={mk("p95")} fill="none" stroke="var(--chart-2)" strokeWidth={1.5} strokeDasharray="4 3" />
          <path d={mk("p50")} fill="none" stroke="var(--chart-1)" strokeWidth={2} />
          {points.map((pt, i) => (pt.errors > 0 ? <circle key={pt.bucket} cx={x(i)} cy={innerH - 2} r={3} fill="var(--stop)" /> : null))}
          {hover !== null && p && (
            <g>
              <line x1={x(hover)} x2={x(hover)} y1={0} y2={innerH} stroke="var(--line-strong)" />
              {p.p50 !== null && (
                <circle cx={x(hover)} cy={y(p.p50)} r={3.5} fill="var(--surface)" stroke="var(--chart-1)" strokeWidth={2} />
              )}
            </g>
          )}
          {points.map((pt, i) =>
            i % labelEvery === 0 ? (
              <text key={pt.bucket} x={x(i)} y={innerH + 17} textAnchor="middle" className="fill-ink-3 text-[11px]">
                {tickLabel(pt.bucket, bucketSeconds)}
              </text>
            ) : null,
          )}
        </g>
      </svg>
      {p && hover !== null && (
        <ChartTooltip x={m.left + x(hover)} y={m.top + 8} width={width}>
          <div className="mb-1.5 font-medium text-ink">{bucketLabel(p.bucket, bucketSeconds)}</div>
          <TooltipRow swatch="var(--chart-1)" label="Median" value={formatMs(p.p50)} />
          <TooltipRow swatch="var(--chart-2)" label="95th percentile" value={formatMs(p.p95)} />
          <TooltipRow label="Calls" value={formatCount(p.calls)} />
          {p.errors > 0 && <TooltipRow swatch="var(--stop)" label="Errors" value={formatCount(p.errors)} />}
        </ChartTooltip>
      )}
    </div>
  );
}

/** A tiny trend line for table rows and summaries. */
export function Sparkline({
  values,
  width = 96,
  height = 24,
  className,
}: {
  values: number[];
  width?: number;
  height?: number;
  className?: string;
}) {
  if (values.length < 2) return <svg width={width} height={height} className={className} aria-hidden="true" />;
  const x = scaleLinear()
    .domain([0, values.length - 1])
    .range([1, width - 1]);
  const y = scaleLinear()
    .domain([0, Math.max(1, max(values) ?? 1)])
    .range([height - 2, 2]);
  const d =
    line<number>()
      .x((_, i) => x(i))
      .y((v) => y(v))
      .curve(curveMonotoneX)(values) ?? "";
  return (
    <svg width={width} height={height} className={className} aria-hidden="true">
      <path d={d} fill="none" stroke="currentColor" strokeWidth={1.5} />
    </svg>
  );
}
