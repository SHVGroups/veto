"use client";

import Link from "next/link";
import type { Route } from "next";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { Fragment, useState } from "react";
import { Icon } from "@/components/icons";
import { Chain } from "@/components/illustrations";
import { Button } from "@/components/ui/button";
import { JsonView, Mono } from "@/components/ui/code";
import { Input, Select } from "@/components/ui/form";
import { PageHeader, Panel, PanelHeader } from "@/components/ui/layout";
import { Pill } from "@/components/ui/pills";
import { EmptyState, ErrorState, LoadingPanel, Notice } from "@/components/ui/states";
import { Table, Td, Th } from "@/components/ui/table";
import { useToast } from "@/components/ui/toast";
import { api, errorMessage } from "@/lib/api";
import { cx } from "@/lib/cx";
import { formatCount, formatMs, formatRelative, formatTimestamp, ledgerKindLabel, shortHash } from "@/lib/format";
import { useLedger } from "@/lib/queries";
import type { VerifyReport } from "@/lib/types";

const KINDS = [
  "action.received",
  "action.held",
  "action.decided",
  "action.expired",
  "grant.issued",
  "grant.consumed",
  "action.executed",
  "outcome.reported",
  "audit.sampled",
  "audit.recorded",
  "policy.created",
  "policy.activated",
  "pack.updated",
  "tool.updated",
  "tool.deleted",
  "agent.created",
  "agent.updated",
  "key.created",
  "key.revoked",
  "connection.changed",
  "mcp_server.changed",
  "webhook.changed",
  "integration.changed",
  "member.changed",
  "settings.updated",
  "session.created",
  "workspace.created",
  "signing_key.rotated",
];

export function LedgerPage() {
  const params = useSearchParams();
  const router = useRouter();
  const pathname = usePathname();
  const toast = useToast();
  const kind = params.get("kind") ?? "";
  const action = params.get("action") ?? "";
  const [cursors, setCursors] = useState<number[]>([0]);
  const before = cursors[cursors.length - 1] ?? 0;
  const q = useLedger(kind, action, before);
  const [expanded, setExpanded] = useState<number | null>(null);
  const [actionInput, setActionInput] = useState(action);
  const [report, setReport] = useState<VerifyReport | null>(null);
  const [verifying, setVerifying] = useState(false);

  const setParam = (k: string, v: string) => {
    const next = new URLSearchParams(params);
    if (v) next.set(k, v);
    else next.delete(k);
    setCursors([0]);
    router.replace(`${pathname}${next.toString() ? `?${next}` : ""}` as Route, { scroll: false });
  };

  async function verify() {
    setVerifying(true);
    try {
      const r = await api.post<VerifyReport>("/v1/ledger/verify");
      setReport(r);
      if (r.break) toast.error(`The ledger is broken at entry ${r.break.seq}`);
      else toast.success(`${formatCount(r.entries)} entries verified`);
    } catch (err) {
      toast.error(errorMessage(err));
    } finally {
      setVerifying(false);
    }
  }

  const page = q.data;
  return (
    <>
      <PageHeader
        title="Ledger"
        description="An append-only record of everything Veto did and everyone who changed it. Each entry includes the hash of the one before, and signed checkpoints let anyone confirm nothing was edited or removed."
        actions={
          <Button variant="primary" icon={<Icon.Check size={15} />} busy={verifying} onClick={() => void verify()}>
            Verify the chain
          </Button>
        }
      />
      {report && (
        <div className="mb-5">
          {report.break ? (
            <Notice tone="stop" title={`Verification failed at entry ${report.break.seq}`}>
              {report.break.reason}. Entries from this point cannot be trusted. Restore from backup and investigate who had database access.
            </Notice>
          ) : (
            <Notice tone="proceed" title={`The chain is intact: ${formatCount(report.entries)} entries, head at ${report.head_seq}`}>
              Head hash <Mono>{shortHash(report.head_hash, 12)}</Mono>.{" "}
              {report.checkpoint
                ? `The latest checkpoint, at entry ${report.checkpoint.seq}, ${report.checkpoint.valid ? "has a valid signature" : "has an invalid signature"}.`
                : "No signed checkpoint yet."}{" "}
              Took {formatMs(report.duration_ms)}.
            </Notice>
          )}
        </div>
      )}

      <div className="grid items-start gap-5 xl:grid-cols-[minmax(0,1fr)_340px]">
        <Panel>
          <div className="mb-4 flex flex-wrap gap-2.5">
            <Select aria-label="Kind" value={kind} onChange={(e) => setParam("kind", e.target.value)} className="w-60">
              <option value="">Every kind</option>
              {KINDS.map((k) => (
                <option key={k} value={k}>
                  {ledgerKindLabel(k)}
                </option>
              ))}
            </Select>
            <form
              className="flex-1"
              onSubmit={(e) => {
                e.preventDefault();
                setParam("action", actionInput.trim());
              }}
            >
              <Input
                placeholder="Action id, such as act_01h…"
                value={actionInput}
                onChange={(e) => setActionInput(e.target.value)}
                onBlur={() => actionInput.trim() !== action && setParam("action", actionInput.trim())}
                aria-label="Action id"
              />
            </form>
          </div>
          {q.isError && <ErrorState error={q.error} onRetry={() => void q.refetch()} />}
          {q.isPending ? (
            <LoadingPanel rows={10} />
          ) : !page?.entries.length ? (
            <EmptyState art={<Chain />} title="No entries match">
              Clear the filters to see the whole chain.
            </EmptyState>
          ) : (
            <>
              <Table>
                <thead>
                  <tr>
                    <Th className="w-16">#</Th>
                    <Th>Entry</Th>
                    <Th>By</Th>
                    <Th>Hash</Th>
                    <Th className="text-right">When</Th>
                  </tr>
                </thead>
                <tbody>
                  {page.entries.map((e) => (
                    <Fragment key={e.seq}>
                      <tr className="cursor-pointer hover:bg-sunken" onClick={() => setExpanded(expanded === e.seq ? null : e.seq)}>
                        <Td className="figures text-ink-3">{e.seq}</Td>
                        <Td>
                          <div className="flex items-center gap-2">
                            <Icon.ChevronRight
                              size={13}
                              className={cx("text-ink-3 transition-transform", expanded === e.seq && "rotate-90")}
                            />
                            <span className="font-medium text-ink">{ledgerKindLabel(e.kind)}</span>
                          </div>
                          {e.action_id && (
                            <Link
                              href={`/actions/${e.action_id}` as Route}
                              className="ml-5 text-xs text-ink-3 hover:text-ink"
                              onClick={(ev) => ev.stopPropagation()}
                            >
                              {e.action_id}
                            </Link>
                          )}
                        </Td>
                        <Td className="max-w-56 truncate text-xs text-ink-2">{e.actor}</Td>
                        <Td>
                          <Mono>{shortHash(e.hash)}</Mono>
                        </Td>
                        <Td className="text-right whitespace-nowrap text-ink-3" title={formatTimestamp(e.created_at)}>
                          {formatRelative(e.created_at)}
                        </Td>
                      </tr>
                      {expanded === e.seq && (
                        <tr>
                          <td colSpan={5} className="border-b border-line bg-sunken/60 px-4 py-4">
                            <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_280px]">
                              <JsonView value={e.payload} maxHeight={300} label="payload" />
                              <div className="space-y-2 text-xs">
                                <div>
                                  <div className="text-ink-3">Previous hash</div>
                                  <Mono copy={e.prev_hash} className="break-all">
                                    {e.prev_hash}
                                  </Mono>
                                </div>
                                <div>
                                  <div className="text-ink-3">Hash</div>
                                  <Mono copy={e.hash}>{e.hash}</Mono>
                                </div>
                                <div className="text-ink-3">{formatTimestamp(e.created_at)}</div>
                              </div>
                            </div>
                          </td>
                        </tr>
                      )}
                    </Fragment>
                  ))}
                </tbody>
              </Table>
              <div className="mt-4 flex items-center justify-between text-sm text-ink-3">
                <span>Newest first. The chain has {formatCount(page.head_seq)} entries.</span>
                <span className="flex gap-2">
                  <Button size="sm" disabled={cursors.length <= 1} onClick={() => setCursors(cursors.slice(0, -1))}>
                    Newer
                  </Button>
                  <Button size="sm" disabled={!page.next_before} onClick={() => setCursors([...cursors, page.next_before])}>
                    Older
                  </Button>
                </span>
              </div>
            </>
          )}
        </Panel>
        <Panel>
          <PanelHeader
            title="Signed checkpoints"
            description="Veto signs the chain head on a schedule with the workspace's Ed25519 key. Keep copies outside Veto to detect a rewritten history."
          />
          {!page?.checkpoints.length ? (
            <p className="text-sm text-ink-3">No checkpoints yet. One is written every 15 minutes while the chain grows.</p>
          ) : (
            <ul className="divide-y divide-line">
              {page.checkpoints.map((c) => (
                <li key={c.seq} className="py-3">
                  <div className="flex items-center justify-between">
                    <span className="text-sm font-medium text-ink">Entry {c.seq}</span>
                    {c.valid ? <Pill tone="proceed">Signature valid</Pill> : <Pill tone="stop">Signature invalid</Pill>}
                  </div>
                  <div className="mt-1 text-xs text-ink-3">
                    {formatRelative(c.created_at)}, key <Mono>{c.key_id}</Mono>
                  </div>
                  <Mono copy={c.signature} className="mt-1">
                    {shortHash(c.signature, 14)}
                  </Mono>
                </li>
              ))}
            </ul>
          )}
        </Panel>
      </div>
    </>
  );
}
