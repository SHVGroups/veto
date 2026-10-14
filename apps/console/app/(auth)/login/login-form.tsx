"use client";

import { useQueryClient } from "@tanstack/react-query";
import type { Route } from "next";
import { useRouter, useSearchParams } from "next/navigation";
import { useEffect, useState } from "react";
import { AuthForm } from "@/components/auth-form";
import { Field, Input } from "@/components/ui/form";
import { api, errorMessage } from "@/lib/api";
import { useSetupState } from "@/lib/queries";
import type { SessionView } from "@/lib/types";

/** Only same-site paths are honoured as a destination after sign-in. */
function safeNext(next: string | null): Route {
  return (next && next.startsWith("/") && !next.startsWith("//") ? next : "/") as Route;
}

export function LoginForm() {
  const router = useRouter();
  const params = useSearchParams();
  const qc = useQueryClient();
  const setup = useSetupState();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (setup.data?.needs_setup) router.replace("/setup");
  }, [setup.data, router]);

  async function submit() {
    if (!email || !password) {
      setError("Enter your email and password.");
      return;
    }
    setBusy(true);
    setError(null);
    try {
      await api.post<SessionView>("/v1/auth/login", { email, password });
      await qc.invalidateQueries();
      router.replace(safeNext(params.get("next")));
    } catch (err) {
      setError(errorMessage(err));
      setBusy(false);
    }
  }

  return (
    <AuthForm
      title="Sign in"
      description="Review held actions and see what your agents are doing."
      submitLabel="Sign in"
      busy={busy}
      error={error}
      onSubmit={submit}
      footer="No account? Ask a workspace owner or admin for an invitation."
    >
      <Field label="Email">
        {(id) => <Input id={id} type="email" autoComplete="username" autoFocus value={email} onChange={(e) => setEmail(e.target.value)} />}
      </Field>
      <Field label="Password">
        {(id) => (
          <Input id={id} type="password" autoComplete="current-password" value={password} onChange={(e) => setPassword(e.target.value)} />
        )}
      </Field>
    </AuthForm>
  );
}
