"use client";

import { useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { Icon } from "@/components/icons";
import { Unplugged } from "@/components/illustrations";
import { useCan } from "@/components/shell/session";
import { Button } from "@/components/ui/button";
import { CodeBlock, JsonView } from "@/components/ui/code";
import { ConfirmDialog, Dialog } from "@/components/ui/dialog";
import { Checkbox, Field, Input, SecretReveal, Switch } from "@/components/ui/form";
import { PageHeader, Panel, PanelHeader } from "@/components/ui/layout";
import { Pill } from "@/components/ui/pills";
import { EmptyState, ErrorState, LoadingPanel } from "@/components/ui/states";
import { Menu, MenuItem, Table, Td, Th } from "@/components/ui/table";
import { useToast } from "@/components/ui/toast";
import { api, errorMessage } from "@/lib/api";
import { cx } from "@/lib/cx";
import { formatDateTime, formatRelative } from "@/lib/format";
import { useDeliveries, useMeta, useWebhooks } from "@/lib/queries";
import type { CreatedWebhook, Delivery, Webhook, WebhookInput } from "@/lib/types";

const eventHelp: Record<string, string> = {
  "action.pending_review": "An action is waiting for a person",
  "action.decided": "An action was approved or stopped",
  "action.expired": "A held action expired and was stopped",
  "outcome.reported": "An agent reported how an action went",
  "audit.sampled": "A decision was queued for audit",
  "tool.discovered": "An agent called a tool Veto had not seen",
  "ledger.verification_failed": "The ledger failed verification",
};

export function WebhooksPage() {
  const q = useWebhooks();
  const canConfigure = useCan("configure");
  const qc = useQueryClient();
  const toast = useToast();
  const [editing, setEditing] = useState<Webhook | "new" | null>(null);
  const [deleting, setDeleting] = useState<Webhook | null>(null);
  const [secret, setSecret] = useState<string | null>(null);
  const [open, setOpen] = useState<string | null>(null);

  async function test(w: Webhook) {
    try {
      await api.post(`/v1/webhooks/${w.id}/test`);
      toast.success("Test event queued. It appears under deliveries in a few seconds.");
      setOpen(w.id);
    } catch (err) {
      toast.error(errorMessage(err));
    }
  }
  async function rotate(w: Webhook) {
    try {
      const r = await api.post<{ secret: string }>(`/v1/webhooks/${w.id}/rotate`);
      setSecret(r.secret);
    } catch (err) {
      toast.error(errorMessage(err));
    }
  }

  return (
    <>
      <PageHeader
        title="Webhooks"
        description="Veto posts signed JSON to your endpoints when things happen. Failed deliveries are retried eight times over about eleven hours."
        actions={
          canConfigure && (
            <Button variant="primary" icon={<Icon.Plus size={15} />} onClick={() => setEditing("new")}>
              Add endpoint
            </Button>
          )
        }
      />
      <div className="grid items-start gap-5 xl:grid-cols-[minmax(0,1fr)_420px]">
        <div className="space-y-4">
          {q.isError && <ErrorState error={q.error} onRetry={() => void q.refetch()} />}
          {q.isPending ? (
            <Panel>
              <LoadingPanel />
            </Panel>
          ) : !q.data?.items.length ? (
            <Panel>
              <EmptyState art={<Unplugged />} title="No endpoints">
                Add an endpoint to send held actions to an incident tool or record decisions in your own systems.
              </EmptyState>
            </Panel>
          ) : (
            q.data.items.map((w) => (
              <Panel key={w.id}>
                <div className="flex items-start justify-between gap-4">
                  <div className="min-w-0">
                    <div className="flex items-center gap-2">
                      <span className="truncate font-medium text-ink">{w.url}</span>
                      {w.disabled_at && <Pill tone="stop">Disabled</Pill>}
                    </div>
                    {w.description && <p className="text-sm text-ink-3">{w.description}</p>}
                    <div className="mt-2 flex flex-wrap gap-1.5">
                      {w.events.map((e) => (
                        <Pill key={e}>{e}</Pill>
                      ))}
                    </div>
                  </div>
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
                          <MenuItem icon={<Icon.Bolt size={15} />} onSelect={() => (close(), void test(w))}>
                            Send a test event
                          </MenuItem>
                          <MenuItem icon={<Icon.Edit size={15} />} onSelect={() => (close(), setEditing(w))}>
                            Edit
                          </MenuItem>
                          <MenuItem icon={<Icon.Key size={15} />} onSelect={() => (close(), void rotate(w))}>
                            Rotate signing secret
                          </MenuItem>
                          <MenuItem tone="danger" icon={<Icon.Trash size={15} />} onSelect={() => (close(), setDeleting(w))}>
                            Delete
                          </MenuItem>
                        </>
                      )}
                    </Menu>
                  )}
                </div>
                <button
                  type="button"
                  onClick={() => setOpen(open === w.id ? null : w.id)}
                  className="mt-4 flex items-center gap-1 text-sm text-ink-2 hover:text-ink"
                  aria-expanded={open === w.id}
                >
                  <Icon.ChevronRight size={14} className={cx("transition-transform", open === w.id && "rotate-90")} />
                  Recent deliveries
                </button>
                {open === w.id && <Deliveries webhookId={w.id} />}
              </Panel>
            ))
          )}
        </div>
        <Panel>
          <PanelHeader
            title="Verify the signature"
            description="Each request has a Veto-Signature header: t=<unix time>,v1=<hex HMAC-SHA256 of t.body>, keyed with the endpoint's secret."
          />
          <CodeBlock
            code={`import crypto from "node:crypto";

export function verify(secret, header, rawBody) {
  const parts = Object.fromEntries(
    header.split(",").map((p) => p.trim().split("=")),
  );
  const age = Date.now() / 1000 - Number(parts.t);
  if (!(age < 300)) return false;
  const expected = crypto
    .createHmac("sha256", secret)
    .update(\`\${parts.t}.\${rawBody}\`)
    .digest("hex");
  return crypto.timingSafeEqual(
    Buffer.from(expected), Buffer.from(parts.v1 ?? ""),
  );
}`}
          />
          <p className="mt-3 text-xs text-ink-3">
            Also sent: Veto-Event with the event name and Veto-Delivery with an id that stays the same across retries.
          </p>
        </Panel>
      </div>

      <WebhookDialog webhook={editing} onClose={() => setEditing(null)} onCreated={(s) => setSecret(s)} />
      <Dialog
        open={!!secret}
        onClose={() => setSecret(null)}
        title="Signing secret"
        footer={
          <Button variant="primary" onClick={() => setSecret(null)}>
            Done
          </Button>
        }
      >
        {secret && <SecretReveal label="Use this to verify deliveries" secret={secret} />}
      </Dialog>
      <ConfirmDialog
        open={!!deleting}
        onClose={() => setDeleting(null)}
        title="Delete this endpoint?"
        body={`Veto stops sending events to ${deleting?.url}. Queued deliveries are dropped.`}
        confirmLabel="Delete endpoint"
        onConfirm={async () => {
          await api.del(`/v1/webhooks/${deleting!.id}`);
          toast.success("Endpoint deleted");
          void qc.invalidateQueries({ queryKey: ["webhooks"] });
        }}
      />
    </>
  );
}

function Deliveries({ webhookId }: { webhookId: string }) {
  const q = useDeliveries(webhookId);
  const [shown, setShown] = useState<Delivery | null>(null);
  if (q.isPending) return <LoadingPanel rows={3} className="mt-4" />;
  if (q.isError) return <ErrorState error={q.error} className="mt-4" />;
  if (!q.data.items.length) return <p className="mt-3 text-sm text-ink-3">Nothing sent yet.</p>;
  return (
    <>
      <Table className="mt-3">
        <thead>
          <tr>
            <Th>Event</Th>
            <Th>Status</Th>
            <Th className="text-right">Attempts</Th>
            <Th>Response</Th>
            <Th className="text-right">Created</Th>
          </tr>
        </thead>
        <tbody>
          {q.data.items.map((d) => (
            <tr key={d.id} className="cursor-pointer hover:bg-sunken" onClick={() => setShown(d)}>
              <Td className="font-medium">{d.event}</Td>
              <Td>
                {d.status === "delivered" ? (
                  <Pill tone="proceed" dot>
                    Delivered
                  </Pill>
                ) : d.status === "failed" ? (
                  <Pill tone="stop" dot>
                    Failed
                  </Pill>
                ) : (
                  <Pill tone="held" dot>
                    Retrying {formatRelative(d.next_attempt_at)}
                  </Pill>
                )}
              </Td>
              <Td className="figures text-right">{d.attempts}</Td>
              <Td className="max-w-60 truncate text-xs text-ink-2">
                {d.response_status ? `HTTP ${d.response_status}` : d.last_error || "–"}
              </Td>
              <Td className="text-right whitespace-nowrap text-ink-3" title={formatDateTime(d.created_at)}>
                {formatRelative(d.created_at)}
              </Td>
            </tr>
          ))}
        </tbody>
      </Table>
      <Dialog
        open={!!shown}
        onClose={() => setShown(null)}
        title={shown?.event ?? ""}
        description={shown ? `Delivery ${shown.id}` : undefined}
        width="lg"
      >
        {shown && (
          <div className="space-y-3">
            {shown.last_error && <p className="rounded-[10px] bg-stop-wash px-3 py-2 text-sm text-stop">{shown.last_error}</p>}
            <JsonView value={shown.payload} maxHeight={420} label="payload" />
          </div>
        )}
      </Dialog>
    </>
  );
}

function WebhookDialog({
  webhook,
  onClose,
  onCreated,
}: {
  webhook: Webhook | "new" | null;
  onClose: () => void;
  onCreated: (secret: string) => void;
}) {
  if (!webhook) return null;
  return (
    <WebhookForm
      key={webhook === "new" ? "new" : webhook.id}
      existing={webhook === "new" ? null : webhook}
      onClose={onClose}
      onCreated={onCreated}
    />
  );
}

function WebhookForm({
  existing,
  onClose,
  onCreated,
}: {
  existing: Webhook | null;
  onClose: () => void;
  onCreated: (secret: string) => void;
}) {
  const qc = useQueryClient();
  const toast = useToast();
  const meta = useMeta();
  const isNew = existing === null;
  const allEvents = meta.data?.webhook_events ?? Object.keys(eventHelp);
  const [form, setForm] = useState<WebhookInput>(() =>
    existing
      ? { url: existing.url, description: existing.description, events: existing.events, disabled: !!existing.disabled_at }
      : { url: "https://", description: "", events: ["action.pending_review", "action.decided"], disabled: false },
  );
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function save() {
    setBusy(true);
    setError(null);
    try {
      if (isNew) {
        const w = await api.post<CreatedWebhook>("/v1/webhooks", form);
        onCreated(w.secret);
      } else {
        await api.patch(`/v1/webhooks/${existing!.id}`, form);
        toast.success("Endpoint saved");
      }
      void qc.invalidateQueries({ queryKey: ["webhooks"] });
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
      title={isNew ? "Add an endpoint" : "Edit endpoint"}
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            Cancel
          </Button>
          <Button variant="primary" busy={busy} disabled={!form.events.length} onClick={() => void save()}>
            {isNew ? "Add endpoint" : "Save changes"}
          </Button>
        </>
      }
    >
      <div className="space-y-4">
        <Field label="URL" hint="HTTPS, reachable from the Veto server.">
          {(id, d) => (
            <Input id={id} aria-describedby={d} value={form.url} onChange={(e) => setForm({ ...form, url: e.target.value.trim() })} />
          )}
        </Field>
        <Field label="Description" optional>
          {(id) => <Input id={id} value={form.description} onChange={(e) => setForm({ ...form, description: e.target.value })} />}
        </Field>
        <div>
          <div className="mb-2 text-sm font-medium text-ink">Events</div>
          <div className="space-y-2.5">
            {allEvents.map((e) => (
              <div key={e}>
                <Checkbox
                  checked={form.events.includes(e)}
                  onChange={(on) => setForm({ ...form, events: on ? [...form.events, e] : form.events.filter((x) => x !== e) })}
                  label={<code className="text-ink">{e}</code>}
                />
                {eventHelp[e] && <p className="ml-6.5 text-xs text-ink-3">{eventHelp[e]}</p>}
              </div>
            ))}
          </div>
        </div>
        {!isNew && (
          <Switch
            checked={form.disabled}
            onChange={(v) => setForm({ ...form, disabled: v })}
            label="Pause deliveries"
            description="Events are not sent while paused."
          />
        )}
        {error && <p className="text-sm text-stop">{error}</p>}
      </div>
    </Dialog>
  );
}
