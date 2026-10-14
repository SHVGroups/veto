"use client";

import { useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { useCan } from "@/components/shell/session";
import { Button } from "@/components/ui/button";
import { CodeBlock, Mono } from "@/components/ui/code";
import { ConfirmDialog } from "@/components/ui/dialog";
import { Field, Input, Switch } from "@/components/ui/form";
import { PageHeader, Panel, PanelHeader } from "@/components/ui/layout";
import { Pill } from "@/components/ui/pills";
import { ErrorState, LoadingPanel, Notice } from "@/components/ui/states";
import { useToast } from "@/components/ui/toast";
import { api, errorMessage } from "@/lib/api";
import { useSlack } from "@/lib/queries";
import type { SlackStatus } from "@/lib/types";

export function SlackPage() {
  const q = useSlack();
  if (q.isError) return <ErrorState error={q.error} onRetry={() => void q.refetch()} />;
  if (!q.data) return <LoadingPanel rows={8} />;
  return <SlackEditor s={q.data} />;
}

function SlackEditor({ s }: { s: SlackStatus }) {
  const qc = useQueryClient();
  const toast = useToast();
  const canConfigure = useCan("configure");
  const [channel, setChannel] = useState(s.settings.channel);
  const [mention, setMention] = useState(s.settings.mention_held);
  const [botToken, setBotToken] = useState("");
  const [signingSecret, setSigningSecret] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [removing, setRemoving] = useState(false);

  const changingSecrets = botToken !== "" || signingSecret !== "";

  async function save() {
    setBusy(true);
    setError(null);
    try {
      await api.put<SlackStatus>("/v1/integrations/slack", {
        settings: { channel, mention_held: mention },
        secrets: changingSecrets ? { bot_token: botToken, signing_secret: signingSecret } : undefined,
      });
      setBotToken("");
      setSigningSecret("");
      toast.success("Slack saved");
      void qc.invalidateQueries({ queryKey: ["slack"] });
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  }

  const manifest = JSON.stringify(
    {
      display_information: { name: "Veto", description: "Approve or stop actions your AI agents want to take." },
      features: { bot_user: { display_name: "Veto", always_online: true } },
      oauth_config: { scopes: { bot: ["chat:write", "users:read", "users:read.email"] } },
      settings: {
        interactivity: { is_enabled: true, request_url: s.interaction_url },
        org_deploy_enabled: false,
        socket_mode_enabled: false,
      },
    },
    null,
    2,
  );

  return (
    <>
      <PageHeader
        title="Slack"
        description="Post held actions to a channel with buttons for the three options. A click counts only when the Slack user's email belongs to a Veto member who can review."
        actions={
          s.configured ? (
            <Pill tone="proceed" dot>
              Connected
            </Pill>
          ) : (
            <Pill>Not connected</Pill>
          )
        }
      />
      <div className="grid items-start gap-5 xl:grid-cols-[minmax(0,1fr)_460px]">
        <Panel>
          <PanelHeader title="Settings" />
          <form
            className="space-y-5"
            onSubmit={(e) => {
              e.preventDefault();
              void save();
            }}
          >
            <Field
              label="Channel id"
              hint="In Slack, open the channel's details; the id is at the bottom, such as C0123456789. Invite the Veto app to the channel."
            >
              {(id, d) => (
                <Input
                  id={id}
                  aria-describedby={d}
                  disabled={!canConfigure}
                  value={channel}
                  onChange={(e) => setChannel(e.target.value.trim())}
                />
              )}
            </Field>
            <Switch
              checked={mention}
              disabled={!canConfigure}
              onChange={setMention}
              label="Notify the channel"
              description="Adds @here to each held action, so people see it promptly."
            />
            <div className="border-t border-line pt-5">
              <div className="mb-1 text-sm font-medium text-ink">App credentials</div>
              <p className="mb-4 text-xs text-ink-3">
                {s.has_secrets
                  ? "Stored encrypted. Fill both fields only to replace them."
                  : "From your Slack app's settings: OAuth and Permissions, and Basic Information."}
              </p>
              <div className="grid gap-4 sm:grid-cols-2">
                <Field label="Bot token">
                  {(id) => (
                    <Input
                      id={id}
                      type="password"
                      autoComplete="off"
                      placeholder={s.has_secrets ? "Unchanged" : "xoxb-…"}
                      disabled={!canConfigure}
                      value={botToken}
                      onChange={(e) => setBotToken(e.target.value.trim())}
                    />
                  )}
                </Field>
                <Field label="Signing secret">
                  {(id) => (
                    <Input
                      id={id}
                      type="password"
                      autoComplete="off"
                      placeholder={s.has_secrets ? "Unchanged" : ""}
                      disabled={!canConfigure}
                      value={signingSecret}
                      onChange={(e) => setSigningSecret(e.target.value.trim())}
                    />
                  )}
                </Field>
              </div>
            </div>
            {error && <p className="rounded-[10px] bg-stop-wash px-3 py-2 text-sm text-stop">{error}</p>}
            {canConfigure && (
              <div className="flex justify-between gap-3">
                {s.configured ? (
                  <Button variant="ghost" onClick={() => setRemoving(true)}>
                    Disconnect
                  </Button>
                ) : (
                  <span />
                )}
                <Button type="submit" variant="primary" busy={busy}>
                  {s.configured ? "Save changes" : "Connect Slack"}
                </Button>
              </div>
            )}
          </form>
        </Panel>
        <div className="space-y-5">
          <Panel>
            <PanelHeader
              title="Create the Slack app"
              description="At api.slack.com/apps, choose Create New App, then From a manifest, and paste this."
            />
            <CodeBlock code={manifest} />
            <p className="mt-3 text-sm text-ink-2">
              Then install it to your workspace and copy the bot token and signing secret into the form. The interaction address is{" "}
              <Mono copy={s.interaction_url}>{s.interaction_url}</Mono>
            </p>
          </Panel>
          {!s.interaction_url.startsWith("https://") && (
            <Notice tone="safer" title="Slack needs a public HTTPS address">
              Set VETO_PUBLIC_URL to the HTTPS address where this console is reachable from the internet, so Slack can send button clicks.
            </Notice>
          )}
        </div>
      </div>
      <ConfirmDialog
        open={removing}
        onClose={() => setRemoving(false)}
        title="Disconnect Slack?"
        body="Held actions stop appearing in Slack and its credentials are deleted. Buttons on old messages stop working."
        confirmLabel="Disconnect"
        onConfirm={async () => {
          await api.del("/v1/integrations/slack");
          toast.success("Slack disconnected");
          void qc.invalidateQueries({ queryKey: ["slack"] });
        }}
      />
    </>
  );
}
