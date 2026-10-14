"use client";

import type { Route } from "next";
import { useRouter, useSearchParams } from "next/navigation";
import { Suspense, useMemo, useState } from "react";
import { Icon } from "@/components/icons";
import { EmptyRows } from "@/components/illustrations";
import { useCan } from "@/components/shell/session";
import { Button } from "@/components/ui/button";
import { Dialog } from "@/components/ui/dialog";
import { Field, Input } from "@/components/ui/form";
import { PageHeader, Panel } from "@/components/ui/layout";
import { Pill, RiskTag } from "@/components/ui/pills";
import { EmptyState, ErrorState, LoadingPanel } from "@/components/ui/states";
import { LinkRow, RowLink, Table, Tabs, Td, Th } from "@/components/ui/table";
import { formatCount, formatProbability, formatRelative } from "@/lib/format";
import { useTools } from "@/lib/queries";
import type { RiskClass } from "@/lib/types";

type Filter = "all" | "unregistered" | RiskClass;

export function ToolsPage() {
  return (
    <Suspense>
      <ToolsList />
    </Suspense>
  );
}

function ToolsList() {
  const router = useRouter();
  const params = useSearchParams();
  const filter = (params.get("filter") ?? "all") as Filter;
  const q = useTools();
  const items = useMemo(() => q.data?.items ?? [], [q.data]);
  const shown = useMemo(
    () =>
      items.filter((t) => (filter === "all" ? true : filter === "unregistered" ? !t.registered : t.registered && t.risk_class === filter)),
    [items, filter],
  );
  const unregistered = items.filter((t) => !t.registered).length;
  const canConfigure = useCan("configure");
  const [adding, setAdding] = useState(false);
  const [name, setName] = useState("");

  return (
    <>
      <PageHeader
        title="Tools"
        description="Every tool your agents call. A tool's risk class sets how sure the decision model must be before it acts alone; its rules build the safer version."
        actions={
          canConfigure && (
            <Button variant="primary" icon={<Icon.Plus size={15} />} onClick={() => setAdding(true)}>
              Add tool
            </Button>
          )
        }
      />
      <Tabs
        className="mb-5"
        value={filter}
        onChange={(v) => router.replace((v === "all" ? "/tools" : `/tools?filter=${v}`) as Route, { scroll: false })}
        tabs={[
          { value: "all", label: "All", count: items.length },
          { value: "unregistered", label: "Needs a risk class", count: unregistered },
          { value: "read", label: "Read" },
          { value: "low", label: "Low" },
          { value: "medium", label: "Medium" },
          { value: "high", label: "High" },
          { value: "critical", label: "Critical" },
        ]}
      />
      <Panel>
        {q.isError && <ErrorState error={q.error} onRetry={() => void q.refetch()} />}
        {q.isPending ? (
          <LoadingPanel rows={6} />
        ) : shown.length === 0 ? (
          <EmptyState art={<EmptyRows />} title={filter === "unregistered" ? "Every tool has a risk class" : "No tools here yet"}>
            {filter === "unregistered"
              ? "New tools appear here the first time an agent calls them."
              : "Tools are added when an agent first calls them, or you can add one ahead of time."}
          </EmptyState>
        ) : (
          <Table>
            <thead>
              <tr>
                <Th>Tool</Th>
                <Th>Risk</Th>
                <Th>Pack</Th>
                <Th className="text-right">Calls, 7 days</Th>
                <Th className="text-right">Held</Th>
                <Th className="text-right">Stopped</Th>
                <Th className="text-right">Safer taken</Th>
                <Th className="text-right">Avg. confidence</Th>
                <Th className="text-right">Last call</Th>
              </tr>
            </thead>
            <tbody>
              {shown.map((t) => (
                <LinkRow key={t.name} href={`/tools/${t.name}`}>
                  <Td>
                    <RowLink href={`/tools/${t.name}`}>{t.name}</RowLink>
                    <div className="max-w-80 truncate text-xs text-ink-3">{t.description || "No description"}</div>
                  </Td>
                  <Td>
                    {t.registered ? (
                      <RiskTag risk={t.risk_class} />
                    ) : (
                      <Pill tone="held" dot>
                        Needs a risk class
                      </Pill>
                    )}
                  </Td>
                  <Td className="text-ink-2">
                    {t.pack}
                    {t.safer && t.safer.length > 0 && <div className="text-xs text-ink-3">{t.safer.length} safer rules</div>}
                  </Td>
                  <Td className="figures text-right">{formatCount(t.calls_7d)}</Td>
                  <Td className="figures text-right text-held">{formatCount(t.held_7d)}</Td>
                  <Td className="figures text-right text-stop">{formatCount(t.denied_7d)}</Td>
                  <Td className="figures text-right text-safer">{formatCount(t.safer_taken_7d)}</Td>
                  <Td className="figures text-right text-ink-2">{formatProbability(t.avg_confidence_7d)}</Td>
                  <Td className="text-right whitespace-nowrap text-ink-3">
                    {t.last_called_at ? formatRelative(t.last_called_at) : "Never"}
                  </Td>
                </LinkRow>
              ))}
            </tbody>
          </Table>
        )}
      </Panel>
      <Dialog
        open={adding}
        onClose={() => setAdding(false)}
        title="Add a tool"
        description="Classify a tool before any agent calls it. Use the exact name agents send."
        width="sm"
        footer={
          <>
            <Button variant="ghost" onClick={() => setAdding(false)}>
              Cancel
            </Button>
            <Button variant="primary" disabled={!TOOL_NAME.test(name)} onClick={() => router.push(`/tools/${name}` as Route)}>
              Continue
            </Button>
          </>
        }
      >
        <Field label="Tool name" hint="For example gmail.send or github/merge_pull_request.">
          {(id, d) => <Input id={id} aria-describedby={d} autoFocus value={name} onChange={(e) => setName(e.target.value.trim())} />}
        </Field>
      </Dialog>
    </>
  );
}

const TOOL_NAME = /^[A-Za-z0-9][A-Za-z0-9_.:/-]{0,199}$/;
