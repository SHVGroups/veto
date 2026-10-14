"use client";

import { useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { Icon } from "@/components/icons";
import { EmptyRows } from "@/components/illustrations";
import { useCan } from "@/components/shell/session";
import { Button } from "@/components/ui/button";
import { Dialog } from "@/components/ui/dialog";
import { Field, Input, SecretReveal, Textarea } from "@/components/ui/form";
import { PageHeader, Panel } from "@/components/ui/layout";
import { Pill } from "@/components/ui/pills";
import { EmptyState, ErrorState, LoadingPanel } from "@/components/ui/states";
import { LinkRow, RowLink, Table, Td, Th } from "@/components/ui/table";
import { api, errorMessage } from "@/lib/api";
import { formatCount, formatPercent, formatRelative } from "@/lib/format";
import { useAgents } from "@/lib/queries";
import type { Agent, IssuedKey } from "@/lib/types";

export function AgentsPage() {
  const q = useAgents();
  const canConfigure = useCan("configure");
  const [creating, setCreating] = useState(false);
  return (
    <>
      <PageHeader
        title="Agents"
        description="Each agent that calls Veto has its own identity and keys, so every action is attributed and can be cut off on its own."
        actions={
          canConfigure && (
            <Button variant="primary" icon={<Icon.Plus size={15} />} onClick={() => setCreating(true)}>
              Add agent
            </Button>
          )
        }
      />
      <Panel>
        {q.isError && <ErrorState error={q.error} onRetry={() => void q.refetch()} />}
        {q.isPending ? (
          <LoadingPanel rows={5} />
        ) : q.data && q.data.items.length === 0 ? (
          <EmptyState
            art={<EmptyRows />}
            title="No agents yet"
            action={
              canConfigure && (
                <Button variant="primary" onClick={() => setCreating(true)}>
                  Add your first agent
                </Button>
              )
            }
          >
            Add an agent to get an API key it uses to send its actions to Veto.
          </EmptyState>
        ) : (
          <Table>
            <thead>
              <tr>
                <Th>Agent</Th>
                <Th className="text-right">Actions, 7 days</Th>
                <Th className="text-right">Held</Th>
                <Th className="text-right">Stopped</Th>
                <Th className="text-right">Decided alone</Th>
                <Th className="text-right">Waiting now</Th>
                <Th>Most used tools</Th>
                <Th className="text-right">Last seen</Th>
              </tr>
            </thead>
            <tbody>
              {q.data?.items.map((a) => (
                <LinkRow key={a.id} href={`/agents/${a.id}`}>
                  <Td>
                    <div className="flex items-center gap-2">
                      <RowLink href={`/agents/${a.id}`}>{a.name}</RowLink>
                      {a.disabled_at && <Pill tone="stop">Disabled</Pill>}
                    </div>
                    <div className="max-w-72 truncate text-xs text-ink-3">{[a.vendor, a.description].filter(Boolean).join(". ")}</div>
                  </Td>
                  <Td className="figures text-right">{formatCount(a.actions_7d)}</Td>
                  <Td className="figures text-right text-held">{formatCount(a.held_7d)}</Td>
                  <Td className="figures text-right text-stop">{formatCount(a.denied_7d)}</Td>
                  <Td className="figures text-right text-ink-2">{formatPercent(a.auto_rate_7d)}</Td>
                  <Td className="figures text-right">
                    {a.pending_now ? <Pill tone="held">{a.pending_now}</Pill> : <span className="text-ink-3">0</span>}
                  </Td>
                  <Td className="max-w-60 truncate text-xs text-ink-2">{a.top_tools_7d?.join(", ") || "–"}</Td>
                  <Td className="text-right whitespace-nowrap text-ink-3">{a.last_seen_at ? formatRelative(a.last_seen_at) : "Never"}</Td>
                </LinkRow>
              ))}
            </tbody>
          </Table>
        )}
      </Panel>
      <CreateAgentDialog open={creating} onClose={() => setCreating(false)} />
    </>
  );
}

function CreateAgentDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  const qc = useQueryClient();
  const [form, setForm] = useState({ name: "", vendor: "", description: "" });
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [created, setCreated] = useState<{ agent: Agent; key: IssuedKey } | null>(null);

  function close() {
    onClose();
    setTimeout(() => {
      setForm({ name: "", vendor: "", description: "" });
      setCreated(null);
      setError(null);
    }, 200);
  }

  async function submit() {
    setBusy(true);
    setError(null);
    try {
      const res = await api.post<{ agent: Agent; key: IssuedKey }>("/v1/agents", form);
      setCreated(res);
      void qc.invalidateQueries({ queryKey: ["agents"] });
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <Dialog
      open={open}
      onClose={close}
      title={created ? `${created.agent.name} is ready` : "Add an agent"}
      description={
        created
          ? "Give this key to the agent. It authenticates every action the agent sends."
          : "Name it after what it does, so actions are easy to read."
      }
      footer={
        created ? (
          <Button variant="primary" onClick={close}>
            Done
          </Button>
        ) : (
          <>
            <Button variant="ghost" onClick={close}>
              Cancel
            </Button>
            <Button variant="primary" busy={busy} onClick={() => void submit()} disabled={!form.name.trim()}>
              Add agent
            </Button>
          </>
        )
      }
    >
      {created ? (
        <div className="space-y-4">
          <SecretReveal label="Agent API key" secret={created.key.secret} />
          <p className="text-sm text-ink-2">
            Set it as <code className="text-ink">VETO_API_KEY</code> where the agent runs, then send each tool call to{" "}
            <code className="text-ink">POST /v1/actions</code> before running it. The SDKs do this for you.
          </p>
        </div>
      ) : (
        <form
          className="space-y-4"
          onSubmit={(e) => {
            e.preventDefault();
            void submit();
          }}
        >
          <Field label="Name" hint="Up to 64 letters, digits, spaces, dots, dashes and underscores. For example support-bot.">
            {(id, d) => (
              <Input
                id={id}
                aria-describedby={d}
                autoFocus
                value={form.name}
                onChange={(e) => setForm({ ...form, name: e.target.value })}
              />
            )}
          </Field>
          <Field label="Built on" optional hint="The model or platform behind it, for your reference.">
            {(id, d) => (
              <Input id={id} aria-describedby={d} value={form.vendor} onChange={(e) => setForm({ ...form, vendor: e.target.value })} />
            )}
          </Field>
          <Field label="What it does" optional>
            {(id) => (
              <Textarea id={id} rows={2} value={form.description} onChange={(e) => setForm({ ...form, description: e.target.value })} />
            )}
          </Field>
          {error && <p className="text-sm text-stop">{error}</p>}
          <button type="submit" hidden />
        </form>
      )}
    </Dialog>
  );
}
