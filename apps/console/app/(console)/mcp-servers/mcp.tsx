"use client";

import { useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { Icon } from "@/components/icons";
import { Unplugged } from "@/components/illustrations";
import { useCan } from "@/components/shell/session";
import { Button } from "@/components/ui/button";
import { CodeBlock, Mono } from "@/components/ui/code";
import { ConfirmDialog, Dialog } from "@/components/ui/dialog";
import { Field, Input, Select, Switch } from "@/components/ui/form";
import { PageHeader, Panel, PanelHeader } from "@/components/ui/layout";
import { Pill } from "@/components/ui/pills";
import { EmptyState, ErrorState, LoadingPanel } from "@/components/ui/states";
import { Menu, MenuItem, Table, Td, Th } from "@/components/ui/table";
import { useToast } from "@/components/ui/toast";
import { api, errorMessage } from "@/lib/api";
import { formatRelative } from "@/lib/format";
import { useConnections, useMCPServers } from "@/lib/queries";
import type { MCPServer, MCPServerInput } from "@/lib/types";

export function McpPage() {
  const q = useMCPServers();
  const connections = useConnections();
  const canConfigure = useCan("configure");
  const qc = useQueryClient();
  const toast = useToast();
  const [editing, setEditing] = useState<MCPServer | "new" | null>(null);
  const [deleting, setDeleting] = useState<MCPServer | null>(null);
  const origin = typeof window === "undefined" ? "" : window.location.origin;
  const example = q.data?.items[0]?.name ?? "github";
  const connName = (id: string | null) => connections.data?.items.find((c) => c.id === id)?.name;

  return (
    <>
      <PageHeader
        title="MCP servers"
        description="Point an agent's MCP client at Veto instead of the server. Every tool call passes through Veto's decision before it reaches the server; everything else is forwarded as is."
        actions={
          canConfigure && (
            <Button variant="primary" icon={<Icon.Plus size={15} />} onClick={() => setEditing("new")}>
              Add server
            </Button>
          )
        }
      />
      <div className="grid items-start gap-5 xl:grid-cols-[minmax(0,1fr)_420px]">
        <Panel>
          {q.isError && <ErrorState error={q.error} onRetry={() => void q.refetch()} />}
          {q.isPending ? (
            <LoadingPanel />
          ) : !q.data?.items.length ? (
            <EmptyState art={<Unplugged />} title="No MCP servers">
              Add a Streamable HTTP MCP server. Agents then connect to Veto&rsquo;s address for it.
            </EmptyState>
          ) : (
            <Table>
              <thead>
                <tr>
                  <Th>Server</Th>
                  <Th>Agents connect to</Th>
                  <Th>Credentials</Th>
                  <Th className="text-right">Updated</Th>
                  <Th />
                </tr>
              </thead>
              <tbody>
                {q.data.items.map((m) => (
                  <tr key={m.id}>
                    <Td>
                      <div className="flex items-center gap-2">
                        <span className="font-medium text-ink">{m.name}</span>
                        {m.disabled_at && <Pill tone="stop">Disabled</Pill>}
                      </div>
                      <div className="max-w-64 truncate text-xs text-ink-3">{m.upstream_url}</div>
                    </Td>
                    <Td>
                      <Mono copy={`${origin}/mcp/${m.name}`}>{`/mcp/${m.name}`}</Mono>
                    </Td>
                    <Td className="text-ink-2">
                      {m.connection_id ? (connName(m.connection_id) ?? "Connection") : <span className="text-ink-3">None</span>}
                    </Td>
                    <Td className="text-right whitespace-nowrap text-ink-3">{formatRelative(m.updated_at)}</Td>
                    <Td className="text-right">
                      {canConfigure && (
                        <Menu
                          trigger={(p) => (
                            <button
                              {...p}
                              type="button"
                              aria-label="More"
                              className="inline-flex size-8 items-center justify-center rounded-[9px] text-ink-3 hover:bg-hover hover:text-ink"
                            >
                              <Icon.More size={16} />
                            </button>
                          )}
                        >
                          {(close) => (
                            <>
                              <MenuItem icon={<Icon.Edit size={15} />} onSelect={() => (close(), setEditing(m))}>
                                Edit
                              </MenuItem>
                              <MenuItem tone="danger" icon={<Icon.Trash size={15} />} onSelect={() => (close(), setDeleting(m))}>
                                Delete
                              </MenuItem>
                            </>
                          )}
                        </Menu>
                      )}
                    </Td>
                  </tr>
                ))}
              </tbody>
            </Table>
          )}
        </Panel>
        <Panel>
          <PanelHeader title="Connect a client" description="Use the agent's own Veto key, so its calls are attributed to it." />
          <CodeBlock
            code={`{
  "mcpServers": {
    "${example}": {
      "type": "http",
      "url": "${origin}/mcp/${example}",
      "headers": {
        "Authorization": "Bearer \${VETO_API_KEY}"
      }
    }
  }
}`}
          />
          <ul className="mt-4 space-y-2 text-sm text-ink-2">
            <li>Calls the decision model approves are forwarded, using the safer version when that was chosen.</li>
            <li>Held calls wait briefly for a reviewer, then return a tool error the agent can retry later.</li>
            <li>Stopped calls return a tool error that says why.</li>
          </ul>
        </Panel>
      </div>
      <ServerDialog server={editing} onClose={() => setEditing(null)} />
      <ConfirmDialog
        open={!!deleting}
        onClose={() => setDeleting(null)}
        title={`Delete ${deleting?.name}?`}
        body="Agents connected to it get errors until they are pointed elsewhere."
        confirmLabel="Delete server"
        onConfirm={async () => {
          await api.del(`/v1/mcp-servers/${deleting!.id}`);
          toast.success("Server deleted");
          void qc.invalidateQueries({ queryKey: ["mcp-servers"] });
        }}
      />
    </>
  );
}

function ServerDialog({ server, onClose }: { server: MCPServer | "new" | null; onClose: () => void }) {
  if (!server) return null;
  return <ServerForm key={server === "new" ? "new" : server.id} existing={server === "new" ? null : server} onClose={onClose} />;
}

function ServerForm({ existing, onClose }: { existing: MCPServer | null; onClose: () => void }) {
  const qc = useQueryClient();
  const toast = useToast();
  const connections = useConnections();
  const isNew = existing === null;
  const [form, setForm] = useState<MCPServerInput>(() =>
    existing
      ? {
          name: existing.name,
          upstream_url: existing.upstream_url,
          connection_id: existing.connection_id,
          disabled: !!existing.disabled_at,
        }
      : { name: "", upstream_url: "https://", connection_id: null, disabled: false },
  );
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function save() {
    setBusy(true);
    setError(null);
    try {
      if (isNew) await api.post("/v1/mcp-servers", form);
      else await api.patch(`/v1/mcp-servers/${existing!.id}`, form);
      toast.success(isNew ? `Server ${form.name} added` : "Server saved");
      void qc.invalidateQueries({ queryKey: ["mcp-servers"] });
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
      title={isNew ? "Add an MCP server" : `Edit ${existing?.name}`}
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            Cancel
          </Button>
          <Button variant="primary" busy={busy} onClick={() => void save()}>
            {isNew ? "Add server" : "Save changes"}
          </Button>
        </>
      }
    >
      <div className="space-y-4">
        <Field label="Name" hint="Lowercase letters, digits, - and _. Agents connect to /mcp/<name>.">
          {(id, d) => (
            <Input
              id={id}
              aria-describedby={d}
              disabled={!isNew}
              value={form.name}
              onChange={(e) => setForm({ ...form, name: e.target.value.trim() })}
            />
          )}
        </Field>
        <Field label="Server URL" hint="The Streamable HTTP endpoint of the MCP server.">
          {(id, d) => (
            <Input
              id={id}
              aria-describedby={d}
              value={form.upstream_url}
              onChange={(e) => setForm({ ...form, upstream_url: e.target.value.trim() })}
            />
          )}
        </Field>
        <Field label="Credentials" hint="Veto adds this connection's authentication to requests it forwards.">
          {(id, d) => (
            <Select
              id={id}
              aria-describedby={d}
              value={form.connection_id ?? ""}
              onChange={(e) => setForm({ ...form, connection_id: e.target.value || null })}
            >
              <option value="">None</option>
              {connections.data?.items.map((c) => (
                <option key={c.id} value={c.id}>
                  {c.name}
                </option>
              ))}
            </Select>
          )}
        </Field>
        {!isNew && (
          <Switch
            checked={form.disabled}
            onChange={(v) => setForm({ ...form, disabled: v })}
            label="Disable"
            description="Requests to this server fail until it is enabled again."
          />
        )}
        {error && <p className="text-sm text-stop">{error}</p>}
      </div>
    </Dialog>
  );
}
