"use client";

import { useInfiniteQuery } from "@tanstack/react-query";
import type { Route } from "next";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { useMemo, useState } from "react";
import { ReasonInline } from "@/components/actions/reasons";
import { Icon } from "@/components/icons";
import { EmptyRows, NothingFound } from "@/components/illustrations";
import { Button } from "@/components/ui/button";
import { Input, Select } from "@/components/ui/form";
import { PageHeader, Panel } from "@/components/ui/layout";
import { DecidedByText, DecisionChip, OutcomeTag } from "@/components/ui/pills";
import { EmptyState, ErrorState, LoadingPanel } from "@/components/ui/states";
import { LinkRow, RowLink, Table, Td, Th } from "@/components/ui/table";
import { api } from "@/lib/api";
import { formatDateTime, formatMs, formatProbability, formatRelative } from "@/lib/format";
import { useAgents, useTools, type ActionFilter } from "@/lib/queries";
import type { ActionPage, ActionSummary } from "@/lib/types";

const FILTER_KEYS = ["status", "agent", "tool", "decided_by", "option", "source", "q", "held", "since"] as const;

const SINCE: Record<string, number> = { "1h": 3600, "24h": 86_400, "7d": 7 * 86_400, "30d": 30 * 86_400 };

function toCSV(rows: ActionSummary[]): string {
  const cols: (keyof ActionSummary)[] = [
    "id",
    "received_at",
    "decided_at",
    "agent_name",
    "tool",
    "principal_display",
    "request",
    "status",
    "option",
    "decided_by",
    "reviewer_name",
    "confidence",
    "risk",
    "outcome",
    "decision_ms",
    "source",
  ];
  const esc = (v: unknown) => {
    const s = v === null || v === undefined ? "" : String(v);
    return /[",\n]/.test(s) ? `"${s.replace(/"/g, '""')}"` : s;
  };
  return [cols.join(","), ...rows.map((r) => cols.map((c) => esc(r[c])).join(","))].join("\n");
}

/** Free-text search, applied on Enter or when the field loses focus. */
function SearchBox({ initial, onSearch }: { initial: string; onSearch: (v: string) => void }) {
  const [value, setValue] = useState(initial);
  return (
    <form
      className="relative"
      onSubmit={(e) => {
        e.preventDefault();
        onSearch(value.trim());
      }}
    >
      <Icon.Search size={15} className="pointer-events-none absolute top-1/2 left-3 -translate-y-1/2 text-ink-3" />
      <Input
        value={value}
        onChange={(e) => setValue(e.target.value)}
        onBlur={() => value.trim() !== initial && onSearch(value.trim())}
        placeholder="Search requests, tools, ids"
        className="pl-9"
        aria-label="Search"
      />
    </form>
  );
}

export function ActionsExplorer() {
  const router = useRouter();
  const pathname = usePathname();
  const params = useSearchParams();
  const agents = useAgents();
  const tools = useTools();

  const filter = useMemo(() => {
    const f: ActionFilter = { limit: 50 };
    for (const k of FILTER_KEYS) {
      const v = params.get(k);
      if (!v) continue;
      if (k === "held") f.held = v === "true";
      else if (k === "since") {
        if (SINCE[v]) f.since = v;
      } else f[k] = v;
    }
    return f;
  }, [params]);

  const setParam = (k: string, v: string) => {
    const next = new URLSearchParams(params);
    if (v) next.set(k, v);
    else next.delete(k);
    const qs = next.toString();
    router.replace(`${pathname}${qs ? `?${qs}` : ""}` as Route, { scroll: false });
  };

  const q = useInfiniteQuery({
    queryKey: ["actions", "explorer", filter],
    // The time window is resolved when fetching, so a refetch moves it forward.
    queryFn: ({ pageParam }) =>
      api.get<ActionPage>("/v1/actions", {
        ...filter,
        since: filter.since ? new Date(Date.now() - SINCE[filter.since]! * 1000).toISOString() : undefined,
        cursor: pageParam || undefined,
      }),
    initialPageParam: "",
    getNextPageParam: (last) => last.next_cursor || undefined,
  });
  const rows = q.data?.pages.flatMap((p) => p.items) ?? [];
  const filtered = FILTER_KEYS.some((k) => params.get(k));

  function exportCSV() {
    const blob = new Blob([toCSV(rows)], { type: "text/csv" });
    const url = URL.createObjectURL(blob);
    const link = document.createElement("a");
    link.href = url;
    link.download = `veto-actions-${new Date().toISOString().slice(0, 19).replace(/:/g, "")}.csv`;
    link.click();
    URL.revokeObjectURL(url);
  }

  return (
    <>
      <PageHeader
        title="Actions"
        description="Every call an agent made through Veto, with what was decided and why. Open one for its full record."
        actions={
          <Button icon={<Icon.Download size={15} />} onClick={exportCSV} disabled={!rows.length}>
            Export loaded rows
          </Button>
        }
      />
      <Panel>
        <div className="mb-5 grid gap-2.5 sm:grid-cols-2 lg:grid-cols-[minmax(220px,1.4fr)_repeat(6,minmax(0,1fr))]">
          <SearchBox key={params.get("q") ?? ""} initial={params.get("q") ?? ""} onSearch={(v) => setParam("q", v)} />
          <Select aria-label="Result" value={params.get("status") ?? ""} onChange={(e) => setParam("status", e.target.value)}>
            <option value="">Any result</option>
            <option value="pending_review">Waiting</option>
            <option value="approved">Approved</option>
            <option value="denied">Stopped</option>
            <option value="expired">Expired</option>
          </Select>
          <Select aria-label="Option" value={params.get("option") ?? ""} onChange={(e) => setParam("option", e.target.value)}>
            <option value="">Any option</option>
            <option value="proceed">Proceeded</option>
            <option value="proceed_safer">Safer version</option>
            <option value="stop">Stop</option>
          </Select>
          <Select aria-label="Decided by" value={params.get("decided_by") ?? ""} onChange={(e) => setParam("decided_by", e.target.value)}>
            <option value="">Anyone</option>
            <option value="model">Decision model</option>
            <option value="human">Reviewer</option>
            <option value="policy">Policy</option>
            <option value="expiry">Review expired</option>
            <option value="failsafe">Failsafe</option>
          </Select>
          <Select aria-label="Agent" value={params.get("agent") ?? ""} onChange={(e) => setParam("agent", e.target.value)}>
            <option value="">All agents</option>
            {agents.data?.items.map((a) => (
              <option key={a.id} value={a.id}>
                {a.name}
              </option>
            ))}
          </Select>
          <Select aria-label="Tool" value={params.get("tool") ?? ""} onChange={(e) => setParam("tool", e.target.value)}>
            <option value="">All tools</option>
            {tools.data?.items.map((t) => (
              <option key={t.name} value={t.name}>
                {t.name}
              </option>
            ))}
          </Select>
          <Select aria-label="Time" value={params.get("since") ?? ""} onChange={(e) => setParam("since", e.target.value)}>
            <option value="">All time</option>
            <option value="1h">Last hour</option>
            <option value="24h">Last 24 hours</option>
            <option value="7d">Last 7 days</option>
            <option value="30d">Last 30 days</option>
          </Select>
        </div>
        {filtered && (
          <div className="mb-4 flex items-center gap-3 text-sm text-ink-3">
            <span>Filtered.</span>
            <button type="button" className="text-ink underline-offset-4 hover:underline" onClick={() => router.replace(pathname as Route)}>
              Clear filters
            </button>
          </div>
        )}

        {q.isError && <ErrorState error={q.error} onRetry={() => void q.refetch()} />}
        {q.isPending ? (
          <LoadingPanel rows={8} />
        ) : rows.length === 0 ? (
          filtered ? (
            <EmptyState art={<NothingFound />} title="No actions match these filters">
              Widen the time range or clear a filter.
            </EmptyState>
          ) : (
            <EmptyState art={<EmptyRows />} title="No actions yet">
              Create an agent, give it a key, and send its first call to <code>POST /v1/actions</code>.
            </EmptyState>
          )
        ) : (
          <>
            <Table>
              <thead>
                <tr>
                  <Th>Action</Th>
                  <Th>Agent</Th>
                  <Th>Request</Th>
                  <Th>Result</Th>
                  <Th>Decided by</Th>
                  <Th className="text-right">Confidence</Th>
                  <Th className="text-right">Risk</Th>
                  <Th>Outcome</Th>
                  <Th className="text-right">Received</Th>
                </tr>
              </thead>
              <tbody>
                {rows.map((a) => (
                  <LinkRow key={a.id} href={`/actions/${a.id}`}>
                    <Td>
                      <RowLink href={`/actions/${a.id}`}>{a.tool}</RowLink>
                      {a.source !== "api" && (
                        <div className="text-xs text-ink-3">{a.source === "mcp" ? "Through MCP" : "Through a connection"}</div>
                      )}
                    </Td>
                    <Td className="whitespace-nowrap text-ink-2">{a.agent_name}</Td>
                    <Td className="max-w-60">
                      <div className="truncate text-ink-2">{a.request || <span className="text-ink-3">No request given</span>}</div>
                      {a.status === "pending_review" && (
                        <div className="truncate text-xs">
                          <ReasonInline reasons={a.hold_reasons} />
                        </div>
                      )}
                    </Td>
                    <Td>
                      <DecisionChip status={a.status} option={a.option} />
                    </Td>
                    <Td className="whitespace-nowrap text-ink-2">
                      <DecidedByText by={a.decided_by} reviewer={a.reviewer_name} />
                      {a.decision_ms !== null && (a.decided_by === "model" || a.decided_by === "policy" || a.decided_by === "failsafe") && (
                        <div className="text-xs text-ink-3">in {formatMs(a.decision_ms)}</div>
                      )}
                    </Td>
                    <Td className="figures text-right text-ink-2">{formatProbability(a.confidence)}</Td>
                    <Td className="figures text-right text-ink-2">{formatProbability(a.risk)}</Td>
                    <Td>{a.outcome ? <OutcomeTag outcome={a.outcome} /> : <span className="text-ink-3">–</span>}</Td>
                    <Td className="text-right whitespace-nowrap text-ink-3" title={formatDateTime(a.received_at)}>
                      {formatRelative(a.received_at)}
                    </Td>
                  </LinkRow>
                ))}
              </tbody>
            </Table>
            <div className="mt-5 flex items-center justify-between text-sm text-ink-3">
              <span>{rows.length} loaded</span>
              {q.hasNextPage && (
                <Button onClick={() => void q.fetchNextPage()} busy={q.isFetchingNextPage}>
                  Load more
                </Button>
              )}
            </div>
          </>
        )}
      </Panel>
    </>
  );
}
