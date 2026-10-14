"use client";

import Link from "next/link";
import type { Route } from "next";
import { ReasonInline, useReasonLabel } from "@/components/actions/reasons";
import { BarList, RiskHistogram } from "@/components/charts/distribution";
import { DecisionFlow } from "@/components/charts/flow";
import { LatencyChart, VolumeChart, volumeLegend } from "@/components/charts/time";
import { Icon } from "@/components/icons";
import { ButtonLink } from "@/components/ui/button";
import { Segmented } from "@/components/ui/form";
import { PageHeader, Panel, PanelHeader, Stat } from "@/components/ui/layout";
import { DecisionChip, LiveDot, Pill } from "@/components/ui/pills";
import { ErrorState, LoadingPanel, Notice, Skeleton } from "@/components/ui/states";
import { cx } from "@/lib/cx";
import { formatCount, formatDuration, formatMs, formatPercent, formatRelative, pluralize } from "@/lib/format";
import { useLive } from "@/lib/live";
import { useStored } from "@/lib/stored";
import { useActions, useOverview } from "@/lib/queries";
import type { Overview as OverviewData, Range } from "@/lib/types";

const RANGES: { value: Range; label: string }[] = [
  { value: "1h", label: "1h" },
  { value: "24h", label: "24h" },
  { value: "7d", label: "7d" },
  { value: "30d", label: "30d" },
  { value: "90d", label: "90d" },
];

const RANGE_WORDS: Record<Range, string> = {
  "1h": "the last hour",
  "24h": "the last 24 hours",
  "7d": "the last 7 days",
  "30d": "the last 30 days",
  "90d": "the last 90 days",
};

export function Overview() {
  const [range, setRange] = useStored<Range>("veto.overview.range", "24h", ["1h", "24h", "7d", "30d", "90d"]);
  const q = useOverview(range);
  const o = q.data;

  return (
    <>
      <PageHeader
        title="Overview"
        description={`What your agents tried to do in ${RANGE_WORDS[range]}, and what Veto did about it.`}
        actions={
          <>
            {o && (
              <Pill tone={o.model.configured ? "neutral" : "safer"} dot>
                {o.model.configured ? `Decision model ${o.model.name}` : "No decision model configured"}
              </Pill>
            )}
            <Segmented label="Time range" value={range} options={RANGES} onChange={(r) => setRange(r)} />
          </>
        }
      />
      {q.isError && <ErrorState error={q.error} onRetry={() => void q.refetch()} className="mb-6" />}
      {!o ? <OverviewSkeleton /> : <OverviewBody o={o} range={range} fetching={q.isFetching} />}
    </>
  );
}

function OverviewBody({ o, range, fetching }: { o: OverviewData; range: Range; fetching: boolean }) {
  const c = o.counts;
  const reasonLabel = useReasonLabel();
  return (
    <div className={cx("space-y-5 transition-opacity", fetching && "opacity-80")}>
      <Attention o={o} />

      <Panel className="grid grid-cols-2 gap-x-6 gap-y-7 md:grid-cols-3 xl:grid-cols-6">
        <Stat label="Actions received" value={formatCount(c.received)} context={`${formatCount(c.decided)} decided`} />
        <Stat
          label="Decided without a person"
          value={formatPercent(o.rates.auto_resolution)}
          context={`${formatCount(c.by_model)} by the model, ${formatCount(c.by_policy)} by policy`}
        />
        <Stat label="Held for review" value={formatPercent(o.rates.hold_rate)} tone="held" context={`${formatCount(c.held)} actions`} />
        <Stat
          label="Safer version taken"
          value={formatPercent(o.rates.safer_take)}
          tone="safer"
          context={`Offered on ${formatPercent(o.rates.safer_offered)} of actions`}
        />
        <Stat label="Stopped" value={formatPercent(o.rates.stop_rate)} tone="stop" context={`${formatCount(c.opt_stop)} actions`} />
        <Stat
          label="Time to review"
          value={formatDuration(o.review_seconds.p50)}
          context={o.review_seconds.p90 !== null ? `90% within ${formatDuration(o.review_seconds.p90)}` : "No reviews yet"}
        />
      </Panel>

      <Panel>
        <PanelHeader
          title="How actions were decided"
          description="Every action in the range, from who or what decided it to the option it ended on. Hover a band for counts."
        />
        <DecisionFlow flows={o.flows} />
      </Panel>

      <div className="grid gap-5 xl:grid-cols-[minmax(0,1fr)_380px]">
        <Panel>
          <PanelHeader title="Actions over time" description="Each bar is split by how the actions in it ended." />
          <VolumeChart series={o.series} bucketSeconds={o.bucket_seconds} height={330} />
          <div className="mt-3 flex flex-wrap gap-x-5 gap-y-1.5">
            {volumeLegend.map((l) => (
              <span key={l.label} className="flex items-center gap-2 text-xs text-ink-3">
                <span className="size-2.5 rounded-[3px]" style={{ background: l.color }} />
                {l.label}
              </span>
            ))}
          </div>
        </Panel>
        <LatestActions />
      </div>

      <div className="grid gap-5 lg:grid-cols-2">
        <Panel>
          <PanelHeader
            title="Decision model"
            description="Latency of the model call for each action. The dashed line is the 95th percentile."
          />
          <div className="mb-4 grid grid-cols-2 gap-5 sm:grid-cols-4">
            <Small label="Calls" value={formatCount(c.model_calls)} />
            <Small label="Errors" value={formatPercent(o.rates.model_error_rate)} tone={c.model_errors ? "stop" : undefined} />
            <Small label="Median" value={formatMs(o.model_latency_ms.p50)} />
            <Small label="Decision, p95" value={formatMs(o.auto_decision_ms.p95)} />
          </div>
          <LatencyChart points={o.latency} bucketSeconds={o.bucket_seconds} />
          <p className="mt-3 text-xs text-ink-3">{formatCount(c.input_tokens)} input tokens sent to the model in this range.</p>
        </Panel>
        <Panel>
          <PanelHeader
            title="Risk of assessed actions"
            description="Composite risk from the question pack's signals. Higher bins are held more often."
          />
          <RiskHistogram bins={o.risk_histogram} />
          <div className="mt-5 grid grid-cols-3 gap-5 border-t border-line pt-5">
            <Small
              label="Wrong automatic approvals"
              value={o.rates.false_approval ? formatPercent(o.rates.false_approval.value) : "–"}
              hint={
                o.rates.false_approval
                  ? `95% interval ${formatPercent(o.rates.false_approval.low)} to ${formatPercent(o.rates.false_approval.high)}, ${pluralize(o.rates.false_approval.n, "audit")}`
                  : "No audited automatic approvals yet"
              }
              tone={o.rates.false_approval && o.rates.false_approval.value > 0 ? "stop" : undefined}
            />
            <Small
              label="Audits reviewed"
              value={formatCount(c.audits_reviewed)}
              hint={`${formatCount(c.audits_wrong)} found a wrong decision`}
            />
            <Small
              label="Bad outcomes reported"
              value={formatCount(c.outcome_failed + c.outcome_reversed + c.outcome_complaint)}
              hint={`${c.outcome_failed} failed, ${c.outcome_reversed} reversed, ${c.outcome_complaint} complaints`}
            />
          </div>
        </Panel>
      </div>

      <div className="grid gap-5 lg:grid-cols-3">
        <Panel>
          <PanelHeader title="Tools" description="Most called. The tinted part was held." />
          <BarList items={o.top_tools} hrefFor={(k) => `/tools/${k}`} emptyText="No tool calls in this range." />
        </Panel>
        <Panel>
          <PanelHeader title="Agents" description="Most active. The tinted part was held." />
          <BarList items={o.top_agents} hrefFor={(k) => `/agents/${k}`} emptyText="No agent activity in this range." />
        </Panel>
        <Panel>
          <PanelHeader title="Why actions were held" description="An action can be held for more than one reason." />
          <BarList items={o.hold_reasons} labelFor={(k) => reasonLabel(k)} emptyText="Nothing was held in this range." />
        </Panel>
      </div>

      <p className="text-xs text-ink-3">
        {new Date(o.from).toLocaleString()} to {new Date(o.to).toLocaleString()}, in buckets of {formatDuration(o.bucket_seconds)}.{" "}
        {range === "1h" ? "Updates live." : "Updates live as actions arrive."}
      </p>
    </div>
  );
}

function Small({ label, value, hint, tone }: { label: string; value: string; hint?: string; tone?: "stop" }) {
  return (
    <div className="min-w-0">
      <div className="text-xs text-ink-3">{label}</div>
      <div className={cx("numeral mt-0.5 text-xl", tone === "stop" ? "text-stop" : "text-ink")}>{value}</div>
      {hint && <div className="mt-0.5 text-xs text-ink-3">{hint}</div>}
    </div>
  );
}

function Attention({ o }: { o: OverviewData }) {
  const l = o.live;
  const items: React.ReactNode[] = [];
  if (l.pending > 0) {
    items.push(
      <Notice
        key="pending"
        tone="held"
        title={`${pluralize(l.pending, "action")} ${l.pending === 1 ? "is" : "are"} waiting for review`}
        action={
          <ButtonLink href="/review" variant="primary" size="sm">
            Open review
          </ButtonLink>
        }
      >
        The oldest arrived {formatRelative(l.oldest_pending_at)}
        {l.next_deadline && <>; the next one expires {formatRelative(l.next_deadline)} and will be stopped if nobody decides</>}.
      </Notice>,
    );
  }
  if (l.unregistered_tools > 0) {
    items.push(
      <Notice
        key="tools"
        title={`${pluralize(l.unregistered_tools, "new tool")} ${l.unregistered_tools === 1 ? "needs" : "need"} a risk class`}
        action={
          <ButtonLink href="/tools?filter=unregistered" size="sm">
            Classify
          </ButtonLink>
        }
      >
        Calls to unclassified tools are held for a person until you set how risky they are.
      </Notice>,
    );
  }
  if (l.open_audits > 0) {
    items.push(
      <Notice
        key="audits"
        title={`${pluralize(l.open_audits, "audit")} to label`}
        action={
          <ButtonLink href="/audits" size="sm">
            Label
          </ButtonLink>
        }
      >
        Labelling sampled decisions is how Veto measures whether automatic approvals can be trusted.
      </Notice>,
    );
  }
  if (!items.length) return null;
  return <div className="grid gap-3 xl:grid-cols-2">{items}</div>;
}

/**
 * The newest actions, refreshed as they arrive. Rows are keyed by action, so
 * only rows that are new to the list mount, and only they slide in.
 */
function LatestActions() {
  const q = useActions({ limit: 7 });
  const live = useLive();
  const items = q.data?.items ?? [];

  return (
    <Panel className="flex flex-col">
      <PanelHeader
        title="Latest actions"
        description={live.state === "live" ? "Arriving live." : "Live updates are reconnecting."}
        actions={<LiveDot state={live.state} />}
      />
      {q.isPending ? (
        <LoadingPanel rows={6} />
      ) : items.length === 0 ? (
        <p className="py-8 text-sm text-ink-3">No actions yet. Once an agent calls Veto, its actions appear here.</p>
      ) : (
        <ul className="-mx-2 flex-1 space-y-0.5">
          {items.map((a) => (
            <li key={a.id} className="animate-[veto-arrive_260ms_ease-out]">
              <Link href={`/actions/${a.id}` as Route} className="flex items-start gap-3 rounded-[10px] px-2 py-2 hover:bg-sunken">
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-2">
                    <span className="truncate text-sm font-medium text-ink">{a.tool}</span>
                  </div>
                  <div className="mt-0.5 truncate text-xs text-ink-3">
                    {a.agent_name}
                    {a.status === "pending_review" ? (
                      <>
                        {", "}
                        <ReasonInline reasons={a.hold_reasons} />
                      </>
                    ) : (
                      `, ${formatRelative(a.received_at)}`
                    )}
                  </div>
                </div>
                <DecisionChip status={a.status} option={a.option} />
              </Link>
            </li>
          ))}
        </ul>
      )}
      <Link href="/actions" className="mt-3 inline-flex items-center gap-1.5 self-start text-sm text-ink-2 hover:text-ink">
        All actions
        <Icon.ChevronRight size={14} />
      </Link>
    </Panel>
  );
}

function OverviewSkeleton() {
  return (
    <div className="space-y-5">
      <Panel className="grid grid-cols-2 gap-6 md:grid-cols-3 xl:grid-cols-6">
        {Array.from({ length: 6 }, (_, i) => (
          <div key={i} className="space-y-2">
            <Skeleton className="h-4 w-24" />
            <Skeleton className="h-9 w-20" />
          </div>
        ))}
      </Panel>
      <Panel>
        <Skeleton className="h-[300px] w-full" />
      </Panel>
      <div className="grid gap-5 xl:grid-cols-[minmax(0,1fr)_380px]">
        <Panel>
          <Skeleton className="h-[240px] w-full" />
        </Panel>
        <Panel>
          <LoadingPanel rows={6} />
        </Panel>
      </div>
    </div>
  );
}
