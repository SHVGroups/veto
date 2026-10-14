"use client";

import { useQueryClient } from "@tanstack/react-query";
import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { useMemo, useState } from "react";
import { CodeEditor } from "@/components/editor/code-editor";
import { Icon } from "@/components/icons";
import { useCan } from "@/components/shell/session";
import { Button } from "@/components/ui/button";
import { ConfirmDialog } from "@/components/ui/dialog";
import { PageHeader, Panel, PanelHeader } from "@/components/ui/layout";
import { Pill } from "@/components/ui/pills";
import { ErrorState, LoadingPanel, Notice } from "@/components/ui/states";
import { Table, Tabs, Td, Th } from "@/components/ui/table";
import { useToast } from "@/components/ui/toast";
import { api, errorMessage } from "@/lib/api";
import { formatProbability, formatRelative, humanize, optionLabel, optionPhrase } from "@/lib/format";
import { parseJsonInput } from "@/lib/json";
import { usePacks, usePackVersions } from "@/lib/queries";
import type { Option, Pack } from "@/lib/types";

export function PackView({ name }: { name: string }) {
  const params = useSearchParams();
  const router = useRouter();
  const qc = useQueryClient();
  const toast = useToast();
  const canConfigure = useCan("configure");
  const packs = usePacks();
  const info = packs.data?.items.find((p) => p.name === name);
  const isNew = !info && params.get("new") === "1";
  const base = isNew ? packs.data?.items.find((p) => p.name === "general")?.definition : info?.definition;
  const versions = usePackVersions(name);
  const initial = useMemo(
    () =>
      base ? JSON.stringify({ ...base, name, version: isNew ? 1 : base.version, title: isNew ? humanize(name) : base.title }, null, 2) : "",
    [base, name, isNew],
  );
  const [tab, setTab] = useState<"overview" | "edit" | "history">(isNew ? "edit" : "overview");
  const [draft, setDraft] = useState<string | null>(null);
  const text = draft ?? initial;
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [resetting, setResetting] = useState(false);

  if (packs.isError) return <ErrorState error={packs.error} onRetry={() => void packs.refetch()} />;
  if (packs.isPending) return <LoadingPanel rows={8} />;
  if (!info && !isNew) return <ErrorState error={new Error(`There is no question pack named ${name}.`)} />;

  const parsed = parseJsonInput(text);
  const errLine = !parsed.ok ? Number(/line (\d+)/.exec(parsed.message)?.[1] ?? 0) : 0;

  async function save() {
    if (!parsed.ok) return setError(parsed.message);
    setBusy(true);
    setError(null);
    try {
      const res = await api.put<{ version: number }>(`/v1/packs/${name}`, parsed.value);
      toast.success(`${name} version ${res.version} saved`);
      await qc.invalidateQueries({ queryKey: ["packs"] });
      if (isNew) router.replace(`/packs/${name}`);
      setDraft(null);
      setTab("overview");
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  }

  const def = info?.definition;
  return (
    <>
      <PageHeader
        meta={
          <Link href="/packs" className="inline-flex items-center gap-1 hover:text-ink">
            <Icon.ChevronLeft size={14} />
            Question packs
          </Link>
        }
        title={
          <span className="flex items-center gap-3">
            {info?.title || humanize(name)}
            {isNew ? (
              <Pill tone="held">New</Pill>
            ) : info?.builtin ? (
              info.overridden ? (
                <Pill tone="safer">Built in, edited</Pill>
              ) : (
                <Pill>Built in</Pill>
              )
            ) : (
              <Pill tone="held">Custom</Pill>
            )}
          </span>
        }
        description={def?.description}
        actions={
          canConfigure &&
          info?.overridden && (
            <Button variant="ghost" onClick={() => setResetting(true)}>
              {info.builtin ? "Restore built-in version" : "Delete pack"}
            </Button>
          )
        }
      />
      <Tabs
        className="mb-5"
        value={tab}
        onChange={setTab}
        tabs={[
          ...(isNew ? [] : [{ value: "overview" as const, label: "Questions" }]),
          ...(canConfigure ? [{ value: "edit" as const, label: "Edit" }] : []),
          ...(isNew ? [] : [{ value: "history" as const, label: "History", count: versions.data?.items.length }]),
        ]}
      />

      {tab === "overview" && def && <PackOverview pack={def} tools={info?.tools ?? []} />}

      {tab === "edit" && (
        <div className="grid items-start gap-5 xl:grid-cols-[minmax(0,1fr)_340px]">
          <Panel>
            <PanelHeader
              title="Definition"
              description="Saving creates a new version for this workspace. Changes apply to the next call."
              actions={
                <Button variant="primary" busy={busy} disabled={!parsed.ok} onClick={() => void save()}>
                  Save version
                </Button>
              }
            />
            <CodeEditor
              language="json"
              label="Pack definition"
              value={text}
              onChange={setDraft}
              minHeight={520}
              errorLines={errLine ? [errLine] : []}
            />
            {(error || !parsed.ok) && (
              <p className="mt-3 rounded-[10px] bg-stop-wash px-3 py-2 text-sm text-stop">{error ?? (parsed.ok ? null : parsed.message)}</p>
            )}
          </Panel>
          <Panel>
            <PanelHeader title="How packs work" />
            <div className="space-y-3 text-sm text-ink-2">
              <p>
                <code className="text-ink">disposition</code> is the choice between the three options. Its{" "}
                <code className="text-ink">criteria</code> describe when each option is right.
              </p>
              <p>
                Each signal is a <code className="text-ink">noul</code> (yes or no with a probability), a{" "}
                <code className="text-ink">choice</code> or a <code className="text-ink">score</code>. Its{" "}
                <code className="text-ink">risk.weight</code> sets its share of the composite risk; <code className="text-ink">invert</code>{" "}
                counts a no as risky.
              </p>
              <p>
                <code className="text-ink">requires</code> skips a signal when the call lacks that input, such as a request.
              </p>
              <p>
                A guard rules options out when a signal crosses <code className="text-ink">above</code> or{" "}
                <code className="text-ink">below</code> a value, whatever the model chose.
              </p>
              <p>Questions refer to the state Veto sends: request, agent, tool, action, safer_variant and untrusted.</p>
            </div>
          </Panel>
        </div>
      )}

      {tab === "history" && (
        <Panel>
          {versions.isPending ? (
            <LoadingPanel />
          ) : versions.data?.items.length ? (
            <Table>
              <thead>
                <tr>
                  <Th>Version</Th>
                  <Th className="text-right">Signals</Th>
                  <Th className="text-right">Guards</Th>
                  <Th>Saved</Th>
                  <Th />
                </tr>
              </thead>
              <tbody>
                {versions.data.items.map((v) => (
                  <tr key={v.version}>
                    <Td className="font-medium">Version {v.version}</Td>
                    <Td className="figures text-right">{v.definition.signals.length}</Td>
                    <Td className="figures text-right">{v.definition.guards.length}</Td>
                    <Td className="text-ink-2">{formatRelative(v.created_at)}</Td>
                    <Td className="text-right">
                      {v.active ? (
                        <Pill tone="proceed" dot>
                          In use
                        </Pill>
                      ) : (
                        canConfigure && (
                          <Button
                            size="sm"
                            variant="ghost"
                            onClick={() => {
                              setDraft(JSON.stringify(v.definition, null, 2));
                              setTab("edit");
                            }}
                          >
                            Open in editor
                          </Button>
                        )
                      )}
                    </Td>
                  </tr>
                ))}
              </tbody>
            </Table>
          ) : (
            <Notice>
              {info?.builtin ? "This pack has not been edited in this workspace. The built-in version applies." : "No versions."}
            </Notice>
          )}
        </Panel>
      )}

      <ConfirmDialog
        open={resetting}
        onClose={() => setResetting(false)}
        title={info?.builtin ? "Restore the built-in pack?" : `Delete ${name}?`}
        body={
          info?.builtin
            ? "This workspace's edits stop applying. Earlier versions stay in the history and the ledger."
            : "Tools that use this pack must be moved to another pack first."
        }
        confirmLabel={info?.builtin ? "Restore" : "Delete pack"}
        onConfirm={async () => {
          await api.del(`/v1/packs/${name}`);
          toast.success(info?.builtin ? "Built-in pack restored" : "Pack deleted");
          await qc.invalidateQueries({ queryKey: ["packs"] });
          if (!info?.builtin) router.replace("/packs");
        }}
      />
    </>
  );
}

const optionOrder: Option[] = ["proceed", "proceed_safer", "stop"];

function PackOverview({ pack, tools }: { pack: Pack; tools: string[] }) {
  const totalWeight = pack.signals.reduce((s, x) => s + (x.risk?.weight ?? 0), 0);
  return (
    <div className="space-y-5">
      <Panel>
        <PanelHeader title="The decision" description={pack.disposition.instructions} />
        <div className="grid gap-3 md:grid-cols-3">
          {optionOrder.map((o) => (
            <div key={o} className="rounded-[14px] bg-sunken p-4 shadow-[inset_0_0_0_1px_var(--line)]">
              <div className="text-sm font-medium text-ink">{optionLabel[o]}</div>
              <p className="mt-1 text-sm text-ink-2">{pack.disposition.criteria[o] ?? "Not described."}</p>
            </div>
          ))}
        </div>
      </Panel>
      <Panel>
        <PanelHeader
          title="Signals"
          description="Questions answered alongside the decision. Weighted answers add up to the composite risk."
        />
        <Table>
          <thead>
            <tr>
              <Th>Signal</Th>
              <Th>Question</Th>
              <Th>Type</Th>
              <Th className="text-right">Share of risk</Th>
            </tr>
          </thead>
          <tbody>
            {pack.signals.map((s) => (
              <tr key={s.id}>
                <Td className="font-medium whitespace-nowrap">{humanize(s.id)}</Td>
                <Td className="max-w-xl text-ink-2">
                  {s.instructions}
                  {s.requires?.length ? <div className="mt-0.5 text-xs text-ink-3">Asked only with: {s.requires.join(", ")}</div> : null}
                </Td>
                <Td className="text-ink-2">
                  {s.type === "noul"
                    ? "Yes or no"
                    : s.type === "choice"
                      ? `One of ${s.choices?.length ?? 0}`
                      : `Score of ${s.levels?.length ?? 0}`}
                  {s.risk?.invert && <div className="text-xs text-ink-3">No is risky</div>}
                </Td>
                <Td className="figures text-right">
                  {s.risk && totalWeight ? (
                    `${Math.round((s.risk.weight / totalWeight) * 100)}%`
                  ) : (
                    <span className="text-ink-3">Guard only</span>
                  )}
                </Td>
              </tr>
            ))}
          </tbody>
        </Table>
      </Panel>
      {pack.guards.length > 0 && (
        <Panel>
          <PanelHeader title="Guards" description="Hard limits that rule options out regardless of the model's choice." />
          <ul className="space-y-2">
            {pack.guards.map((g, i) => (
              <li key={i} className="rounded-[12px] bg-sunken px-4 py-3 text-sm shadow-[inset_0_0_0_1px_var(--line)]">
                <span className="font-medium text-ink">{g.reason}</span>
                <span className="block text-ink-2">
                  When {humanize(g.signal).toLowerCase()} is{" "}
                  {g.above !== undefined ? `above ${formatProbability(g.above)}` : `below ${formatProbability(g.below)}`}, rule out{" "}
                  {g.forbid.map((o) => optionPhrase[o]).join(" and ")}.
                </span>
              </li>
            ))}
          </ul>
        </Panel>
      )}
      <p className="text-sm text-ink-3">
        {tools.length ? `Used by ${tools.join(", ")}.` : "No tools use this pack yet. Choose it on a tool's page."}
      </p>
    </div>
  );
}
