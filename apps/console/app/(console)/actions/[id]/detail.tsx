"use client";

import Link from "next/link";
import type { Route } from "next";
import { useState } from "react";
import { AssessmentView, useEffectiveThresholds } from "@/components/actions/assessment";
import { ActionContext } from "@/components/actions/context";
import { Deadline } from "@/components/actions/deadline";
import { ReasonList } from "@/components/actions/reasons";
import { ReviewPanel } from "@/components/actions/review-panel";
import { Icon } from "@/components/icons";
import { CopyButton, JsonDiff, JsonView, Mono } from "@/components/ui/code";
import { Details, PageHeader, Panel, PanelHeader } from "@/components/ui/layout";
import { DecisionChip, OptionTag, OutcomeTag, Pill } from "@/components/ui/pills";
import { ErrorState, LoadingPanel, Notice } from "@/components/ui/states";
import { Tabs } from "@/components/ui/table";
import { cx } from "@/lib/cx";
import {
  auditVerdictLabel,
  decidedByLabel,
  formatDuration,
  formatMs,
  formatProbability,
  formatTimestamp,
  humanize,
  ledgerKindLabel,
  optionLabel,
  shortHash,
} from "@/lib/format";
import { useActionDetail } from "@/lib/queries";
import type { ActionDetail, Evaluation, LedgerEntry, Option } from "@/lib/types";

export function ActionDetailView({ id }: { id: string }) {
  const q = useActionDetail(id);
  const d = q.data;
  const thresholds = useEffectiveThresholds(d?.tool ?? null, d?.decision?.thresholds);

  if (q.isError) return <ErrorState error={q.error} onRetry={() => void q.refetch()} />;
  if (!d) return <LoadingPanel rows={10} />;

  const a = d.action;
  const offered = a.options.filter((o) => o.available).map((o) => o.key);
  const pending = a.status === "pending_review";

  return (
    <>
      <PageHeader
        meta={
          <Link href="/actions" className="inline-flex items-center gap-1 hover:text-ink">
            <Icon.ChevronLeft size={14} />
            Actions
          </Link>
        }
        title={
          <span className="flex flex-wrap items-center gap-3">
            {a.tool}
            <DecisionChip status={a.status} option={d.decision?.option ?? null} />
          </span>
        }
        description={
          <span className="inline-flex items-center gap-1.5">
            <Mono copy={a.id}>{a.id}</Mono>
          </span>
        }
        actions={<CopyButton value={typeof window === "undefined" ? "" : window.location.href} label="Copy link to this action" />}
      />

      {a.redacted_at && (
        <Notice className="mb-5" title="Arguments and request text were removed by the retention policy">
          Redacted {formatTimestamp(a.redacted_at)}. Hashes in the ledger still prove what was decided.
        </Notice>
      )}

      <div className="grid items-start gap-5 xl:grid-cols-[minmax(0,1fr)_380px]">
        <div className="min-w-0 space-y-5">
          <Panel>
            <PanelHeader title="What the agent wanted to do" />
            <ActionContext detail={d} />
          </Panel>

          {pending && (
            <Panel>
              <PanelHeader
                title="Waiting for a decision"
                actions={<Deadline receivedAt={a.received_at} deadline={a.review_deadline} bar className="w-52" />}
              />
              <div className="mb-5 rounded-[14px] bg-held-wash px-4 py-3">
                <div className="text-xs font-medium text-held">Why Veto is asking a person</div>
                <ReasonList reasons={a.hold_reasons} className="mt-1 space-y-0.5" />
              </div>
              <ReviewPanel key={a.id} detail={d} keyboard />
            </Panel>
          )}

          {d.decision && <DecisionPanel d={d} />}

          {d.assessment && (
            <Panel>
              <PanelHeader
                title="Decision model"
                description="The model chose among the options offered for this call. Its answers are probabilities, not text."
              />
              <AssessmentView a={d.assessment} thresholds={thresholds} offered={offered} />
            </Panel>
          )}

          {!pending && <OptionsPanel d={d} />}

          <EvaluationsPanel evaluations={d.evaluations} />
        </div>

        <div className="space-y-5 xl:sticky xl:top-6">
          <Panel>
            <PanelHeader title="Timeline" />
            <Timeline entries={d.ledger} />
          </Panel>
          {(d.grants.length > 0 || d.outcomes.length > 0 || d.audit) && <AfterwardsPanel d={d} />}
          <Panel>
            <PanelHeader title="Integrity" description="Hashes that let anyone verify this record was not changed." />
            <Details
              rows={[
                [
                  "Arguments hash",
                  <Mono key="h" copy={d.args_hash} title={d.args_hash}>
                    {shortHash(d.args_hash, 10)}
                  </Mono>,
                ],
                ["Policy version", a.policy_version ? `v${a.policy_version}` : "–"],
                ["Ledger entries", String(d.ledger.length)],
                [
                  "",
                  <Link
                    key="l"
                    href={`/ledger?action=${a.id}` as Route}
                    className="inline-flex items-center gap-1 text-sm text-ink-2 hover:text-ink"
                  >
                    Open in ledger
                    <Icon.ChevronRight size={13} />
                  </Link>,
                ],
              ]}
            />
          </Panel>
        </div>
      </div>
    </>
  );
}

function DecisionPanel({ d }: { d: ActionDetail }) {
  const dec = d.decision!;
  const by =
    dec.decided_by === "human"
      ? `${dec.reviewer_name ?? "A reviewer"}${dec.review_seconds !== null ? `, after ${formatDuration(dec.review_seconds)}` : ""}`
      : decidedByLabel[dec.decided_by];
  const changedTool = dec.final_tool && dec.final_tool !== d.action.tool;
  return (
    <Panel>
      <PanelHeader
        title="Decision"
        description={`${by}, ${formatTimestamp(dec.created_at)}.`}
        actions={<OptionTag option={dec.option}>{optionLabel[dec.option]}</OptionTag>}
      />
      <div className="grid gap-6 lg:grid-cols-2">
        <div className="space-y-4">
          <div>
            <div className="mb-1.5 text-xs text-ink-3">Reasons</div>
            <ReasonList reasons={dec.reasons} className="space-y-0.5" />
          </div>
          {dec.note && (
            <div>
              <div className="mb-1 text-xs text-ink-3">Note</div>
              <p className="text-sm text-ink">{dec.note}</p>
            </div>
          )}
          <Details
            rows={[
              ["Confidence", formatProbability(dec.confidence)],
              ["Risk", formatProbability(dec.risk)],
              [
                "Model's pick",
                dec.model_choice ? (
                  <span key="m">
                    {optionLabel[dec.model_choice]} at {formatProbability(dec.model_confidence)}
                  </span>
                ) : (
                  "Not asked"
                ),
              ],
              [
                "Thresholds",
                <span key="t" className="figures text-ink-2">
                  runs above {formatProbability(dec.thresholds.auto_confidence)}, risk ceiling{" "}
                  {formatProbability(dec.thresholds.risk_ceiling)}, stops above {formatProbability(dec.thresholds.block_confidence)}
                </span>,
              ],
            ]}
          />
        </div>
        <div>
          {dec.option === "stop" ? (
            <div className="flex h-full min-h-32 items-center justify-center rounded-[14px] bg-stop-wash p-6 text-center text-sm text-ink-2">
              Nothing ran. The agent was told this action was stopped.
            </div>
          ) : (
            <>
              <div className="mb-1.5 flex items-center justify-between text-xs text-ink-3">
                <span>
                  Approved call: <code className="text-ink">{dec.final_tool}</code>
                  {changedTool && <span className="text-safer"> instead of {d.action.tool}</span>}
                </span>
              </div>
              <JsonView value={dec.final_args} maxHeight={300} label="approved arguments" />
            </>
          )}
        </div>
      </div>
    </Panel>
  );
}

function OptionsPanel({ d }: { d: ActionDetail }) {
  const a = d.action;
  const proposed = a.options.find((o) => o.key === "proceed");
  const safer = a.options.find((o) => o.key === "proceed_safer");
  const [tab, setTab] = useState<"proposed" | "safer">("proposed");
  return (
    <Panel>
      <PanelHeader
        title="The options"
        description="What the agent proposed, and the safer version built from the tool's rules. Stop is always available."
      />
      <Tabs
        className="mb-4"
        value={tab}
        onChange={setTab}
        tabs={[
          { value: "proposed", label: "A. As proposed" },
          { value: "safer", label: safer?.available ? "B. Safer version" : "B. No safer version" },
        ]}
      />
      {tab === "proposed" ? (
        <JsonView value={proposed?.args ?? a.args} label="proposed arguments" />
      ) : safer?.available ? (
        <div className="space-y-3">
          {safer.tool !== a.tool && (
            <p className="text-sm text-ink-2">
              Runs <code className="text-ink">{safer.tool}</code> instead of <code className="text-ink">{a.tool}</code>.
            </p>
          )}
          {safer.changes && (
            <ul className="space-y-1">
              {safer.changes.map((c) => (
                <li key={c} className="flex items-start gap-2 text-sm text-ink">
                  <Icon.Check size={15} className="mt-0.5 text-safer" />
                  {c}
                </li>
              ))}
            </ul>
          )}
          <JsonDiff before={proposed?.args ?? {}} after={safer.args ?? {}} />
        </div>
      ) : (
        <p className="text-sm text-ink-3">
          The tool&rsquo;s rules produced no change for this call, so only running it as proposed or stopping were offered.
        </p>
      )}
    </Panel>
  );
}

const stageTitle: Record<string, string> = {
  gate: "Policy gate",
  model: "Decision model",
  model_recheck: "Decision model, options reversed",
};

function EvaluationsPanel({ evaluations }: { evaluations: Evaluation[] }) {
  if (!evaluations.length) return null;
  return (
    <Panel>
      <PanelHeader title="Evaluations" description="Each check Veto ran, with the exact request and response for the model calls." />
      <div className="space-y-3">
        {evaluations.map((e) => (
          <EvaluationRow key={e.id} e={e} />
        ))}
      </div>
    </Panel>
  );
}

function EvaluationRow({ e }: { e: Evaluation }) {
  const [open, setOpen] = useState(false);
  const gate = e.stage === "gate" ? (e.result as { outcome?: string; policies?: string[] | null; policy_version?: number } | null) : null;
  return (
    <div className="rounded-[14px] shadow-[inset_0_0_0_1px_var(--line)]">
      <button
        type="button"
        onClick={() => setOpen((o) => !o)}
        className="flex w-full items-center gap-3 px-4 py-3 text-left"
        aria-expanded={open}
      >
        <Icon.ChevronRight size={14} className={cx("shrink-0 text-ink-3 transition-transform", open && "rotate-90")} />
        <span className="min-w-0 flex-1">
          <span className="text-sm font-medium text-ink">{stageTitle[e.stage] ?? humanize(e.stage)}</span>
          <span className="ml-2 text-xs text-ink-3">
            {e.provider}
            {e.model && ` ${e.model}`}
          </span>
        </span>
        {e.error ? (
          <Pill tone="stop">Failed</Pill>
        ) : gate ? (
          <Pill>{gate.outcome === "continue" ? "No policy matched" : humanize(gate.outcome ?? "")}</Pill>
        ) : (
          <span className="figures text-xs text-ink-3">
            {formatMs(e.latency_ms)}, {e.input_tokens} tokens
          </span>
        )}
      </button>
      {open && (
        <div className="space-y-3 border-t border-line px-4 py-4">
          {e.error && <p className="text-sm text-stop">{e.error}</p>}
          {gate && (
            <Details
              rows={[
                ["Outcome", humanize(gate.outcome ?? "")],
                ["Matched policies", gate.policies?.length ? gate.policies.join(", ") : "None"],
                ["Policy version", gate.policy_version ? `v${gate.policy_version}` : "–"],
              ]}
            />
          )}
          {e.request !== null && e.request !== undefined && (
            <div>
              <div className="mb-1.5 text-xs text-ink-3">Request sent to the model</div>
              <JsonView value={e.request} maxHeight={320} label="model request" />
            </div>
          )}
          {e.response !== null && e.response !== undefined && (
            <div>
              <div className="mb-1.5 text-xs text-ink-3">Response</div>
              <JsonView value={e.response} maxHeight={320} label="model response" />
            </div>
          )}
          {!gate && e.result !== null && e.result !== undefined && (
            <div>
              <div className="mb-1.5 text-xs text-ink-3">What Veto made of it</div>
              <JsonView value={e.result} maxHeight={260} />
            </div>
          )}
        </div>
      )}
    </div>
  );
}

function entryDetail(e: LedgerEntry): string | null {
  const p = (e.payload ?? {}) as Record<string, unknown>;
  switch (e.kind) {
    case "action.decided": {
      const opt = p.option as Option | undefined;
      const by = p.decided_by as keyof typeof decidedByLabel | undefined;
      return [opt && optionLabel[opt], by && `by ${decidedByLabel[by].toLowerCase()}`].filter(Boolean).join(" ");
    }
    case "outcome.reported":
      return typeof p.status === "string" ? humanize(p.status) : null;
    case "audit.recorded":
      return typeof p.verdict === "string" ? (auditVerdictLabel[p.verdict] ?? humanize(p.verdict)) : null;
    case "gate.evaluated":
      return typeof p.outcome === "string" ? (p.outcome === "continue" ? "No policy matched" : humanize(p.outcome)) : null;
    case "model.evaluated":
      return typeof p.choice === "string" ? `Picked ${optionLabel[p.choice as Option]?.toLowerCase() ?? p.choice}` : null;
    default:
      return null;
  }
}

function Timeline({ entries }: { entries: LedgerEntry[] }) {
  if (!entries.length) return <p className="text-sm text-ink-3">No ledger entries.</p>;
  return (
    <ol className="relative space-y-4 pl-5">
      <span className="absolute top-1.5 bottom-1.5 left-[5px] w-px bg-line" aria-hidden="true" />
      {entries.map((e) => {
        const detail = entryDetail(e);
        const tone =
          e.kind === "action.held"
            ? "bg-held"
            : e.kind === "action.decided"
              ? "bg-ink"
              : e.kind === "action.expired"
                ? "bg-stop"
                : "bg-line-strong";
        return (
          <li key={e.seq} className="relative">
            <span className={cx("absolute top-1.5 -left-5 size-[11px] rounded-full ring-4 ring-surface", tone)} />
            <div className="flex items-baseline justify-between gap-3">
              <span className="text-sm text-ink">{ledgerKindLabel(e.kind)}</span>
              <span className="figures shrink-0 text-xs text-ink-3">{new Date(e.created_at).toLocaleTimeString()}</span>
            </div>
            {detail && <div className="text-xs text-ink-2">{detail}</div>}
            <div className="mt-0.5 flex items-center gap-2 text-2xs text-ink-3">
              <span>#{e.seq}</span>
              <span className="truncate" title={e.actor}>
                {e.actor}
              </span>
            </div>
          </li>
        );
      })}
    </ol>
  );
}

function AfterwardsPanel({ d }: { d: ActionDetail }) {
  return (
    <Panel>
      <PanelHeader title="After the decision" />
      <div className="space-y-5">
        {d.grants.map((g) => (
          <div key={g.id}>
            <div className="flex items-center justify-between">
              <span className="text-sm font-medium text-ink">Grant</span>
              {g.consumed_at ? (
                <Pill tone="neutral">Used</Pill>
              ) : new Date(g.expires_at) < new Date() ? (
                <Pill tone="muted">Expired</Pill>
              ) : (
                <Pill tone="proceed">Valid</Pill>
              )}
            </div>
            <Details
              className="mt-2"
              rows={[
                ["For", <code key="t">{g.tool}</code>],
                [
                  "Arguments hash",
                  <Mono key="h" copy={g.args_hash}>
                    {shortHash(g.args_hash)}
                  </Mono>,
                ],
                ["Signing key", <Mono key="k">{g.key_id}</Mono>],
                ["Issued", formatTimestamp(g.created_at)],
                [g.consumed_at ? "Used" : "Expires", formatTimestamp(g.consumed_at ?? g.expires_at)],
              ]}
            />
          </div>
        ))}
        {d.outcomes.map((o) => (
          <div key={o.id}>
            <div className="flex items-center justify-between">
              <span className="text-sm font-medium text-ink">Outcome</span>
              <OutcomeTag outcome={o.status} />
            </div>
            <p className="mt-1 text-xs text-ink-3">
              Reported by {o.reported_by}, {formatTimestamp(o.created_at)}
            </p>
            {o.detail !== null && o.detail !== undefined && Object.keys(o.detail as object).length > 0 && (
              <JsonView className="mt-2" value={o.detail} maxHeight={160} />
            )}
          </div>
        ))}
        {d.audit && (
          <div>
            <div className="flex items-center justify-between">
              <span className="text-sm font-medium text-ink">Audit</span>
              {d.audit.verdict ? (
                <Pill tone={d.audit.verdict === "correct" ? "proceed" : "stop"}>{auditVerdictLabel[d.audit.verdict]}</Pill>
              ) : (
                <Pill tone="held">Waiting for a label</Pill>
              )}
            </div>
            <p className="mt-1 text-xs text-ink-3">
              {d.audit.reason === "outcome" ? "Queued because a bad outcome was reported." : "Picked by random sampling."}
              {d.audit.reviewer_name && ` Labelled by ${d.audit.reviewer_name}.`}
            </p>
            {d.audit.note && <p className="mt-1 text-sm text-ink">{d.audit.note}</p>}
            {!d.audit.verdict && (
              <Link href="/audits" className="mt-2 inline-flex items-center gap-1 text-sm text-ink-2 hover:text-ink">
                Label it in Audits
                <Icon.ChevronRight size={13} />
              </Link>
            )}
          </div>
        )}
      </div>
    </Panel>
  );
}
