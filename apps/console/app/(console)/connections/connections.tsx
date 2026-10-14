"use client";

import { useQueryClient } from "@tanstack/react-query";
import Link from "next/link";
import type { Route } from "next";
import { useState } from "react";
import { Icon } from "@/components/icons";
import { Unplugged } from "@/components/illustrations";
import { useCan } from "@/components/shell/session";
import { Button } from "@/components/ui/button";
import { CodeBlock } from "@/components/ui/code";
import { ConfirmDialog, Dialog } from "@/components/ui/dialog";
import { Checkbox, Field, Input, Select, Switch, Textarea } from "@/components/ui/form";
import { PageHeader, Panel, PanelHeader } from "@/components/ui/layout";
import { Pill } from "@/components/ui/pills";
import { EmptyState, ErrorState, LoadingPanel } from "@/components/ui/states";
import { Menu, MenuItem, Table, Td, Th } from "@/components/ui/table";
import { useToast } from "@/components/ui/toast";
import { api, errorMessage } from "@/lib/api";
import { formatRelative } from "@/lib/format";
import { useConnections, useStatus } from "@/lib/queries";
import type { Connection, ConnectionInput } from "@/lib/types";

const METHODS = ["GET", "POST", "PUT", "PATCH", "DELETE"];
const authLabel: Record<string, string> = {
  none: "No authentication",
  bearer: "Bearer token",
  header: "Custom header",
  basic: "Basic authentication",
};

export function ConnectionsPage() {
  const q = useConnections();
  const canConfigure = useCan("configure");
  const [editing, setEditing] = useState<Connection | "new" | null>(null);
  const [deleting, setDeleting] = useState<Connection | null>(null);
  const qc = useQueryClient();
  const toast = useToast();
  const origin = typeof window === "undefined" ? "" : window.location.origin;
  const example = q.data?.items[0]?.name ?? "zendesk";

  return (
    <>
      <PageHeader
        title="Connections"
        description="Credentials Veto holds for your agents. An agent asks Veto to make the HTTP call, so it never sees the secret, and a call runs only after it is approved."
        actions={
          canConfigure && (
            <Button variant="primary" icon={<Icon.Plus size={15} />} onClick={() => setEditing("new")}>
              Add connection
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
            <EmptyState art={<Unplugged />} title="No connections">
              Add an API your agents use. Veto stores the secret encrypted and attaches it only to approved calls.
            </EmptyState>
          ) : (
            <Table>
              <thead>
                <tr>
                  <Th>Connection</Th>
                  <Th>Authentication</Th>
                  <Th>Methods</Th>
                  <Th>Tool</Th>
                  <Th className="text-right">Updated</Th>
                  <Th />
                </tr>
              </thead>
              <tbody>
                {q.data.items.map((c) => (
                  <tr key={c.id}>
                    <Td>
                      <div className="flex items-center gap-2">
                        <span className="font-medium text-ink">{c.name}</span>
                        {c.disabled_at && <Pill tone="stop">Disabled</Pill>}
                        {c.allow_private && <Pill tone="safer">Private network</Pill>}
                      </div>
                      <div className="max-w-72 truncate text-xs text-ink-3">{c.base_url}</div>
                    </Td>
                    <Td className="text-ink-2">
                      {authLabel[c.auth_scheme] ?? c.auth_scheme}
                      {c.auth_scheme !== "none" && (
                        <div className="text-xs text-ink-3">
                          {c.has_secret ? (c.rotated_at ? `Rotated ${formatRelative(c.rotated_at)}` : "Secret stored") : "No secret"}
                        </div>
                      )}
                    </Td>
                    <Td className="text-xs text-ink-2">{c.allow_methods.join(", ")}</Td>
                    <Td>
                      <Link href={`/tools/http.${c.name}` as Route} className="text-sm text-ink hover:underline">
                        http.{c.name}
                      </Link>
                    </Td>
                    <Td className="text-right whitespace-nowrap text-ink-3">{formatRelative(c.updated_at)}</Td>
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
                              <MenuItem icon={<Icon.Edit size={15} />} onSelect={() => (close(), setEditing(c))}>
                                Edit
                              </MenuItem>
                              <MenuItem tone="danger" icon={<Icon.Trash size={15} />} onSelect={() => (close(), setDeleting(c))}>
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
          <PanelHeader
            title="How an agent uses one"
            description={`Each connection is the tool http.${example}. The agent proposes a request; Veto decides, then runs it.`}
          />
          <CodeBlock
            code={`# 1. Propose the request
curl -s "${origin}/v1/actions?wait=10s" \\
  -H "Authorization: Bearer $VETO_API_KEY" \\
  -H "Content-Type: application/json" \\
  -d '{"tool": "http.${example}",
       "args": {"method": "POST", "path": "/tickets/42/comments",
                "body": {"text": "Refund issued"}},
       "request": "Tell the customer the refund went out"}'

# 2. If approved, have Veto run it with the stored secret
curl -s -X POST "${origin}/v1/actions/<id>/execute" \\
  -H "Authorization: Bearer $VETO_API_KEY"`}
          />
        </Panel>
      </div>
      <ConnectionDialog connection={editing} onClose={() => setEditing(null)} />
      <ConfirmDialog
        open={!!deleting}
        onClose={() => setDeleting(null)}
        title={`Delete ${deleting?.name}?`}
        body="Its secret is destroyed. Approved calls that have not run yet will fail."
        confirmLabel="Delete connection"
        typeToConfirm={deleting?.name}
        onConfirm={async () => {
          await api.del(`/v1/connections/${deleting!.id}`);
          toast.success("Connection deleted");
          void qc.invalidateQueries({ queryKey: ["connections"] });
        }}
      />
    </>
  );
}

function ConnectionDialog({ connection, onClose }: { connection: Connection | "new" | null; onClose: () => void }) {
  if (!connection) return null;
  return (
    <ConnectionForm
      key={connection === "new" ? "new" : connection.id}
      existing={connection === "new" ? null : connection}
      onClose={onClose}
    />
  );
}

function ConnectionForm({ existing, onClose }: { existing: Connection | null; onClose: () => void }) {
  const qc = useQueryClient();
  const toast = useToast();
  const status = useStatus();
  const isNew = existing === null;
  const [form, setForm] = useState<ConnectionInput>(() =>
    existing
      ? {
          name: existing.name,
          description: existing.description,
          base_url: existing.base_url,
          auth_scheme: existing.auth_scheme,
          header_name: existing.header_name,
          allow_methods: existing.allow_methods,
          allow_private: existing.allow_private,
          disabled: !!existing.disabled_at,
        }
      : {
          name: "",
          description: "",
          base_url: "https://",
          auth_scheme: "bearer",
          header_name: "",
          allow_methods: ["GET", "POST"],
          allow_private: false,
          disabled: false,
        },
  );
  const [secret, setSecret] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function save() {
    setBusy(true);
    setError(null);
    const body: ConnectionInput = { ...form, ...(secret ? { secret } : {}) };
    try {
      if (isNew) await api.post("/v1/connections", body);
      else await api.patch(`/v1/connections/${existing!.id}`, body);
      toast.success(isNew ? `Connection ${form.name} added` : "Connection saved");
      void qc.invalidateQueries({ queryKey: ["connections"] });
      void qc.invalidateQueries({ queryKey: ["tools"] });
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
      title={isNew ? "Add a connection" : `Edit ${existing?.name}`}
      width="lg"
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            Cancel
          </Button>
          <Button variant="primary" busy={busy} onClick={() => void save()}>
            {isNew ? "Add connection" : "Save changes"}
          </Button>
        </>
      }
    >
      <div className="grid gap-4 sm:grid-cols-2">
        <Field label="Name" hint="Lowercase letters, digits, - and _. The tool is http.<name>.">
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
        <Field label="Base URL" hint="Every request path is joined to this.">
          {(id, d) => (
            <Input
              id={id}
              aria-describedby={d}
              value={form.base_url}
              onChange={(e) => setForm({ ...form, base_url: e.target.value.trim() })}
            />
          )}
        </Field>
        <Field label="Description" optional className="sm:col-span-2">
          {(id) => (
            <Textarea id={id} rows={2} value={form.description} onChange={(e) => setForm({ ...form, description: e.target.value })} />
          )}
        </Field>
        <Field label="Authentication">
          {(id) => (
            <Select id={id} value={form.auth_scheme} onChange={(e) => setForm({ ...form, auth_scheme: e.target.value })}>
              {Object.entries(authLabel).map(([k, v]) => (
                <option key={k} value={k}>
                  {v}
                </option>
              ))}
            </Select>
          )}
        </Field>
        {form.auth_scheme === "header" ? (
          <Field label="Header name">
            {(id) => (
              <Input
                id={id}
                placeholder="X-Api-Key"
                value={form.header_name}
                onChange={(e) => setForm({ ...form, header_name: e.target.value })}
              />
            )}
          </Field>
        ) : (
          <div />
        )}
        {form.auth_scheme !== "none" && (
          <Field
            label="Secret"
            className="sm:col-span-2"
            hint={
              existing?.has_secret
                ? "Leave empty to keep the stored secret. A new value replaces it."
                : form.auth_scheme === "basic"
                  ? "As user:password."
                  : "Stored encrypted. It cannot be read back."
            }
          >
            {(id, d) => (
              <Input
                id={id}
                aria-describedby={d}
                type="password"
                autoComplete="off"
                value={secret}
                onChange={(e) => setSecret(e.target.value)}
              />
            )}
          </Field>
        )}
        <div className="sm:col-span-2">
          <div className="mb-2 text-sm font-medium text-ink">Allowed methods</div>
          <div className="flex flex-wrap gap-4">
            {METHODS.map((m) => (
              <Checkbox
                key={m}
                label={m}
                checked={form.allow_methods.includes(m)}
                onChange={(on) =>
                  setForm({ ...form, allow_methods: on ? [...form.allow_methods, m] : form.allow_methods.filter((x) => x !== m) })
                }
              />
            ))}
          </div>
        </div>
        <div className="space-y-4 sm:col-span-2">
          <Switch
            checked={form.allow_private}
            disabled={!status.data?.private_connections_allowed}
            onChange={(v) => setForm({ ...form, allow_private: v })}
            label="Allow private network addresses"
            description={
              status.data?.private_connections_allowed
                ? "Lets this connection reach internal hosts. Use only for services you control."
                : "Disabled by the server. Set VETO_CONNECTOR_ALLOW_PRIVATE=true to permit it."
            }
          />
          {!isNew && (
            <Switch
              checked={form.disabled}
              onChange={(v) => setForm({ ...form, disabled: v })}
              label="Disable"
              description="Calls through this connection fail until it is enabled again."
            />
          )}
        </div>
        {error && <p className="text-sm text-stop sm:col-span-2">{error}</p>}
      </div>
    </Dialog>
  );
}
