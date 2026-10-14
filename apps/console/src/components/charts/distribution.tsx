"use client";

import { scaleBand, scaleLinear } from "d3-scale";
import Link from "next/link";
import type { Route } from "next";
import { useState } from "react";
import { cx } from "@/lib/cx";
import { formatCount, formatPercent, formatProbability } from "@/lib/format";
import type { CalibrationBand } from "@/lib/types";
import { ChartTooltip, TooltipRow } from "./tooltip";
import { useWidth } from "./use-size";

/**
 * Composite risk of model-assessed actions in ten bins from 0 to 1. Bins at
 * or above the ceiling of a medium-risk tool are drawn in the stop colour.
 */
export function RiskHistogram({ bins, ceiling = 0.45, height = 150 }: { bins: number[]; ceiling?: number; height?: number }) {
  const [ref, width] = useWidth<HTMLDivElement>();
  const [hover, setHover] = useState<number | null>(null);
  const m = { top: 6, right: 2, bottom: 22, left: 2 };
  const innerW = Math.max(10, width - m.left - m.right);
  const innerH = height - m.top - m.bottom;
  const x = scaleBand<number>()
    .domain(bins.map((_, i) => i))
    .range([0, innerW])
    .paddingInner(0.16);
  const y = scaleLinear()
    .domain([0, Math.max(1, ...bins)])
    .range([innerH, 0]);
  const total = bins.reduce((a, b) => a + b, 0);

  return (
    <div ref={ref} className="relative" onMouseLeave={() => setHover(null)}>
      <svg width={width} height={height} role="img" aria-label="Distribution of risk scores">
        <g transform={`translate(${m.left},${m.top})`}>
          {bins.map((v, i) => {
            const lo = i / bins.length;
            const tone = lo >= ceiling ? "var(--stop)" : lo >= ceiling - 0.2 ? "var(--safer)" : "var(--chart-1)";
            return (
              <g key={i} onMouseEnter={() => setHover(i)}>
                <rect x={x(i)} y={0} width={x.bandwidth()} height={innerH} fill="transparent" />
                <rect
                  x={x(i)}
                  y={y(v)}
                  width={x.bandwidth()}
                  height={Math.max(v ? 2 : 0, innerH - y(v))}
                  rx={3}
                  fill={tone}
                  opacity={hover === null || hover === i ? 1 : 0.4}
                />
              </g>
            );
          })}
          <line x1={0} x2={innerW} y1={innerH} y2={innerH} stroke="var(--line-strong)" />
          {[0, 0.5, 1].map((t) => (
            <text
              key={t}
              x={t * innerW}
              y={innerH + 16}
              textAnchor={t === 0 ? "start" : t === 1 ? "end" : "middle"}
              className="figures fill-ink-3 text-[11px]"
            >
              {t.toFixed(1)}
            </text>
          ))}
        </g>
      </svg>
      {hover !== null && (
        <ChartTooltip x={m.left + (x(hover) ?? 0) + x.bandwidth()} y={10} width={width}>
          <div className="mb-1 font-medium text-ink">
            Risk {(hover / bins.length).toFixed(1)} to {((hover + 1) / bins.length).toFixed(1)}
          </div>
          <TooltipRow label="Actions" value={formatCount(bins[hover] ?? 0)} />
          <TooltipRow label="Share" value={formatPercent(total ? (bins[hover] ?? 0) / total : 0)} />
        </ChartTooltip>
      )}
    </div>
  );
}

/** A ranked list with proportional bars; the held share is drawn inside each bar. */
export function BarList({
  items,
  hrefFor,
  emptyText = "Nothing yet.",
  labelFor,
}: {
  items: { key: string; label: string; count: number; held?: number }[];
  hrefFor?: (key: string) => string;
  emptyText?: string;
  labelFor?: (key: string, label: string) => string;
}) {
  if (!items.length) return <p className="py-6 text-sm text-ink-3">{emptyText}</p>;
  const top = Math.max(...items.map((i) => i.count));
  return (
    <ul className="space-y-1">
      {items.map((it) => {
        const w = (it.count / top) * 100;
        const heldW = it.held ? (it.held / it.count) * w : 0;
        const label = labelFor ? labelFor(it.key, it.label) : it.label;
        const content = (
          <>
            <div className="relative h-8 flex-1 overflow-hidden rounded-[8px]">
              <div className="absolute inset-y-0 left-0 rounded-[8px] bg-hover" style={{ width: `${w}%` }} />
              {heldW > 0 && <div className="absolute inset-y-0 left-0 rounded-l-[8px] bg-held-wash" style={{ width: `${heldW}%` }} />}
              <span className="relative flex h-full items-center truncate px-2.5 text-sm text-ink">{label}</span>
            </div>
            <span className="figures w-14 shrink-0 text-right text-sm text-ink">{formatCount(it.count)}</span>
            <span className="figures w-12 shrink-0 text-right text-xs text-held">{it.held ? formatCount(it.held) : ""}</span>
          </>
        );
        return (
          <li key={it.key}>
            {hrefFor ? (
              <Link href={hrefFor(it.key) as Route} className="flex items-center gap-3 rounded-[8px] hover:bg-sunken">
                {content}
              </Link>
            ) : (
              <div className="flex items-center gap-3">{content}</div>
            )}
          </li>
        );
      })}
    </ul>
  );
}

/**
 * Reliability of the model's confidence: for each confidence band, how often
 * reviewers' verdicts agreed with the model, with a 95% interval. A well
 * calibrated model sits on or above the target line in the bands it acts on.
 */
export function ReliabilityChart({
  bands,
  target,
  threshold,
  suggested,
  height = 260,
}: {
  bands: CalibrationBand[];
  target: number;
  threshold?: number;
  suggested?: number | null;
  height?: number;
}) {
  const [ref, width] = useWidth<HTMLDivElement>();
  const [hover, setHover] = useState<number | null>(null);
  const m = { top: 12, right: 12, bottom: 54, left: 42 };
  const innerW = Math.max(10, width - m.left - m.right);
  const innerH = height - m.top - m.bottom;
  const x = scaleLinear().domain([0, 1]).range([0, innerW]);
  const y = scaleLinear().domain([0, 1]).range([innerH, 0]);
  const maxN = Math.max(1, ...bands.map((b) => b.labeled));
  const countH = 26;
  const b = hover !== null ? bands[hover] : undefined;

  return (
    <div ref={ref} className="relative" onMouseLeave={() => setHover(null)}>
      <svg width={width} height={height} role="img" aria-label="Agreement between the model and reviewers by confidence">
        <g transform={`translate(${m.left},${m.top})`}>
          {[0, 0.25, 0.5, 0.75, 1].map((t) => (
            <g key={t}>
              <line x1={0} x2={innerW} y1={y(t)} y2={y(t)} stroke="var(--line)" />
              <text x={-10} y={y(t)} dy="0.32em" textAnchor="end" className="figures fill-ink-3 text-[11px]">
                {formatPercent(t)}
              </text>
            </g>
          ))}
          <line x1={0} x2={innerW} y1={y(target)} y2={y(target)} stroke="var(--proceed)" strokeDasharray="5 4" />
          <text x={innerW} y={y(target) - 6} textAnchor="end" className="fill-proceed text-[11px]">
            Target {formatPercent(target, 0)}
          </text>
          {threshold !== undefined && threshold <= 1 && (
            <g>
              <line x1={x(threshold)} x2={x(threshold)} y1={0} y2={innerH} stroke="var(--ink)" strokeWidth={1.5} />
              <text x={x(threshold) + 6} y={12} className="fill-ink text-[11px]">
                Auto above {formatProbability(threshold)}
              </text>
            </g>
          )}
          {suggested !== null && suggested !== undefined && suggested !== threshold && (
            <g>
              <line x1={x(suggested)} x2={x(suggested)} y1={0} y2={innerH} stroke="var(--held)" strokeDasharray="3 3" strokeWidth={1.5} />
              <text x={x(suggested) + 6} y={28} className="fill-held text-[11px]">
                Suggested {formatProbability(suggested)}
              </text>
            </g>
          )}
          {bands.map((band, i) => {
            const cx0 = x((band.low + band.high) / 2);
            const bw = Math.max(4, x(band.high) - x(band.low) - 6);
            const nh = (band.labeled / maxN) * countH;
            return (
              <g key={i} onMouseEnter={() => setHover(i)}>
                <rect x={x(band.low)} y={0} width={x(band.high) - x(band.low)} height={innerH + 40} fill="transparent" />
                <rect x={cx0 - bw / 2} y={innerH + 34 - nh} width={bw} height={nh} rx={2} fill="var(--line-strong)" />
                {band.agreement && (
                  <g opacity={hover === null || hover === i ? 1 : 0.4}>
                    <line
                      x1={cx0}
                      x2={cx0}
                      y1={y(band.agreement.low)}
                      y2={y(band.agreement.high)}
                      stroke="var(--ink-2)"
                      strokeWidth={1.5}
                    />
                    <line
                      x1={cx0 - 4}
                      x2={cx0 + 4}
                      y1={y(band.agreement.low)}
                      y2={y(band.agreement.low)}
                      stroke="var(--ink-2)"
                      strokeWidth={1.5}
                    />
                    <line
                      x1={cx0 - 4}
                      x2={cx0 + 4}
                      y1={y(band.agreement.high)}
                      y2={y(band.agreement.high)}
                      stroke="var(--ink-2)"
                      strokeWidth={1.5}
                    />
                    <circle
                      cx={cx0}
                      cy={y(band.agreement.value)}
                      r={5}
                      fill={
                        band.agreement.low >= target ? "var(--proceed)" : band.agreement.value >= target ? "var(--surface)" : "var(--stop)"
                      }
                      stroke={
                        band.agreement.low >= target ? "var(--proceed)" : band.agreement.value >= target ? "var(--proceed)" : "var(--stop)"
                      }
                      strokeWidth={2}
                    />
                  </g>
                )}
              </g>
            );
          })}
          <line x1={0} x2={innerW} y1={innerH} y2={innerH} stroke="var(--line-strong)" />
          {[0, 0.2, 0.4, 0.6, 0.8, 1].map((t) => (
            <text key={t} x={x(t)} y={innerH + 48} textAnchor="middle" className="figures fill-ink-3 text-[11px]">
              {t.toFixed(1)}
            </text>
          ))}
        </g>
      </svg>
      {b && hover !== null && (
        <ChartTooltip x={m.left + x(b.high)} y={m.top + 20} width={width}>
          <div className="mb-1.5 font-medium text-ink">
            Confidence {formatProbability(b.low)} to {formatProbability(b.high)}
          </div>
          <TooltipRow label="Labelled by reviewers" value={formatCount(b.labeled)} />
          <TooltipRow label="Agreed with the model" value={formatCount(b.agreed)} />
          <TooltipRow
            label="Agreement"
            value={
              b.agreement
                ? `${formatPercent(b.agreement.value)} (${formatPercent(b.agreement.low)} to ${formatPercent(b.agreement.high)})`
                : "–"
            }
          />
          <TooltipRow label="Ran without a person" value={formatCount(b.auto)} />
          <TooltipRow label="Held" value={formatCount(b.held)} />
        </ChartTooltip>
      )}
      <div className={cx("flex flex-wrap gap-x-5 gap-y-1 text-xs text-ink-3")} style={{ paddingLeft: m.left }}>
        <span>Model confidence (horizontal), agreement with reviewers (vertical)</span>
        <span>Bars below: labelled actions per band</span>
      </div>
    </div>
  );
}
