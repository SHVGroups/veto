"use client";

import { useQueryClient } from "@tanstack/react-query";
import Link from "next/link";
import { useState } from "react";
import { Icon } from "@/components/icons";
import { useCan } from "@/components/shell/session";
import { Button } from "@/components/ui/button";
import { CodeBlock, Mono } from "@/components/ui/code";
import { ConfirmDialog, Dialog } from "@/components/ui/dialog";
import { Field, Input, SecretReveal, Switch, Textarea } from "@/components/ui/form";
import { Details, PageHeader, Panel, PanelHeader, Stat } from "@/components/ui/layout";
import { DecisionChip, Pill } from "@/components/ui/pills";
import { ErrorState, LoadingPanel } from "@/components/ui/states";
import { LinkRow, RowLink, Table, Td, Th } from "@/components/ui/table";
import { useToast } from "@/components/ui/toast";
import { api, errorMessage } from "@/lib/api";
import { formatCount, formatDate, formatPercent, formatRelative } from "@/lib/format";
import { useAgent } from "@/lib/queries";
import type { AgentDetail, APIKey, IssuedKey } from "@/lib/types";

export function AgentView({ id }: { id: string }) {
  const q = useAgent(id);
  const a = q.data;
  const canConfigure = useCan("configure");
  const [editing, setEditing] = useState(false);
  const [issuing, setIssuing] = useState(false);

  if (q.isError) return <ErrorState error={q.error} onRetry={() => void q.refetch()} />;
  if (!a) return <LoadingPanel rows={8} />;

  const origin = typeof window === "undefined" ? "https://veto.example.com" : window.location.origin;

  return (
    <>
      <PageHeader
        meta={
          <Link href="/agents" className="inline-flex items-center gap-1 hover:text-ink">
            <Icon.ChevronLeft size={14} />
            Agents
          </Link>
        }
        title={
          <span className="flex items-center gap-3">
            {a.name}
            {a.disabled_at && <Pill tone="stop">Disabled</Pill>}
          </span>
        }
        description={[a.vendor, a.description].filter(Boolean).join(". ") || "No description."}
        actions={
          canConfigure && (
            <Button icon={<Icon.Edit size={15} />} onClick={() => setEditing(true)}>
              Edit
            </Button>
          )
        }
      />

      <div className="space-y-5">
        <Panel className="grid grid-cols-2 gap-6 md:grid-cols-5">
          <Stat label="Actions, 7 days" value={formatCount(a.actions_7d)} />
          <Stat label="Held" value={formatCount(a.held_7d)} tone="held" />
          <Stat label="Stopped" value={formatCount(a.denied_7d)} tone="stop" />
          <Stat label="Decided without a person" value={formatPercent(a.auto_rate_7d)} />
          <Stat
            label="Waiting now"
            value={formatCount(a.pending_now)}
            context={a.last_seen_at ? `Last seen ${formatRelative(a.last_seen_at)}` : "Never seen"}
          />
        </Panel>

        <div className="grid items-start gap-5 xl:grid-cols-[minmax(0,1fr)_420px]">
          <Panel>
            <PanelHeader
              title="Recent actions"
              actions={
                <Link href={`/actions?agent=${a.id}`} className="inline-flex items-center gap-1 text-sm text-ink-2 hover:text-ink">
                  All
                  <Icon.ChevronRight size={13} />
                </Link>
              }
            />
            {a.recent.length === 0 ? (
              <p className="py-6 text-sm text-ink-3">
                This agent has not sent an action yet. The snippet on the right sends its first one.
              </p>
            ) : (
              <Table>
                <thead>
                  <tr>
                    <Th>Tool</Th>
                    <Th>Request</Th>
                    <Th>Result</Th>
                    <Th className="text-right">Received</Th>
                  </tr>
                </thead>
                <tbody>
                  {a.recent.map((r) => (
                    <LinkRow key={r.id} href={`/actions/${r.id}`}>
                      <Td>
                        <RowLink href={`/actions/${r.id}`}>{r.tool}</RowLink>
                      </Td>
                      <Td className="max-w-72 truncate text-ink-2">{r.request || "–"}</Td>
                      <Td>
                        <DecisionChip status={r.status} option={r.option} />
                      </Td>
                      <Td className="text-right whitespace-nowrap text-ink-3">{formatRelative(r.received_at)}</Td>
                    </LinkRow>
                  ))}
                </tbody>
              </Table>
            )}
          </Panel>

          <div className="space-y-5">
            <Panel>
              <PanelHeader
                title="API keys"
                description="Agents authenticate with a key. Revoking one stops its use at once."
                actions={
                  canConfigure && (
                    <Button size="sm" icon={<Icon.Plus size={14} />} onClick={() => setIssuing(true)}>
                      New key
                    </Button>
                  )
                }
              />
              <KeyList keys={a.keys} agentId={a.id} />
            </Panel>
            <Panel>
              <PanelHeader title="Send an action" description="Ask Veto before the agent runs a tool." />
              <CodeBlock
                code={`curl -s ${origin}/v1/actions?wait=10s \\
  -H "Authorization: Bearer $VETO_API_KEY" \\
  -H "Content-Type: application/json" \\
  -d '{
    "tool": "gmail.send",
    "args": {"to": ["ops@example.com"], "subject": "Report"},
    "request": "Send the weekly report to ops"
  }'`}
              />
              <p className="mt-3 text-xs text-ink-3">
                The answer says whether to run the call and carries a grant bound to the approved arguments. See the quickstart in the
                documentation.
              </p>
            </Panel>
          </div>
        </div>
      </div>

      <EditAgentDialog agent={a} open={editing} onClose={() => setEditing(false)} />
      <IssueKeyDialog agentId={a.id} open={issuing} onClose={() => setIssuing(false)} />
    </>
  );
}

export function KeyList({ keys, agentId }: { keys: APIKey[]; agentId?: string }) {
  const qc = useQueryClient();
  const toast = useToast();
  const canConfigure = useCan("configure");
  const [revoking, setRevoking] = useState<APIKey | null>(null);
  if (keys.length === 0) return <p className="text-sm text-ink-3">No keys.</p>;
  return (
    <>
      <ul className="divide-y divide-line">
        {keys.map((k) => (
          <li key={k.id} className="flex items-center gap-3 py-3">
            <Icon.Key size={16} className="shrink-0 text-ink-3" />
            <div className="min-w-0 flex-1">
              <div className="flex items-center gap-2">
                <span className="truncate text-sm font-medium text-ink">{k.name}</span>
                {k.revoked_at && <Pill tone="muted">Revoked</Pill>}
              </div>
              <div className="text-xs text-ink-3">
                <Mono>{k.prefix}…</Mono> created {formatDate(k.created_at)},{" "}
                {k.last_used_at ? `used ${formatRelative(k.last_used_at)}` : "never used"}
              </div>
            </div>
            {canConfigure && !k.revoked_at && (
              <Button size="sm" variant="ghost" onClick={() => setRevoking(k)}>
                Revoke
              </Button>
            )}
          </li>
        ))}
      </ul>
      <ConfirmDialog
        open={!!revoking}
        onClose={() => setRevoking(null)}
        title={`Revoke ${revoking?.name}?`}
        body="Requests with this key fail from now on. Anything using it needs a new key."
        confirmLabel="Revoke key"
        onConfirm={async () => {
          await api.del(`/v1/keys/${revoking!.id}`);
          toast.success("Key revoked");
          void qc.invalidateQueries({ queryKey: ["agents"] });
          void qc.invalidateQueries({ queryKey: ["keys"] });
          if (agentId) void qc.invalidateQueries({ queryKey: ["agents", agentId] });
        }}
      />
    </>
  );
}

function EditAgentDialog({ agent, open, onClose }: { agent: AgentDetail; open: boolean; onClose: () => void }) {
  return open ? <EditAgentForm agent={agent} onClose={onClose} /> : null;
}

function EditAgentForm({ agent, onClose }: { agent: AgentDetail; onClose: () => void }) {
  const qc = useQueryClient();
  const toast = useToast();
  const [form, setForm] = useState({
    name: agent.name,
    vendor: agent.vendor,
    description: agent.description,
    disabled: !!agent.disabled_at,
  });
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function save() {
    setBusy(true);
    setError(null);
    try {
      await api.patch(`/v1/agents/${agent.id}`, form);
      toast.success("Agent saved");
      void qc.invalidateQueries({ queryKey: ["agents"] });
      onClose();
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <Dialog
      open
      onClose={onClose}
      title="Edit agent"
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            Cancel
          </Button>
          <Button variant="primary" busy={busy} onClick={() => void save()}>
            Save changes
          </Button>
        </>
      }
    >
      <div className="space-y-4">
        <Field label="Name">
          {(id) => <Input id={id} value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} />}
        </Field>
        <Field label="Built on" optional>
          {(id) => <Input id={id} value={form.vendor} onChange={(e) => setForm({ ...form, vendor: e.target.value })} />}
        </Field>
        <Field label="What it does" optional>
          {(id) => (
            <Textarea id={id} rows={2} value={form.description} onChange={(e) => setForm({ ...form, description: e.target.value })} />
          )}
        </Field>
        <Switch
          checked={form.disabled}
          onChange={(v) => setForm({ ...form, disabled: v })}
          label="Disable this agent"
          description="Every action it sends is stopped until it is enabled again."
        />
        {error && <p className="text-sm text-stop">{error}</p>}
        <Details
          rows={[
            [
              "Agent id",
              <Mono key="i" copy={agent.id}>
                {agent.id}
              </Mono>,
            ],
          ]}
        />
      </div>
    </Dialog>
  );
}

function IssueKeyDialog({ agentId, open, onClose }: { agentId: string; open: boolean; onClose: () => void }) {
  const qc = useQueryClient();
  const [name, setName] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [key, setKey] = useState<IssuedKey | null>(null);

  function close() {
    onClose();
    setTimeout(() => {
      setName("");
      setKey(null);
      setError(null);
    }, 200);
  }

  async function issue() {
    setBusy(true);
    setError(null);
    try {
      const k = await api.post<IssuedKey>(`/v1/agents/${agentId}/keys`, { name });
      setKey(k);
      void qc.invalidateQueries({ queryKey: ["agents", agentId] });
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
      title={key ? "New key" : "Create a key"}
      description={key ? undefined : "Use one key per deployment, so you can rotate them independently."}
      footer={
        key ? (
          <Button variant="primary" onClick={close}>
            Done
          </Button>
        ) : (
          <>
            <Button variant="ghost" onClick={close}>
              Cancel
            </Button>
            <Button variant="primary" busy={busy} onClick={() => void issue()}>
              Create key
            </Button>
          </>
        )
      }
    >
      {key ? (
        <SecretReveal label={key.name} secret={key.secret} />
      ) : (
        <div className="space-y-3">
          <Field label="Name" hint="Where the key is used, such as production or staging.">
            {(id, d) => <Input id={id} aria-describedby={d} autoFocus value={name} onChange={(e) => setName(e.target.value)} />}
          </Field>
          {error && <p className="text-sm text-stop">{error}</p>}
        </div>
      )}
    </Dialog>
  );
}
