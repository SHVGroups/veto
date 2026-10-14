"use client";

import { useQueryClient } from "@tanstack/react-query";
import Link from "next/link";
import type { Route } from "next";
import { useState } from "react";
import { ReliabilityChart } from "@/components/charts/distribution";
import { Calibrating } from "@/components/illustrations";
import { useCan } from "@/components/shell/session";
import { Button } from "@/components/ui/button";
import { Segmented, Select } from "@/components/ui/form";
import { PageHeader, Panel, PanelHeader } from "@/components/ui/layout";
import { RiskTag } from "@/components/ui/pills";
import { EmptyState, ErrorState, LoadingPanel, Notice } from "@/components/ui/states";
import { Table, Td, Th } from "@/components/ui/table";
import { useToast } from "@/components/ui/toast";
import { api, errorMessage, toolInput, toolPath } from "@/lib/api";
import { formatCount, formatPercent, formatProbability } from "@/lib/format";
import { useCalibration } from "@/lib/queries";
import type { Range, Thresholds, Tool, ToolCalibration } from "@/lib/types";

export function CalibrationPage() {
  const [range, setRange] = useState<Range>("30d");
  const [tool, setTool] = useState("");
  const q = useCalibration(range, tool);
  const c = q.data;
  const selected = c?.tools.find((t) => t.tool === tool);
  const bands = selected ? selected.bands : (c?.overall ?? []);
  const labelled = bands.reduce((s, b) => s + b.labeled, 0);

  return (
    <>
      <PageHeader
        title="Calibration"
        description="Whether the decision model's confidence can be trusted. Every review and audit verdict is a label; Veto compares labels with what the model chose at each confidence."
        actions={
          <>
            <Select aria-label="Tool" value={tool} onChange={(e) => setTool(e.target.value)} className="w-56">
              <option value="">All tools</option>
              {c?.tools.map((t) => (
                <option key={t.tool} value={t.tool}>
                  {t.tool}
                </option>
              ))}
            </Select>
            <Segmented
              label="Range"
              value={range}
              onChange={setRange}
              options={(["7d", "30d", "90d"] as Range[]).map((r) => ({ value: r, label: r }))}
            />
          </>
        }
      />
      {q.isError && <ErrorState error={q.error} onRetry={() => void q.refetch()} className="mb-5" />}
      {!c ? (
        <Panel>
          <LoadingPanel rows={8} />
        </Panel>
      ) : (
        <div className="space-y-5">
          <Panel>
            <PanelHeader
              title={selected ? `Reliability for ${selected.tool}` : "Reliability across all tools"}
              description={`${formatCount(labelled)} labelled decisions. Filled points are reliably at or above the ${formatPercent(c.target, 0)} target; hollow points are above it but with too few labels to be sure; red points are below it.`}
            />
            {labelled === 0 ? (
              <EmptyState art={<Calibrating />} title="No labels in this range">
                Labels come from reviewers deciding held actions and from audit verdicts. Label a few audits to start measuring.
              </EmptyState>
            ) : (
              <ReliabilityChart
                bands={bands}
                target={c.target}
                threshold={selected?.current.auto_proceed ? selected.current.auto_confidence : undefined}
                suggested={selected?.suggested}
              />
            )}
          </Panel>

          <Panel>
            <PanelHeader
              title="Thresholds by tool"
              description={`A suggestion is the lowest confidence where agreement with reviewers stays at or above the target with 95% certainty, once a tool has at least ${c.min_n} labels.`}
            />
            {c.tools.length === 0 ? (
              <p className="py-6 text-sm text-ink-3">No tools have labelled decisions in this range.</p>
            ) : (
              <Table>
                <thead>
                  <tr>
                    <Th>Tool</Th>
                    <Th>Risk</Th>
                    <Th className="text-right">Labels</Th>
                    <Th>Agreement</Th>
                    <Th className="text-right">Runs above</Th>
                    <Th className="text-right">Suggested</Th>
                    <Th />
                  </tr>
                </thead>
                <tbody>
                  {c.tools.map((t) => (
                    <ToolRow key={t.tool} t={t} onInspect={() => setTool(t.tool)} />
                  ))}
                </tbody>
              </Table>
            )}
          </Panel>
          <Notice title="How to read this">
            Raising a tool&rsquo;s threshold sends more of its calls to people; lowering it lets the model decide more. Change thresholds on
            the tool&rsquo;s page, or apply a suggestion here. Every change is recorded in the ledger.
          </Notice>
        </div>
      )}
    </>
  );
}

function ToolRow({ t, onInspect }: { t: ToolCalibration; onInspect: () => void }) {
  const qc = useQueryClient();
  const toast = useToast();
  const canConfigure = useCan("configure");
  const [busy, setBusy] = useState(false);

  async function apply() {
    if (t.suggested === null) return;
    setBusy(true);
    try {
      const cur = await api.get<{ tool: Tool; effective: Thresholds }>(`/v1/tools/${toolPath(t.tool)}`);
      await api.put(`/v1/tools/${toolPath(t.tool)}`, { ...toolInput(cur.tool), auto_confidence: t.suggested });
      toast.success(`${t.tool} now runs above ${formatProbability(t.suggested)}`);
      void qc.invalidateQueries({ queryKey: ["calibration"] });
      void qc.invalidateQueries({ queryKey: ["tools"] });
    } catch (err) {
      toast.error(errorMessage(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <tr>
      <Td>
        <Link href={`/tools/${t.tool}` as Route} className="font-medium text-ink hover:underline">
          {t.tool}
        </Link>
        {t.note && <div className="max-w-md text-xs text-ink-3">{t.note}</div>}
      </Td>
      <Td>
        <RiskTag risk={t.risk_class} />
      </Td>
      <Td className="figures text-right text-ink-2">{formatCount(t.labeled)}</Td>
      <Td className="figures text-ink-2">
        {t.agreement ? (
          <>
            {formatPercent(t.agreement.value)}
            <span className="text-xs text-ink-3">
              {" "}
              ({formatPercent(t.agreement.low)} to {formatPercent(t.agreement.high)})
            </span>
          </>
        ) : (
          "–"
        )}
      </Td>
      <Td className="figures text-right text-ink-2">{t.current.auto_proceed ? formatProbability(t.current.auto_confidence) : "Never"}</Td>
      <Td className="figures text-right font-medium text-held">{formatProbability(t.suggested)}</Td>
      <Td className="text-right whitespace-nowrap">
        <Button size="sm" variant="ghost" onClick={onInspect}>
          Inspect
        </Button>
        {canConfigure && t.suggested !== null && t.suggested !== t.current.auto_confidence && (
          <Button size="sm" busy={busy} onClick={() => void apply()} className="ml-1">
            Apply
          </Button>
        )}
      </Td>
    </tr>
  );
}
