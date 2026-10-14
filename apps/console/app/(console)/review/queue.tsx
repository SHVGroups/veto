"use client";

import Link from "next/link";
import type { Route } from "next";
import { useRouter, useSearchParams } from "next/navigation";
import { useEffect, useMemo } from "react";
import { AssessmentView, useEffectiveThresholds } from "@/components/actions/assessment";
import { ActionContext } from "@/components/actions/context";
import { Deadline } from "@/components/actions/deadline";
import { ReasonInline, ReasonList } from "@/components/actions/reasons";
import { ReviewPanel } from "@/components/actions/review-panel";
import { Icon } from "@/components/icons";
import { QueueClear } from "@/components/illustrations";
import { PageHeader, Panel, PanelHeader } from "@/components/ui/layout";
import { OptionTag } from "@/components/ui/pills";
import { EmptyState, ErrorState, LoadingPanel } from "@/components/ui/states";
import { cx } from "@/lib/cx";
import { pluralize } from "@/lib/format";
import { useActionDetail, useQueue } from "@/lib/queries";
import type { ActionSummary } from "@/lib/types";

/** Soonest deadline first: those are the actions that will be stopped next if nobody decides. */
function byDeadline(a: ActionSummary, b: ActionSummary) {
  const da = a.review_deadline ? Date.parse(a.review_deadline) : Infinity;
  const db = b.review_deadline ? Date.parse(b.review_deadline) : Infinity;
  return da - db;
}

export function ReviewQueue() {
  const router = useRouter();
  const params = useSearchParams();
  const q = useQueue();
  const items = useMemo(() => [...(q.data?.items ?? [])].sort(byDeadline), [q.data]);
  const requested = params.get("id");
  const selectedId = items.some((i) => i.id === requested) ? requested! : items[0]?.id;
  const index = items.findIndex((i) => i.id === selectedId);

  const select = (id: string | undefined) => {
    if (id) router.replace(`/review?id=${id}` as Route, { scroll: false });
  };

  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      const t = e.target as HTMLElement;
      if (t.closest("input,textarea,select") || e.metaKey || e.ctrlKey) return;
      if (e.key === "j" || e.key === "ArrowDown") {
        e.preventDefault();
        select(items[Math.min(items.length - 1, index + 1)]?.id);
      } else if (e.key === "k" || e.key === "ArrowUp") {
        e.preventDefault();
        select(items[Math.max(0, index - 1)]?.id);
      }
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  });

  return (
    <>
      <PageHeader
        title="Review"
        description={
          items.length
            ? `${pluralize(items.length, "action")} waiting for a decision. Each one is stopped when its time runs out.`
            : "Actions Veto held for a person appear here."
        }
      />
      {q.isError && <ErrorState error={q.error} onRetry={() => void q.refetch()} className="mb-5" />}
      {q.isPending ? (
        <Panel>
          <LoadingPanel rows={6} />
        </Panel>
      ) : items.length === 0 ? (
        <Panel>
          <EmptyState art={<QueueClear />} title="Nothing is waiting for you">
            When an action needs a person, it appears here and in Slack if you connected it. This page updates on its own.
          </EmptyState>
        </Panel>
      ) : (
        <div className="grid items-start gap-5 xl:grid-cols-[360px_minmax(0,1fr)]">
          <Panel padded={false} className="xl:sticky xl:top-6">
            <ul className="scroll-quiet max-h-[calc(100dvh-180px)] overflow-y-auto p-2" aria-label="Held actions">
              {items.map((a) => (
                <li key={a.id}>
                  <button
                    type="button"
                    onClick={() => select(a.id)}
                    aria-current={a.id === selectedId ? "true" : undefined}
                    className={cx(
                      "w-full animate-[veto-arrive_220ms_ease-out] rounded-[12px] px-3.5 py-3 text-left transition-colors",
                      a.id === selectedId ? "bg-held-wash" : "hover:bg-sunken",
                    )}
                  >
                    <div className="flex items-center justify-between gap-3">
                      <span className="truncate text-sm font-medium text-ink">{a.tool}</span>
                      <Deadline receivedAt={a.received_at} deadline={a.review_deadline} className="shrink-0 text-xs" />
                    </div>
                    <div className="mt-0.5 truncate text-xs text-ink-3">
                      {a.agent_name}
                      {a.principal_display && ` for ${a.principal_display}`}
                    </div>
                    {a.request && <div className="mt-1.5 line-clamp-2 text-sm text-ink-2">{a.request}</div>}
                    <div className="mt-2 flex items-center justify-between gap-2 text-xs">
                      <span className="truncate">
                        <ReasonInline reasons={a.hold_reasons} />
                      </span>
                      {a.ranking[0] && <OptionTag option={a.ranking[0]} />}
                    </div>
                  </button>
                </li>
              ))}
            </ul>
          </Panel>
          {selectedId && (
            <ReviewDetail key={selectedId} id={selectedId} onDone={() => select(items[index + 1]?.id ?? items[index - 1]?.id)} />
          )}
        </div>
      )}
    </>
  );
}

function ReviewDetail({ id, onDone }: { id: string; onDone: () => void }) {
  const q = useActionDetail(id);
  const d = q.data;
  const thresholds = useEffectiveThresholds(d?.tool ?? null, d?.decision?.thresholds);
  if (q.isError) return <ErrorState error={q.error} onRetry={() => void q.refetch()} />;
  if (!d) {
    return (
      <Panel>
        <LoadingPanel rows={8} />
      </Panel>
    );
  }
  const a = d.action;
  const offered = a.options.filter((o) => o.available).map((o) => o.key);
  return (
    <div className="space-y-5">
      <Panel>
        <div className="mb-5 flex flex-wrap items-start justify-between gap-4">
          <div className="min-w-0">
            <h2 className="truncate text-xl text-ink">{a.tool}</h2>
            <Link href={`/actions/${a.id}` as Route} className="mt-1 inline-flex items-center gap-1 text-xs text-ink-3 hover:text-ink">
              {a.id}
              <Icon.ChevronRight size={12} />
            </Link>
          </div>
          <Deadline receivedAt={a.received_at} deadline={a.review_deadline} bar className="w-56" />
        </div>
        <ActionContext detail={d} />
        <div className="mt-6 rounded-[14px] bg-held-wash px-4 py-3">
          <div className="text-xs font-medium text-held">Why Veto is asking you</div>
          <ReasonList reasons={a.hold_reasons} className="mt-1 space-y-0.5" />
        </div>
      </Panel>

      <Panel>
        <PanelHeader
          title="Your decision"
          description="Pick what should happen. The agent receives a grant only for the call you approve."
        />
        <ReviewPanel key={a.id} detail={d} keyboard onDecided={onDone} />
      </Panel>

      {d.assessment && (
        <Panel>
          <PanelHeader
            title="What the decision model thought"
            description="Its answer, for context. The reasons above say why it did not decide alone."
          />
          <AssessmentView a={d.assessment} thresholds={thresholds} offered={offered} />
        </Panel>
      )}
    </div>
  );
}
