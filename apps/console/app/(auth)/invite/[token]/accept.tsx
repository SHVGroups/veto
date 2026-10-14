"use client";

import { useQuery, useQueryClient } from "@tanstack/react-query";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState } from "react";
import { AuthForm } from "@/components/auth-form";
import { Field, Input } from "@/components/ui/form";
import { LoadingPanel } from "@/components/ui/states";
import { api, errorMessage } from "@/lib/api";
import { formatDateTime, roleDescription, roleLabel } from "@/lib/format";
import type { InvitationInfo, SessionView } from "@/lib/types";

export function AcceptInvitation({ token }: { token: string }) {
  const router = useRouter();
  const qc = useQueryClient();
  const info = useQuery({
    queryKey: ["invitation", token],
    queryFn: () => api.get<InvitationInfo>(`/v1/invitations/token/${encodeURIComponent(token)}`),
    retry: false,
  });
  const [name, setName] = useState("");
  const [password, setPassword] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  if (info.isPending) return <LoadingPanel rows={3} />;
  if (info.isError)
    return (
      <div>
        <h1 className="text-2xl text-ink">This invitation cannot be used</h1>
        <p className="mt-2 text-base text-ink-2">{errorMessage(info.error)}</p>
        <p className="mt-6 text-sm text-ink-3">
          Ask the person who invited you for a new link, or{" "}
          <Link href="/login" className="text-ink underline underline-offset-4">
            sign in
          </Link>{" "}
          if you already have an account.
        </p>
      </div>
    );

  const inv = info.data;

  async function submit() {
    setBusy(true);
    setError(null);
    try {
      await api.post<SessionView>("/v1/invitations/accept", { token, name, password });
      await qc.invalidateQueries();
      router.replace("/");
    } catch (err) {
      setError(errorMessage(err));
      setBusy(false);
    }
  }

  return (
    <AuthForm
      title={`Join ${inv.workspace_name}`}
      description={
        <>
          You were invited as {roleLabel[inv.role].toLowerCase()}: {roleDescription[inv.role].toLowerCase()} The link expires{" "}
          {formatDateTime(inv.expires_at)}.
        </>
      }
      submitLabel={inv.account_exists ? "Join workspace" : "Create account and join"}
      busy={busy}
      error={error}
      onSubmit={submit}
    >
      <Field label="Email">{(id) => <Input id={id} value={inv.email} disabled readOnly />}</Field>
      {!inv.account_exists && (
        <Field label="Your name">
          {(id) => <Input id={id} autoComplete="name" autoFocus value={name} onChange={(e) => setName(e.target.value)} />}
        </Field>
      )}
      <Field label="Password" hint={inv.account_exists ? "The password of your existing Veto account." : "At least 12 characters."}>
        {(id, d) => (
          <Input
            id={id}
            aria-describedby={d}
            type="password"
            autoComplete={inv.account_exists ? "current-password" : "new-password"}
            value={password}
            onChange={(e) => setPassword(e.target.value)}
          />
        )}
      </Field>
    </AuthForm>
  );
}
