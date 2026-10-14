"use client";

import { useQueryClient } from "@tanstack/react-query";
import Link from "next/link";
import type { Route } from "next";
import { useState } from "react";
import { Icon } from "@/components/icons";
import { Calibrating } from "@/components/illustrations";
import { useCan } from "@/components/shell/session";
import { Button } from "@/components/ui/button";
import { JsonView } from "@/components/ui/code";
import { Textarea } from "@/components/ui/form";
import { PageHeader, Panel } from "@/components/ui/layout";
import { DecidedByText, OptionTag, OutcomeTag, Pill } from "@/components/ui/pills";
import { EmptyState, ErrorState, LoadingPanel } from "@/components/ui/states";
import { Table, Tabs, Td, Th } from "@/components/ui/table";
import { useToast } from "@/components/ui/toast";
import { api, errorMessage } from "@/lib/api";
import { cx } from "@/lib/cx";
import { auditVerdictLabel, formatProbability, formatRelative, formatTimestamp } from "@/lib/format";
import { useAudits } from "@/lib/queries";
import type { AuditItem, AuditVerdict } from "@/lib/types";

const VERDICTS: { value: AuditVerdict; label: string; tone: string }[] = [
  { value: "correct", label: "Correct", tone: "proceed" },
  { value: "should_be_safer", label: "Should have been the safer version", tone: "safer" },
  { value: "should_hold", label: "Should have waited for a person", tone: "held" },
  { value: "should_block", label: "Should have been stopped", tone: "stop" },
];

export function Audits() {
  const [tab, setTab] = useState<"open" | "reviewed">("open");
  const open = useAudits("open");
  const reviewed = useAudits("reviewed");
  const q = tab === "open" ? open : reviewed;
  return (
    <>
      <PageHeader
        title="Audits"
        description="A sample of decisions made without a person, plus any whose outcome went wrong. Your verdicts measure how far automatic decisions can be trusted and feed calibration."
      />
      <Tabs
        className="mb-5"
        value={tab}
        onChange={setTab}
        tabs={[
          { value: "open", label: "To label", count: open.data?.items.length },
          { value: "reviewed", label: "Labelled", count: reviewed.data?.items.length },
        ]}
      />
      {q.isError && <ErrorState error={q.error} onRetry={() => void q.refetch()} />}
      {q.isPending ? (
        <Panel>
          <LoadingPanel />
        </Panel>
      ) : tab === "open" ? (
        open.data!.items.length === 0 ? (
          <Panel>
            <EmptyState art={<Calibrating />} title="Nothing to label">
              Veto samples a share of automatic decisions, set in Settings, and queues any decision whose outcome is reported as failed,
              reversed or a complaint.
            </EmptyState>
          </Panel>
        ) : (
          <div className="space-y-4">
            {open.data!.items.map((a) => (
              <AuditCard key={a.id} a={a} />
            ))}
          </div>
        )
      ) : (
        <Panel>
          {reviewed.data!.items.length === 0 ? (
            <EmptyState art={<Calibrating />} title="No labels yet">
              Labelled audits appear here with who labelled them and when.
            </EmptyState>
          ) : (
            <Table>
              <thead>
                <tr>
                  <Th>Action</Th>
                  <Th>Decision</Th>
                  <Th>Verdict</Th>
                  <Th>Labelled by</Th>
                  <Th>Note</Th>
                  <Th className="text-right">When</Th>
                </tr>
              </thead>
              <tbody>
                {reviewed.data!.items.map((a) => (
                  <tr key={a.id}>
                    <Td>
                      <Link href={`/actions/${a.action_id}` as Route} className="font-medium text-ink hover:underline">
                        {a.tool}
                      </Link>
                      <div className="text-xs text-ink-3">{a.agent_name}</div>
                    </Td>
                    <Td>
                      <OptionTag option={a.option} />
                    </Td>
                    <Td>
                      <Pill tone={a.verdict === "correct" ? "proceed" : "stop"}>{a.verdict ? auditVerdictLabel[a.verdict] : "–"}</Pill>
                    </Td>
                    <Td className="text-ink-2">{a.reviewer_name ?? "–"}</Td>
                    <Td className="max-w-72 truncate text-ink-2">{a.note || "–"}</Td>
                    <Td className="text-right whitespace-nowrap text-ink-3" title={formatTimestamp(a.reviewed_at)}>
                      {formatRelative(a.reviewed_at)}
                    </Td>
                  </tr>
                ))}
              </tbody>
            </Table>
          )}
        </Panel>
      )}
    </>
  );
}

function AuditCard({ a }: { a: AuditItem }) {
  const qc = useQueryClient();
  const toast = useToast();
  const canReview = useCan("review");
  const [verdict, setVerdict] = useState<AuditVerdict | null>(null);
  const [note, setNote] = useState("");
  const [busy, setBusy] = useState(false);
  const [showArgs, setShowArgs] = useState(false);

  async function save() {
    if (!verdict) return;
    setBusy(true);
    try {
      await api.post(`/v1/audits/${a.id}`, { verdict, note });
      toast.success("Verdict saved");
      void qc.invalidateQueries({ queryKey: ["audits"] });
      void qc.invalidateQueries({ queryKey: ["calibration"] });
      void qc.invalidateQueries({ queryKey: ["overview"] });
    } catch (err) {
      toast.error(errorMessage(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <Panel>
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2.5">
            <Link href={`/actions/${a.action_id}` as Route} className="text-lg font-medium text-ink hover:underline">
              {a.tool}
            </Link>
            <OptionTag option={a.option} />
            {a.outcome && <OutcomeTag outcome={a.outcome} />}
          </div>
          <p className="mt-1 text-sm text-ink-3">
            {a.agent_name}, {formatRelative(a.received_at)}.{" "}
            {a.reason === "outcome" ? "Queued because the outcome went wrong." : "Picked by random sampling."}
          </p>
        </div>
        <div className="figures flex gap-6 text-sm">
          <div>
            <div className="text-xs text-ink-3">Decided by</div>
            <DecidedByText by={a.decided_by} />
          </div>
          <div>
            <div className="text-xs text-ink-3">Confidence</div>
            {formatProbability(a.confidence)}
          </div>
          <div>
            <div className="text-xs text-ink-3">Risk</div>
            {formatProbability(a.risk)}
          </div>
        </div>
      </div>
      {a.request && <blockquote className="mt-4 border-l-2 border-line-strong pl-4 text-ink">&ldquo;{a.request}&rdquo;</blockquote>}
      <button
        type="button"
        onClick={() => setShowArgs((s) => !s)}
        className="mt-4 flex items-center gap-1 text-xs text-ink-3 hover:text-ink"
      >
        <Icon.ChevronRight size={13} className={cx("transition-transform", showArgs && "rotate-90")} />
        What ran: <code className="text-ink-2">{a.final_tool || "nothing"}</code>
      </button>
      {showArgs && a.final_args !== null && <JsonView className="mt-2" value={a.final_args} maxHeight={240} />}

      {canReview && (
        <div className="mt-5 border-t border-line pt-5">
          <div className="mb-3 text-sm font-medium text-ink">Was this the right call?</div>
          <div className="flex flex-wrap gap-2" role="radiogroup" aria-label="Verdict">
            {VERDICTS.map((v) => (
              <button
                key={v.value}
                type="button"
                role="radio"
                aria-checked={verdict === v.value}
                onClick={() => setVerdict(v.value)}
                className={cx(
                  "h-9 rounded-full px-3.5 text-sm transition-colors",
                  verdict === v.value
                    ? {
                        proceed: "bg-proceed text-white",
                        safer: "bg-safer text-white",
                        held: "bg-held text-white",
                        stop: "bg-stop text-white",
                      }[v.tone]
                    : "bg-sunken text-ink-2 shadow-[inset_0_0_0_1px_var(--line)] hover:text-ink",
                )}
              >
                {v.label}
              </button>
            ))}
          </div>
          {verdict && (
            <div className="mt-4 flex flex-col gap-3 sm:flex-row sm:items-end">
              <Textarea
                rows={1}
                placeholder="Note, optional"
                value={note}
                onChange={(e) => setNote(e.target.value)}
                className="flex-1"
                aria-label="Note"
              />
              <Button variant="primary" busy={busy} onClick={() => void save()}>
                Save verdict
              </Button>
            </div>
          )}
        </div>
      )}
    </Panel>
  );
}
