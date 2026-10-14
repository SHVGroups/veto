"use client";

import { useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { Icon } from "@/components/icons";
import { useCan, useSession } from "@/components/shell/session";
import { Button } from "@/components/ui/button";
import { CopyButton } from "@/components/ui/code";
import { ConfirmDialog, Dialog } from "@/components/ui/dialog";
import { Field, Input, Select } from "@/components/ui/form";
import { PageHeader, Panel, PanelHeader } from "@/components/ui/layout";
import { Pill } from "@/components/ui/pills";
import { ErrorState, LoadingPanel } from "@/components/ui/states";
import { Table, Td, Th } from "@/components/ui/table";
import { useToast } from "@/components/ui/toast";
import { api, errorMessage } from "@/lib/api";
import { formatDate, formatRelative, roleDescription, roleLabel } from "@/lib/format";
import { useInvitations, useMembers } from "@/lib/queries";
import type { Invitation, Member, Role } from "@/lib/types";

const ROLES: Role[] = ["owner", "admin", "reviewer", "viewer"];

export function MembersPage() {
  const members = useMembers();
  const invitations = useInvitations();
  const me = useSession();
  const canManage = useCan("manage_members");
  const isOwner = useCan("own");
  const qc = useQueryClient();
  const toast = useToast();
  const [inviting, setInviting] = useState(false);
  const [removing, setRemoving] = useState<Member | null>(null);
  const [revoking, setRevoking] = useState<Invitation | null>(null);
  const pending = invitations.data?.items.filter((i) => !i.accepted_at && !i.revoked_at && new Date(i.expires_at) > new Date()) ?? [];

  async function setRole(m: Member, role: Role) {
    try {
      await api.patch(`/v1/members/${m.id}`, { role });
      toast.success(`${m.name} is now ${roleLabel[role].toLowerCase()}`);
      void qc.invalidateQueries({ queryKey: ["members"] });
    } catch (err) {
      toast.error(errorMessage(err));
    }
  }

  return (
    <>
      <PageHeader
        title="Members"
        description="People who can sign in to this workspace, and what each one may do."
        actions={
          canManage && (
            <Button variant="primary" icon={<Icon.Plus size={15} />} onClick={() => setInviting(true)}>
              Invite someone
            </Button>
          )
        }
      />
      <div className="grid items-start gap-5 xl:grid-cols-[minmax(0,1fr)_340px]">
        <div className="space-y-5">
          <Panel>
            {members.isError && <ErrorState error={members.error} onRetry={() => void members.refetch()} />}
            {members.isPending ? (
              <LoadingPanel />
            ) : (
              <Table>
                <thead>
                  <tr>
                    <Th>Member</Th>
                    <Th>Role</Th>
                    <Th>Joined</Th>
                    <Th>Last sign-in</Th>
                    <Th />
                  </tr>
                </thead>
                <tbody>
                  {members.data?.items.map((m) => {
                    const self = m.id === me.user.id;
                    const editable = canManage && !self && (isOwner || m.role !== "owner");
                    return (
                      <tr key={m.id}>
                        <Td>
                          <div className="flex items-center gap-2">
                            <span className="font-medium text-ink">{m.name}</span>
                            {self && <Pill tone="muted">You</Pill>}
                            {m.disabled_at && <Pill tone="stop">Disabled</Pill>}
                          </div>
                          <div className="text-xs text-ink-3">{m.email}</div>
                        </Td>
                        <Td>
                          {editable ? (
                            <Select
                              aria-label={`Role of ${m.name}`}
                              value={m.role}
                              onChange={(e) => void setRole(m, e.target.value as Role)}
                              className="w-36"
                            >
                              {ROLES.filter((r) => isOwner || r !== "owner").map((r) => (
                                <option key={r} value={r}>
                                  {roleLabel[r]}
                                </option>
                              ))}
                            </Select>
                          ) : (
                            <span className="text-ink-2">{roleLabel[m.role]}</span>
                          )}
                        </Td>
                        <Td className="text-ink-2">{formatDate(m.joined_at)}</Td>
                        <Td className="text-ink-2">{m.last_login_at ? formatRelative(m.last_login_at) : "Never"}</Td>
                        <Td className="text-right">
                          {editable && (
                            <Button size="sm" variant="ghost" onClick={() => setRemoving(m)}>
                              Remove
                            </Button>
                          )}
                        </Td>
                      </tr>
                    );
                  })}
                </tbody>
              </Table>
            )}
          </Panel>
          {canManage && (
            <Panel>
              <PanelHeader title="Pending invitations" description="Links work once and expire after seven days." />
              {pending.length === 0 ? (
                <p className="text-sm text-ink-3">None.</p>
              ) : (
                <ul className="divide-y divide-line">
                  {pending.map((i) => (
                    <li key={i.id} className="flex items-center justify-between gap-3 py-3">
                      <div>
                        <div className="text-sm text-ink">{i.email}</div>
                        <div className="text-xs text-ink-3">
                          {roleLabel[i.role]}, expires {formatRelative(i.expires_at)}
                        </div>
                      </div>
                      <Button size="sm" variant="ghost" onClick={() => setRevoking(i)}>
                        Revoke
                      </Button>
                    </li>
                  ))}
                </ul>
              )}
            </Panel>
          )}
        </div>
        <Panel>
          <PanelHeader title="Roles" />
          <dl className="space-y-3">
            {ROLES.map((r) => (
              <div key={r}>
                <dt className="text-sm font-medium text-ink">{roleLabel[r]}</dt>
                <dd className="text-sm text-ink-2">{roleDescription[r]}</dd>
              </div>
            ))}
          </dl>
        </Panel>
      </div>

      <InviteDialog open={inviting} onClose={() => setInviting(false)} canInviteOwner={isOwner} />
      <ConfirmDialog
        open={!!removing}
        onClose={() => setRemoving(null)}
        title={`Remove ${removing?.name}?`}
        body="They are signed out and can no longer open this workspace. Their past decisions stay in the ledger under their name."
        confirmLabel="Remove member"
        onConfirm={async () => {
          await api.del(`/v1/members/${removing!.id}`);
          toast.success("Member removed");
          void qc.invalidateQueries({ queryKey: ["members"] });
        }}
      />
      <ConfirmDialog
        open={!!revoking}
        onClose={() => setRevoking(null)}
        title="Revoke this invitation?"
        body={`The link sent to ${revoking?.email} stops working.`}
        confirmLabel="Revoke"
        onConfirm={async () => {
          await api.del(`/v1/invitations/${revoking!.id}`);
          toast.success("Invitation revoked");
          void qc.invalidateQueries({ queryKey: ["invitations"] });
        }}
      />
    </>
  );
}

function InviteDialog({ open, onClose, canInviteOwner }: { open: boolean; onClose: () => void; canInviteOwner: boolean }) {
  const qc = useQueryClient();
  const [email, setEmail] = useState("");
  const [role, setRole] = useState<Role>("reviewer");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [link, setLink] = useState<string | null>(null);

  function close() {
    onClose();
    setTimeout(() => {
      setEmail("");
      setRole("reviewer");
      setLink(null);
      setError(null);
    }, 200);
  }

  async function invite() {
    setBusy(true);
    setError(null);
    try {
      const r = await api.post<{ invitation: Invitation; token: string; link: string }>("/v1/invitations", { email, role });
      setLink(r.link || `${window.location.origin}/invite/${r.token}`);
      void qc.invalidateQueries({ queryKey: ["invitations"] });
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
      title={link ? "Invitation ready" : "Invite someone"}
      description={link ? `Send this link to ${email}. It works once.` : "Veto does not send email; you share the link yourself."}
      footer={
        link ? (
          <Button variant="primary" onClick={close}>
            Done
          </Button>
        ) : (
          <>
            <Button variant="ghost" onClick={close}>
              Cancel
            </Button>
            <Button variant="primary" busy={busy} disabled={!email.includes("@")} onClick={() => void invite()}>
              Create invitation
            </Button>
          </>
        )
      }
    >
      {link ? (
        <div className="flex items-center gap-2">
          <code className="min-w-0 flex-1 truncate rounded-[10px] bg-sunken px-3 py-2 text-xs text-ink shadow-[inset_0_0_0_1px_var(--line)]">
            {link}
          </code>
          <CopyButton value={link} label="Copy link" className="size-9" />
        </div>
      ) : (
        <div className="space-y-4">
          <Field label="Email">
            {(id) => <Input id={id} type="email" autoFocus value={email} onChange={(e) => setEmail(e.target.value.trim())} />}
          </Field>
          <Field label="Role" hint={roleDescription[role]}>
            {(id, d) => (
              <Select id={id} aria-describedby={d} value={role} onChange={(e) => setRole(e.target.value as Role)}>
                {ROLES.filter((r) => canInviteOwner || r !== "owner").map((r) => (
                  <option key={r} value={r}>
                    {roleLabel[r]}
                  </option>
                ))}
              </Select>
            )}
          </Field>
          {error && <p className="text-sm text-stop">{error}</p>}
        </div>
      )}
    </Dialog>
  );
}
