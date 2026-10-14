"use client";

import { useQueryClient } from "@tanstack/react-query";
import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import { AuthForm } from "@/components/auth-form";
import { Field, Input } from "@/components/ui/form";
import { api, errorMessage } from "@/lib/api";
import { useSetupState } from "@/lib/queries";
import type { SessionView } from "@/lib/types";

const MIN_PASSWORD = 12;

export function SetupForm() {
  const router = useRouter();
  const qc = useQueryClient();
  const setup = useSetupState();
  const [form, setForm] = useState({ workspace_name: "", name: "", email: "", password: "", setup_token: "" });
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (setup.data && !setup.data.needs_setup) router.replace("/login");
  }, [setup.data, router]);

  const set = (k: keyof typeof form) => (e: React.ChangeEvent<HTMLInputElement>) => setForm((f) => ({ ...f, [k]: e.target.value }));

  async function submit() {
    if (!form.workspace_name || !form.name || !form.email) {
      setError("Fill in every field.");
      return;
    }
    if (form.password.length < MIN_PASSWORD) {
      setError(`Use a password of at least ${MIN_PASSWORD} characters.`);
      return;
    }
    setBusy(true);
    setError(null);
    try {
      await api.post<SessionView>("/v1/setup", {
        ...form,
        setup_token: setup.data?.setup_token_required ? form.setup_token : undefined,
      });
      await qc.invalidateQueries();
      router.replace("/");
    } catch (err) {
      setError(errorMessage(err));
      setBusy(false);
    }
  }

  return (
    <AuthForm
      title="Set up Veto"
      description="Create the workspace and its first owner. You can invite your team afterwards."
      submitLabel="Create workspace"
      busy={busy}
      error={error}
      onSubmit={submit}
    >
      <Field label="Workspace name" hint="Usually your company or team.">
        {(id, d) => <Input id={id} aria-describedby={d} autoFocus value={form.workspace_name} onChange={set("workspace_name")} />}
      </Field>
      <Field label="Your name">{(id) => <Input id={id} autoComplete="name" value={form.name} onChange={set("name")} />}</Field>
      <Field label="Email">
        {(id) => <Input id={id} type="email" autoComplete="username" value={form.email} onChange={set("email")} />}
      </Field>
      <Field label="Password" hint={`At least ${MIN_PASSWORD} characters.`}>
        {(id, d) => (
          <Input
            id={id}
            aria-describedby={d}
            type="password"
            autoComplete="new-password"
            value={form.password}
            onChange={set("password")}
          />
        )}
      </Field>
      {setup.data?.setup_token_required && (
        <Field label="Setup token" hint="The value of VETO_SETUP_TOKEN on the server. It proves you operate this deployment.">
          {(id, d) => <Input id={id} aria-describedby={d} autoComplete="off" value={form.setup_token} onChange={set("setup_token")} />}
        </Field>
      )}
    </AuthForm>
  );
}
