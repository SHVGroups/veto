"use client";

import { useMemo, useState } from "react";
import { decidedByLabel, formatCount, formatPercent } from "@/lib/format";
import type { DecidedBy, Flow, Option } from "@/lib/types";
import { ChartTooltip } from "./tooltip";
import { useWidth } from "./use-size";

type Deciders = DecidedBy | "pending";
type Results = Option | "pending";

const DECIDER_ORDER: Deciders[] = ["policy", "model", "human", "failsafe", "expiry", "pending"];
const RESULT_ORDER: Results[] = ["proceed", "proceed_safer", "stop", "pending"];

const resultLabel: Record<Results, string> = {
  proceed: "Ran as proposed",
  proceed_safer: "Safer version",
  stop: "Stopped",
  pending: "Waiting",
};

const resultColor: Record<Results, { stroke: string; wash: string; text: string }> = {
  proceed: { stroke: "var(--proceed)", wash: "var(--proceed-wash)", text: "var(--proceed)" },
  proceed_safer: { stroke: "var(--safer)", wash: "var(--safer-wash)", text: "var(--safer)" },
  stop: { stroke: "var(--stop)", wash: "var(--stop-wash)", text: "var(--stop)" },
  pending: { stroke: "var(--held)", wash: "var(--held-wash)", text: "var(--held)" },
};

const deciderStyle = (d: Deciders) =>
  d === "human" || d === "pending" ? { wash: "var(--held-wash)", text: "var(--held)" } : { wash: "var(--sunken)", text: "var(--ink)" };

interface Node<K> {
  key: K;
  count: number;
  y: number;
  h: number;
}

/** Stacks nodes in a column. Every node gets room for its label; the rest is shared by count. */
function layout<K>(keys: K[], counts: Map<K, number>, height: number, gap: number, minH: number): Node<K>[] {
  const present = keys.filter((k) => (counts.get(k) ?? 0) > 0);
  const total = present.reduce((s, k) => s + (counts.get(k) ?? 0), 0) || 1;
  const free = Math.max(0, height - gap * (present.length - 1) - minH * present.length);
  let y = 0;
  return present.map((k) => {
    const c = counts.get(k) ?? 0;
    const h = minH + (free * c) / total;
    const node = { key: k, count: c, y, h };
    y += h + gap;
    return node;
  });
}

function band(x0: number, y0a: number, y0b: number, x1: number, y1a: number, y1b: number) {
  const xm = (x0 + x1) / 2;
  return `M${x0},${y0a} C${xm},${y0a} ${xm},${y1a} ${x1},${y1a} L${x1},${y1b} C${xm},${y1b} ${xm},${y0b} ${x0},${y0b} Z`;
}

/**
 * How every action in the range moved through Veto: who or what decided it,
 * and which option it ended on. Band widths are proportional to counts.
 */
export function DecisionFlow({ flows, height = 300 }: { flows: Flow[]; height?: number }) {
  const [ref, width] = useWidth<HTMLDivElement>();
  const [hover, setHover] = useState<{ from: Deciders | "all"; to: Results | Deciders; x: number; y: number } | null>(null);

  const data = useMemo(() => {
    const byDecider = new Map<Deciders, number>();
    const byResult = new Map<Results, number>();
    const pair = new Map<string, number>();
    let total = 0;
    for (const f of flows) {
      // A stop decided by expiry is shown as stopped; "pending" is its own result.
      const r: Results = f.option;
      byDecider.set(f.decided_by, (byDecider.get(f.decided_by) ?? 0) + f.count);
      byResult.set(r, (byResult.get(r) ?? 0) + f.count);
      pair.set(`${f.decided_by}|${r}`, (pair.get(`${f.decided_by}|${r}`) ?? 0) + f.count);
      total += f.count;
    }
    return { byDecider, byResult, pair, total };
  }, [flows]);

  const compact = width < 600;
  const gap = 10;
  const minH = compact ? 34 : 40;
  const blockL = compact ? 84 : 112;
  const blockM = compact ? 104 : 148;
  const blockR = compact ? 104 : 148;
  const top = 26;
  const innerH = height - top;

  const deciders = layout(DECIDER_ORDER, data.byDecider, innerH, gap, minH);
  const results = layout(RESULT_ORDER, data.byResult, innerH, gap, minH);
  // The received block is as tall as the deciders' blocks without their gaps, centred.
  const leftH = deciders.reduce((s, n) => s + n.h, 0);
  const leftY = (innerH - leftH) / 2;

  const xL = 0;
  const xM = Math.round((width - blockM) / 2);
  const xR = width - blockR;

  // Left to middle: the received block splits into one band per decider.
  const leftBands: { d: Node<Deciders>; path: string }[] = [];
  for (let i = 0, acc = leftY; i < deciders.length; i++) {
    const d = deciders[i]!;
    leftBands.push({ d, path: band(xL + blockL, top + acc, top + acc + d.h, xM, top + d.y, top + d.y + d.h) });
    acc += d.h;
  }

  // Middle to right: each decider splits by result; each result stacks its inbound bands.
  const resultFill = new Map<Results, number>(results.map((r) => [r.key, r.y]));
  const rightBands: { from: Deciders; to: Results; count: number; path: string }[] = [];
  for (const d of deciders) {
    let accM = d.y;
    for (const r of results) {
      const c = data.pair.get(`${d.key}|${r.key}`) ?? 0;
      if (!c) continue;
      const hm = (d.h * c) / d.count;
      const hr = (r.h * c) / r.count;
      const yr = resultFill.get(r.key) ?? r.y;
      rightBands.push({
        from: d.key,
        to: r.key,
        count: c,
        path: band(xM + blockM, top + accM, top + accM + hm, xR, top + yr, top + yr + hr),
      });
      accM += hm;
      resultFill.set(r.key, yr + hr);
    }
  }

  if (data.total === 0) {
    return (
      <div ref={ref} className="flex h-40 items-center justify-center text-sm text-ink-3">
        No actions in this range yet.
      </div>
    );
  }

  const dim = (from: Deciders | "all", to: Results | Deciders) => (hover && !(hover.from === from && hover.to === to) ? 0.35 : 1);

  return (
    <div ref={ref} className="relative" onMouseLeave={() => setHover(null)}>
      <svg width={width} height={height} role="img" aria-label="How actions were decided and what they ended as">
        <text x={xL} y={14} className="fill-ink-3 text-[12px]">
          Received
        </text>
        <text x={xM} y={14} className="fill-ink-3 text-[12px]">
          Decided by
        </text>
        <text x={xR} y={14} className="fill-ink-3 text-[12px]">
          Result
        </text>

        {leftBands.map(({ d, path }) => (
          <path
            key={d.key}
            d={path}
            fill="var(--line-strong)"
            opacity={0.55 * dim("all", d.key)}
            onMouseMove={(e) => setHover({ from: "all", to: d.key, x: e.nativeEvent.offsetX, y: e.nativeEvent.offsetY })}
          />
        ))}
        {rightBands.map((b) => (
          <path
            key={`${b.from}-${b.to}`}
            d={b.path}
            fill={resultColor[b.to].stroke}
            opacity={0.26 * dim(b.from, b.to) + (hover?.from === b.from && hover.to === b.to ? 0.2 : 0)}
            onMouseMove={(e) => setHover({ from: b.from, to: b.to, x: e.nativeEvent.offsetX, y: e.nativeEvent.offsetY })}
          />
        ))}

        <g>
          <rect x={xL} y={top + leftY} width={blockL} height={leftH} rx={12} fill="var(--ink)" />
          <text x={xL + 14} y={top + leftY + 24} className="fill-surface text-[12px]">
            All actions
          </text>
          <text x={xL + 14} y={top + leftY + 50} className="numeral fill-surface text-[24px]">
            {formatCount(data.total)}
          </text>
        </g>

        {deciders.map((d) => {
          const s = deciderStyle(d.key);
          return (
            <g key={d.key}>
              <rect x={xM} y={top + d.y} width={blockM} height={d.h} rx={10} fill={s.wash} />
              <NodeLabel
                x={xM + 12}
                y={top + d.y}
                h={d.h}
                label={decidedByLabel[d.key]}
                count={d.count}
                total={data.total}
                color={s.text}
              />
            </g>
          );
        })}
        {results.map((r) => (
          <g key={r.key}>
            <rect x={xR} y={top + r.y} width={blockR} height={r.h} rx={10} fill={resultColor[r.key].wash} />
            <rect x={xR} y={top + r.y} width={3} height={r.h} rx={1.5} fill={resultColor[r.key].stroke} />
            <NodeLabel
              x={xR + 14}
              y={top + r.y}
              h={r.h}
              label={resultLabel[r.key]}
              count={r.count}
              total={data.total}
              color={resultColor[r.key].text}
            />
          </g>
        ))}
      </svg>
      {hover && (
        <ChartTooltip x={hover.x} y={hover.y} width={width}>
          <FlowTooltip hover={hover} data={data} />
        </ChartTooltip>
      )}
    </div>
  );
}

function NodeLabel({
  x,
  y,
  h,
  label,
  count,
  total,
  color,
}: {
  x: number;
  y: number;
  h: number;
  label: string;
  count: number;
  total: number;
  color: string;
}) {
  const twoLines = h >= 44;
  return twoLines ? (
    <>
      <text x={x} y={y + 18} className="text-[12px]" fill="var(--ink-2)">
        {label}
      </text>
      <text x={x} y={y + 36} className="figures text-[14px] font-semibold" fill={color}>
        {formatCount(count)}
        <tspan dx={6} className="text-[11px] font-normal" fill="var(--ink-3)">
          {formatPercent(count / total)}
        </tspan>
      </text>
    </>
  ) : (
    <text x={x} y={y + h / 2} dy="0.35em" className="text-[12px]" fill="var(--ink-2)">
      {label}
      <tspan dx={6} className="figures font-semibold" fill={color}>
        {formatCount(count)}
      </tspan>
    </text>
  );
}

function FlowTooltip({
  hover,
  data,
}: {
  hover: { from: Deciders | "all"; to: Results | Deciders };
  data: { byDecider: Map<Deciders, number>; pair: Map<string, number>; total: number };
}) {
  if (hover.from === "all") {
    const c = data.byDecider.get(hover.to as Deciders) ?? 0;
    return (
      <>
        <div className="font-medium text-ink">{decidedByLabel[hover.to as Deciders]}</div>
        <div className="mt-1 text-ink-2">
          {formatCount(c)} of {formatCount(data.total)} actions ({formatPercent(c / data.total)})
        </div>
      </>
    );
  }
  const c = data.pair.get(`${hover.from}|${hover.to}`) ?? 0;
  const from = data.byDecider.get(hover.from) ?? 1;
  return (
    <>
      <div className="font-medium text-ink">
        {decidedByLabel[hover.from]} to {resultLabel[hover.to as Results].toLowerCase()}
      </div>
      <div className="mt-1 text-ink-2">
        {formatCount(c)} actions, {formatPercent(c / from)} of what {decidedByLabel[hover.from].toLowerCase()} decided
      </div>
    </>
  );
}
