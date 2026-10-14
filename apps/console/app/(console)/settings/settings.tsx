"use client";

import { useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { KeyList } from "../agents/[id]/agent";
import { Icon } from "@/components/icons";
import { useCan, useSession } from "@/components/shell/session";
import { Button } from "@/components/ui/button";
import { Mono } from "@/components/ui/code";
import { ConfirmDialog, Dialog } from "@/components/ui/dialog";
import { Field, Input, SecretReveal } from "@/components/ui/form";
import { Details, PageHeader, Panel, PanelHeader } from "@/components/ui/layout";
import { Pill } from "@/components/ui/pills";
import { ErrorState, LoadingPanel } from "@/components/ui/states";
import { Tabs } from "@/components/ui/table";
import { useToast } from "@/components/ui/toast";
import { api, errorMessage } from "@/lib/api";
import { formatDate, formatDuration, formatMs, formatTimestamp } from "@/lib/format";
import { useKeys, useStatus, useWorkspace } from "@/lib/queries";
import type { IssuedKey, Settings, Workspace } from "@/lib/types";

type Tab = "workspace" | "profile" | "keys" | "system";

export function SettingsPage() {
  const [tab, setTab] = useState<Tab>("workspace");
  return (
    <>
      <PageHeader title="Settings" description="The workspace's rules, your account, API keys for automation, and the server's state." />
      <Tabs
        className="mb-6"
        value={tab}
        onChange={setTab}
        tabs={[
          { value: "workspace", label: "Workspace" },
          { value: "profile", label: "Your account" },
          { value: "keys", label: "Admin API keys" },
          { value: "system", label: "System" },
        ]}
      />
      {tab === "workspace" && <WorkspaceSettings />}
      {tab === "profile" && <Profile />}
      {tab === "keys" && <AdminKeys />}
      {tab === "system" && <System />}
    </>
  );
}

function NumberField({
  label,
  hint,
  value,
  onChange,
  disabled,
  suffix,
}: {
  label: string;
  hint: string;
  value: string;
  onChange: (v: string) => void;
  disabled: boolean;
  suffix?: string;
}) {
  return (
    <Field label={label} hint={hint}>
      {(id, d) => (
        <div className="relative">
          <Input
            id={id}
            aria-describedby={d}
            inputMode="decimal"
            disabled={disabled}
            value={value}
            onChange={(e) => onChange(e.target.value)}
            className={suffix ? "pr-20" : undefined}
          />
          {suffix && <span className="pointer-events-none absolute top-1/2 right-3 -translate-y-1/2 text-xs text-ink-3">{suffix}</span>}
        </div>
      )}
    </Field>
  );
}

type WorkspaceForm = Record<keyof Settings | "name", string>;

function toWorkspaceForm(w: Workspace): WorkspaceForm {
  const s = w.settings;
  return {
    name: w.name,
    review_timeout_seconds: String(s.review_timeout_seconds / 60),
    audit_sample_rate: String(Math.round(s.audit_sample_rate * 1000) / 10),
    retention_days: String(s.retention_days),
    untrusted_max_chars: String(s.untrusted_max_chars),
    calibration_target: String(Math.round(s.calibration_target * 1000) / 10),
  };
}

function WorkspaceSettings() {
  const q = useWorkspace();
  if (q.isError) return <ErrorState error={q.error} onRetry={() => void q.refetch()} />;
  if (!q.data) return <LoadingPanel rows={6} />;
  return <WorkspaceEditor workspace={q.data} />;
}

function WorkspaceEditor({ workspace }: { workspace: Workspace }) {
  const qc = useQueryClient();
  const toast = useToast();
  const canManage = useCan("manage_members");
  const [form, setForm] = useState<WorkspaceForm>(() => toWorkspaceForm(workspace));
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const set = (k: keyof WorkspaceForm) => (v: string) => setForm({ ...form, [k]: v });

  async function save() {
    setBusy(true);
    setError(null);
    try {
      await api.patch<Workspace>("/v1/workspace", {
        name: form.name,
        settings: {
          review_timeout_seconds: Math.round(Number(form.review_timeout_seconds) * 60),
          audit_sample_rate: Number(form.audit_sample_rate) / 100,
          retention_days: Math.round(Number(form.retention_days)),
          untrusted_max_chars: Math.round(Number(form.untrusted_max_chars)),
          calibration_target: Number(form.calibration_target) / 100,
        },
      });
      toast.success("Workspace saved");
      void qc.invalidateQueries({ queryKey: ["workspace"] });
      void qc.invalidateQueries({ queryKey: ["me"] });
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  }

  const disabled = !canManage;
  return (
    <Panel className="max-w-3xl">
      <div className="grid gap-5 sm:grid-cols-2">
        <Field label="Workspace name" className="sm:col-span-2">
          {(id) => <Input id={id} disabled={disabled} value={form.name} onChange={(e) => set("name")(e.target.value)} />}
        </Field>
        <NumberField
          label="Time to review"
          hint="A held action is stopped if nobody decides within this. 1 minute to 7 days."
          suffix="minutes"
          value={form.review_timeout_seconds}
          onChange={set("review_timeout_seconds")}
          disabled={disabled}
        />
        <NumberField
          label="Audit sample"
          hint="Share of decisions made without a person that are queued for an audit."
          suffix="percent"
          value={form.audit_sample_rate}
          onChange={set("audit_sample_rate")}
          disabled={disabled}
        />
        <NumberField
          label="Keep arguments for"
          hint="After this, arguments and request text are removed. Hashes in the ledger remain."
          suffix="days"
          value={form.retention_days}
          onChange={set("retention_days")}
          disabled={disabled}
        />
        <NumberField
          label="Untrusted content limit"
          hint="Characters of untrusted text shown to the decision model per action."
          suffix="characters"
          value={form.untrusted_max_chars}
          onChange={set("untrusted_max_chars")}
          disabled={disabled}
        />
        <NumberField
          label="Calibration target"
          hint="Agreement with reviewers that automatic decisions must reach. 50 to 100."
          suffix="percent"
          value={form.calibration_target}
          onChange={set("calibration_target")}
          disabled={disabled}
        />
      </div>
      {error && <p className="mt-5 rounded-[10px] bg-stop-wash px-3 py-2 text-sm text-stop">{error}</p>}
      {canManage && (
        <div className="mt-6 flex justify-end">
          <Button variant="primary" busy={busy} onClick={() => void save()}>
            Save changes
          </Button>
        </div>
      )}
    </Panel>
  );
}

function Profile() {
  const me = useSession();
  const qc = useQueryClient();
  const toast = useToast();
  const [name, setName] = useState(me.user.name);
  const [current, setCurrent] = useState("");
  const [next, setNext] = useState("");
  const [busy, setBusy] = useState<"name" | "password" | null>(null);
  const [error, setError] = useState<string | null>(null);

  async function saveName() {
    setBusy("name");
    try {
      await api.patch("/v1/auth/me", { name });
      toast.success("Name saved");
      void qc.invalidateQueries({ queryKey: ["me"] });
    } catch (err) {
      toast.error(errorMessage(err));
    } finally {
      setBusy(null);
    }
  }
  async function savePassword() {
    setBusy("password");
    setError(null);
    try {
      await api.post("/v1/auth/password", { current_password: current, new_password: next });
      setCurrent("");
      setNext("");
      toast.success("Password changed. Your other sessions were signed out.");
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(null);
    }
  }

  return (
    <div className="grid max-w-4xl gap-5 lg:grid-cols-2">
      <Panel>
        <PanelHeader title="Profile" />
        <div className="space-y-4">
          <Field label="Name">{(id) => <Input id={id} value={name} onChange={(e) => setName(e.target.value)} />}</Field>
          <Details
            rows={[
              ["Email", me.user.email],
              ["Joined", formatDate(me.user.created_at)],
            ]}
          />
          <div className="flex justify-end">
            <Button
              variant="primary"
              busy={busy === "name"}
              disabled={!name.trim() || name === me.user.name}
              onClick={() => void saveName()}
            >
              Save name
            </Button>
          </div>
        </div>
      </Panel>
      <Panel>
        <PanelHeader title="Password" />
        <div className="space-y-4">
          <Field label="Current password">
            {(id) => (
              <Input id={id} type="password" autoComplete="current-password" value={current} onChange={(e) => setCurrent(e.target.value)} />
            )}
          </Field>
          <Field label="New password" hint="At least 12 characters.">
            {(id, d) => (
              <Input
                id={id}
                aria-describedby={d}
                type="password"
                autoComplete="new-password"
                value={next}
                onChange={(e) => setNext(e.target.value)}
              />
            )}
          </Field>
          {error && <p className="text-sm text-stop">{error}</p>}
          <div className="flex justify-end">
            <Button
              variant="primary"
              busy={busy === "password"}
              disabled={!current || next.length < 12}
              onClick={() => void savePassword()}
            >
              Change password
            </Button>
          </div>
        </div>
      </Panel>
    </div>
  );
}

function AdminKeys() {
  const q = useKeys();
  const qc = useQueryClient();
  const canConfigure = useCan("own");
  const [open, setOpen] = useState(false);
  const [name, setName] = useState("");
  const [key, setKey] = useState<IssuedKey | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const adminKeys = q.data?.items.filter((k) => k.kind === "admin") ?? [];

  async function issue() {
    setBusy(true);
    setError(null);
    try {
      setKey(await api.post<IssuedKey>("/v1/keys", { name }));
      void qc.invalidateQueries({ queryKey: ["keys"] });
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <Panel className="max-w-3xl">
      <PanelHeader
        title="Admin API keys"
        description="For scripts and infrastructure-as-code that manage this workspace. An admin key has the admin role: it can configure and review, but not manage ownership, sign in or send agent actions. Only owners create them."
        actions={
          canConfigure && (
            <Button size="sm" icon={<Icon.Plus size={14} />} onClick={() => setOpen(true)}>
              New key
            </Button>
          )
        }
      />
      {q.isPending ? <LoadingPanel rows={3} /> : <KeyList keys={adminKeys} />}
      <Dialog
        open={open}
        onClose={() => {
          setOpen(false);
          setTimeout(() => (setKey(null), setName(""), setError(null)), 200);
        }}
        title={key ? "New admin key" : "Create an admin key"}
        footer={
          key ? (
            <Button variant="primary" onClick={() => setOpen(false)}>
              Done
            </Button>
          ) : (
            <Button variant="primary" busy={busy} onClick={() => void issue()}>
              Create key
            </Button>
          )
        }
      >
        {key ? (
          <SecretReveal label={key.name} secret={key.secret} />
        ) : (
          <div className="space-y-3">
            <Field label="Name" hint="What will use it, such as terraform.">
              {(id, d) => <Input id={id} aria-describedby={d} autoFocus value={name} onChange={(e) => setName(e.target.value)} />}
            </Field>
            {error && <p className="text-sm text-stop">{error}</p>}
          </div>
        )}
      </Dialog>
    </Panel>
  );
}

function System() {
  const q = useStatus();
  const qc = useQueryClient();
  const toast = useToast();
  const isOwner = useCan("own");
  const [rotating, setRotating] = useState(false);
  if (q.isError) return <ErrorState error={q.error} onRetry={() => void q.refetch()} />;
  if (!q.data) return <LoadingPanel rows={6} />;
  const s = q.data;
  return (
    <div className="grid max-w-5xl gap-5 lg:grid-cols-2">
      <Panel>
        <PanelHeader title="Server" />
        <Details
          rows={[
            ["Version", s.version],
            ["Database schema", s.schema_version],
            [
              "Decision model",
              s.model.configured ? (
                <span key="m">{s.model.name}</span>
              ) : (
                <Pill key="m" tone="safer">
                  Not configured
                </Pill>
              ),
            ],
            ["Decision time limit", formatMs(s.decision_timeout_ms)],
            [
              "Public address",
              s.public_url ? (
                <Mono key="u">{s.public_url}</Mono>
              ) : (
                <Pill key="u" tone="safer">
                  Not set
                </Pill>
              ),
            ],
            ["Slack", s.slack_available ? "Available" : "Unavailable"],
            ["Private network connections", s.private_connections_allowed ? "Allowed" : "Blocked"],
            ["Server clock", formatTimestamp(s.server_time)],
            ["Clock difference", formatDuration(Math.abs(q.dataUpdatedAt - new Date(s.server_time).getTime()) / 1000)],
          ]}
        />
      </Panel>
      <Panel>
        <PanelHeader
          title="Grant signing keys"
          description="Ed25519 keys that sign grants and ledger checkpoints. Tools can verify grants offline with the public keys at /v1/grants/keys."
          actions={
            isOwner && (
              <Button size="sm" onClick={() => setRotating(true)}>
                Rotate
              </Button>
            )
          }
        />
        <ul className="divide-y divide-line">
          {s.signing_keys.map((k) => (
            <li key={k.key_id} className="py-3">
              <div className="flex items-center justify-between">
                <Mono copy={k.key_id}>{k.key_id}</Mono>
                {k.retired_at ? (
                  <Pill tone="muted">Retired {formatDate(k.retired_at)}</Pill>
                ) : (
                  <Pill tone="proceed" dot>
                    Signing
                  </Pill>
                )}
              </div>
              <Mono copy={k.public_key} className="mt-1">
                {k.public_key}
              </Mono>
              <div className="mt-0.5 text-xs text-ink-3">Created {formatDate(k.created_at)}</div>
            </li>
          ))}
        </ul>
      </Panel>
      <ConfirmDialog
        open={rotating}
        onClose={() => setRotating(false)}
        tone="primary"
        title="Rotate the signing key?"
        body="New grants and checkpoints are signed with a new key. The old key is retired but stays published, so grants already issued keep verifying."
        confirmLabel="Rotate key"
        onConfirm={async () => {
          await api.post("/v1/signing-keys/rotate");
          toast.success("Signing key rotated");
          await qc.invalidateQueries({ queryKey: ["status"] });
        }}
      />
    </div>
  );
}
