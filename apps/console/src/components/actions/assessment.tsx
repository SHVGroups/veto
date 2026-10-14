"use client";

import { cx } from "@/lib/cx";
import { formatProbability, humanize, optionPhrase, optionShort } from "@/lib/format";
import { useMeta } from "@/lib/queries";
import type { Assessment, Meta, Option, SignalValue, Thresholds, Tool } from "@/lib/types";
import { Gauge, Triad } from "../charts/triad";
import { Icon } from "../icons";
import { Pill } from "../ui/pills";

/** A tool's thresholds after its overrides are applied to its risk class's defaults. */
export function effectiveThresholds(tool: Tool | null, meta: Meta | undefined): Thresholds | null {
  if (!tool || !meta) return null;
  const base = meta.risk_classes.find((r) => r.name === tool.risk_class)?.thresholds;
  if (!base) return null;
  return {
    auto_confidence: tool.auto_confidence ?? base.auto_confidence,
    risk_ceiling: tool.risk_ceiling ?? base.risk_ceiling,
    block_confidence: tool.block_confidence ?? base.block_confidence,
    double_check: tool.double_check ?? base.double_check,
    auto_proceed: base.auto_proceed,
  };
}

export function useEffectiveThresholds(tool: Tool | null, decided?: Thresholds | null): Thresholds | null {
  const meta = useMeta();
  return decided ?? effectiveThresholds(tool, meta.data);
}

/**
 * The decision model's view of an action: how likely each option was, how
 * confident the pick, how risky the call, and every signal behind the risk.
 */
export function AssessmentView({
  a,
  thresholds,
  offered,
  compact = false,
}: {
  a: Assessment;
  thresholds: Thresholds | null;
  offered: Option[];
  compact?: boolean;
}) {
  const confMarks = thresholds
    ? a.choice === "stop"
      ? [{ at: thresholds.block_confidence, label: "Stops above" }]
      : thresholds.auto_proceed
        ? [{ at: thresholds.auto_confidence, label: "Runs above" }]
        : []
    : [];
  const signals = Object.entries(a.signals).sort((x, y) => y[1].risk * y[1].weight - x[1].risk * x[1].weight);

  return (
    <div className="space-y-6">
      <Triad probabilities={a.probabilities} choice={a.choice} offered={offered} />

      <div className="grid gap-6 sm:grid-cols-2">
        <Gauge label={`Confidence in ${optionShort[a.choice].toLowerCase()}`} value={a.confidence} marks={confMarks} />
        <Gauge
          label="Risk"
          value={a.risk}
          tone={thresholds && a.risk > thresholds.risk_ceiling ? "stop" : "ink"}
          marks={thresholds ? [{ at: thresholds.risk_ceiling, label: "Ceiling" }] : []}
        />
      </div>

      {(a.guards.length > 0 || a.recheck) && (
        <div className="space-y-2">
          {a.guards.map((g) => (
            <div key={g.signal} className="flex items-start gap-2.5 rounded-[12px] bg-stop-wash px-3.5 py-2.5 text-sm">
              <Icon.Alert size={16} className="mt-0.5 shrink-0 text-stop" />
              <div>
                <span className="font-medium text-ink">{g.reason}</span>
                <span className="text-ink-2">
                  {" "}
                  Signal {humanize(g.signal).toLowerCase()} at {formatProbability(g.value)} rules out{" "}
                  {g.forbid.map((o) => optionPhrase[o]).join(" and ")}.
                </span>
              </div>
            </div>
          ))}
          {a.recheck && (
            <div
              className={cx(
                "flex items-start gap-2.5 rounded-[12px] px-3.5 py-2.5 text-sm",
                a.recheck.agrees ? "bg-sunken shadow-[inset_0_0_0_1px_var(--line)]" : "bg-safer-wash",
              )}
            >
              <Icon.Refresh size={16} className={cx("mt-0.5 shrink-0", a.recheck.agrees ? "text-ink-3" : "text-safer")} />
              <span className="text-ink-2">
                Asked again with the options in reverse order, the model chose{" "}
                <span className="font-medium text-ink">{optionShort[a.recheck.choice].toLowerCase()}</span> at{" "}
                {formatProbability(a.recheck.confidence)}.{" "}
                {a.recheck.agrees ? "The answer did not depend on the order." : "The answer changed with the order, so a person decides."}
              </span>
            </div>
          )}
        </div>
      )}

      {!compact && (
        <div>
          <div className="mb-2 flex items-baseline justify-between">
            <h3 className="text-sm text-ink">Signals</h3>
            <span className="text-xs text-ink-3">
              Pack {a.pack} v{a.pack_version}
            </span>
          </div>
          <SignalTable signals={signals} />
          {a.skipped && a.skipped.length > 0 && (
            <p className="mt-2 text-xs text-ink-3">
              Not asked because their inputs were missing: {a.skipped.map((s) => humanize(s).toLowerCase()).join(", ")}.
            </p>
          )}
        </div>
      )}
    </div>
  );
}

function answerText(s: SignalValue): string {
  switch (s.type) {
    case "noul":
      return `${formatProbability(s.value)} yes`;
    case "choice":
      return s.choice ? humanize(s.choice) : formatProbability(s.value);
    case "score": {
      const level = s.answer.score !== undefined ? s.levels?.[Math.round(s.answer.score)] : undefined;
      return level ?? formatProbability(s.value);
    }
  }
}

function SignalTable({ signals }: { signals: [string, SignalValue][] }) {
  if (!signals.length) return <p className="text-sm text-ink-3">This pack asked no signal questions.</p>;
  const maxContribution = Math.max(0.0001, ...signals.map(([, s]) => s.risk * s.weight));
  return (
    <div className="overflow-hidden rounded-[12px] shadow-[inset_0_0_0_1px_var(--line)]">
      <table className="w-full text-sm">
        <thead>
          <tr className="bg-sunken text-left text-xs text-ink-3">
            <th className="px-3 py-2 font-medium">Question</th>
            <th className="px-3 py-2 font-medium">Answer</th>
            <th className="hidden px-3 py-2 font-medium sm:table-cell">Weight</th>
            <th className="w-[34%] px-3 py-2 font-medium">Adds to risk</th>
          </tr>
        </thead>
        <tbody>
          {signals.map(([id, s]) => {
            const contribution = s.risk * s.weight;
            return (
              <tr key={id} className="border-t border-line">
                <td className="px-3 py-2 text-ink">
                  {humanize(id)}
                  {s.confidence !== undefined && <span className="ml-1.5 text-xs text-ink-3">conf. {formatProbability(s.confidence)}</span>}
                </td>
                <td className="figures px-3 py-2 text-ink-2">{answerText(s)}</td>
                <td className="figures hidden px-3 py-2 text-ink-3 sm:table-cell">{s.weight ? formatProbability(s.weight) : "–"}</td>
                <td className="px-3 py-2">
                  {s.weight ? (
                    <div className="flex items-center gap-2">
                      <div className="h-1.5 flex-1 overflow-hidden rounded-full bg-hover">
                        <div
                          className={cx("h-full rounded-full", s.risk >= 0.5 ? "bg-stop" : "bg-ink-2")}
                          style={{ width: `${(contribution / maxContribution) * 100}%` }}
                        />
                      </div>
                      <span className="figures w-9 text-right text-xs text-ink-2">{contribution.toFixed(2)}</span>
                    </div>
                  ) : (
                    <Pill tone="muted">Guard only</Pill>
                  )}
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}
