"use client";

import { useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef, useState } from "react";
import { CodeEditor, type CodeEditorHandle } from "@/components/editor/code-editor";
import { Icon } from "@/components/icons";
import { useCan } from "@/components/shell/session";
import { Button } from "@/components/ui/button";
import { ConfirmDialog, Dialog } from "@/components/ui/dialog";
import { Checkbox, Field, Input } from "@/components/ui/form";
import { PageHeader, Panel, PanelHeader } from "@/components/ui/layout";
import { Pill } from "@/components/ui/pills";
import { ErrorState, LoadingPanel } from "@/components/ui/states";
import { useToast } from "@/components/ui/toast";
import { api, errorMessage } from "@/lib/api";
import { cx } from "@/lib/cx";
import { formatDateTime, formatRelative } from "@/lib/format";
import { usePolicies } from "@/lib/queries";
import type { PolicyCheck, PolicyVersion } from "@/lib/types";

const effectTone = { deny: "stop", allow: "proceed", review: "held" } as const;

export function PoliciesPage() {
  const q = usePolicies();
  const qc = useQueryClient();
  const toast = useToast();
  const canConfigure = useCan("configure");
  const versions = q.data?.items ?? [];
  const active = versions.find((v) => v.active);
  const [selected, setSelected] = useState<number | null>(null);
  const [draft, setDraft] = useState<string | null>(null);
  const [check, setCheck] = useState<PolicyCheck | null>(null);
  const [checking, setChecking] = useState(false);
  const [saving, setSaving] = useState(false);
  const [activating, setActivating] = useState<PolicyVersion | null>(null);
  const editor = useRef<CodeEditorHandle>(null);
  const shown = versions.find((v) => v.version === selected) ?? active;
  const source = draft ?? shown?.source ?? "";
  const dirty = draft !== null && draft !== shown?.source;

  // Check as you type, after a pause.
  useEffect(() => {
    if (!source) return;
    const t = window.setTimeout(async () => {
      setChecking(true);
      try {
        setCheck(await api.post<PolicyCheck>("/v1/policies/check", { source }));
      } catch {
        setCheck(null);
      } finally {
        setChecking(false);
      }
    }, 450);
    return () => window.clearTimeout(t);
  }, [source]);

  if (q.isError) return <ErrorState error={q.error} onRetry={() => void q.refetch()} />;
  if (q.isPending) return <LoadingPanel rows={10} />;

  const errLines = (check?.diagnostics ?? []).map((d) => d.line ?? 0).filter(Boolean);

  return (
    <>
      <PageHeader
        title="Policies"
        description="Rules in Cedar that run before the decision model. A forbid stops a call; a permit either allows it outright or sends it to a person. Calls no rule matches go to the model."
        actions={
          canConfigure && (
            <>
              {dirty && (
                <Button variant="ghost" onClick={() => setDraft(null)}>
                  Discard changes
                </Button>
              )}
              <Button variant="primary" disabled={!dirty || !check?.valid} onClick={() => setSaving(true)}>
                Save as new version
              </Button>
            </>
          )
        }
      />

      <div className="grid items-start gap-5 xl:grid-cols-[minmax(0,1fr)_340px]">
        <div className="space-y-5">
          <Panel>
            <PanelHeader
              title={
                <span className="flex items-center gap-2.5">
                  {dirty ? "Draft" : `Version ${shown?.version ?? ""}`}
                  {!dirty && shown?.active && (
                    <Pill tone="proceed" dot>
                      Active
                    </Pill>
                  )}
                  {dirty && <Pill tone="safer">Unsaved</Pill>}
                </span>
              }
              description={dirty ? `Based on version ${shown?.version}.` : shown?.note || undefined}
              actions={
                <span className={cx("flex items-center gap-2 text-sm", check?.valid ? "text-proceed" : check ? "text-stop" : "text-ink-3")}>
                  {checking ? (
                    "Checking"
                  ) : check?.valid ? (
                    <>
                      <Icon.Check size={15} /> Valid
                    </>
                  ) : check ? (
                    <>
                      <Icon.Alert size={15} /> {check.diagnostics?.length} {check.diagnostics?.length === 1 ? "problem" : "problems"}
                    </>
                  ) : null}
                </span>
              }
            />
            <CodeEditor
              ref={editor}
              language="cedar"
              label="Policy source"
              value={source}
              readOnly={!canConfigure}
              errorLines={errLines}
              minHeight={460}
              onChange={setDraft}
            />
            {check && !check.valid && (
              <ul className="mt-3 space-y-1.5">
                {check.diagnostics?.map((d, i) => (
                  <li key={i}>
                    <button
                      type="button"
                      onClick={() => d.line && editor.current?.goToLine(d.line, d.column)}
                      className="flex w-full items-start gap-2 rounded-[10px] bg-stop-wash px-3 py-2 text-left text-sm hover:opacity-90"
                    >
                      <span className="figures shrink-0 text-xs text-stop">
                        {d.line ? `Line ${d.line}${d.column ? `:${d.column}` : ""}` : "Policy"}
                      </span>
                      <span className="text-ink">
                        {d.policy_id && <code className="mr-1.5 text-ink-2">{d.policy_id}</code>}
                        {d.message}
                      </span>
                    </button>
                  </li>
                ))}
              </ul>
            )}
          </Panel>

          {check?.valid && check.policies && check.policies.length > 0 && (
            <Panel>
              <PanelHeader
                title="Rules in this version"
                description="In source order. Forbid wins over any permit; a permit that sends to review wins over one that allows."
              />
              <ul className="divide-y divide-line">
                {check.policies.map((p) => (
                  <li key={p.id} className="flex items-center justify-between gap-3 py-2.5">
                    <button
                      type="button"
                      onClick={() => editor.current?.goToLine(p.line)}
                      className="truncate text-left text-sm text-ink hover:underline"
                    >
                      {p.id}
                    </button>
                    <span className="flex items-center gap-3">
                      <span className="figures text-xs text-ink-3">line {p.line}</span>
                      <Pill tone={effectTone[p.effect]}>
                        {p.effect === "allow" ? "Allows" : p.effect === "review" ? "Sends to a person" : "Stops"}
                      </Pill>
                    </span>
                  </li>
                ))}
              </ul>
            </Panel>
          )}
        </div>

        <div className="space-y-5">
          <Panel>
            <PanelHeader title="Versions" description="Every save is a new version. Activating one takes effect on the next call." />
            <ul className="-mx-2 space-y-0.5">
              {versions.map((v) => (
                <li
                  key={v.version}
                  className={cx(
                    "flex items-start gap-2 rounded-[10px] px-3 py-2.5",
                    shown?.version === v.version && !dirty ? "bg-hover" : "hover:bg-sunken",
                  )}
                >
                  <button
                    type="button"
                    onClick={() => {
                      setSelected(v.version);
                      setDraft(null);
                    }}
                    className="min-w-0 flex-1 text-left"
                  >
                    <span className="block text-sm font-medium text-ink">Version {v.version}</span>
                    <span className="block truncate text-xs text-ink-3" title={formatDateTime(v.created_at)}>
                      {v.note || "No note"}, {formatRelative(v.created_at)}
                    </span>
                  </button>
                  {v.active ? (
                    <Pill tone="proceed" dot>
                      Active
                    </Pill>
                  ) : (
                    canConfigure && (
                      <button type="button" onClick={() => setActivating(v)} className="text-xs text-ink-2 hover:text-ink hover:underline">
                        Activate
                      </button>
                    )
                  )}
                </li>
              ))}
            </ul>
          </Panel>
          <Panel>
            <PanelHeader title="Writing rules" />
            <div className="space-y-3 text-sm text-ink-2">
              <p>
                Each rule needs <code className="text-ink">@id(&quot;…&quot;)</code>. A permit also needs{" "}
                <code className="text-ink">@veto(&quot;allow&quot;)</code> or <code className="text-ink">@veto(&quot;review&quot;)</code>.
              </p>
              <dl className="space-y-2 text-xs">
                <Attr name="principal" desc="The agent: .name, .vendor" />
                <Attr name="resource" desc="The tool: .risk_class, .reversible, .registered" />
                <Attr name="context.args" desc="The call's arguments" />
                <Attr name="context.principal" desc="Who the agent acts for" />
                <Attr name="context.source" desc="api, mcp or connection" />
                <Attr name="context.hour_utc" desc="0 to 23" />
                <Attr name="context.weekday" desc="monday to sunday" />
              </dl>
              <pre className="overflow-x-auto rounded-[10px] bg-sunken p-3 text-xs leading-relaxed text-ink">{`@id("no-refunds-at-night")
forbid (
  principal,
  action,
  resource == Tool::"stripe.refund"
)
when { context.hour_utc < 6 };`}</pre>
            </div>
          </Panel>
        </div>
      </div>

      <SaveDialog
        open={saving}
        onClose={() => setSaving(false)}
        onSave={async (note, activate) => {
          const v = await api.post<PolicyVersion>("/v1/policies", { source, note, activate });
          toast.success(activate ? `Version ${v.version} saved and active` : `Version ${v.version} saved`);
          await qc.invalidateQueries({ queryKey: ["policies"] });
          setSelected(v.version);
          setDraft(null);
        }}
      />
      <ConfirmDialog
        open={!!activating}
        onClose={() => setActivating(null)}
        tone="primary"
        title={`Activate version ${activating?.version}?`}
        body="Every call from now on is checked against this version. The ledger records the change."
        confirmLabel="Activate"
        onConfirm={async () => {
          await api.post(`/v1/policies/${activating!.version}/activate`);
          toast.success(`Version ${activating!.version} is active`);
          await qc.invalidateQueries({ queryKey: ["policies"] });
        }}
      />
    </>
  );
}

function Attr({ name, desc }: { name: string; desc: string }) {
  return (
    <div className="flex gap-2">
      <dt>
        <code className="text-ink">{name}</code>
      </dt>
      <dd className="text-ink-3">{desc}</dd>
    </div>
  );
}

function SaveDialog({
  open,
  onClose,
  onSave,
}: {
  open: boolean;
  onClose: () => void;
  onSave: (note: string, activate: boolean) => Promise<void>;
}) {
  const [note, setNote] = useState("");
  const [activate, setActivate] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  return (
    <Dialog
      open={open}
      onClose={onClose}
      title="Save policies"
      width="sm"
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            Cancel
          </Button>
          <Button
            variant="primary"
            busy={busy}
            onClick={async () => {
              setBusy(true);
              setError(null);
              try {
                await onSave(note, activate);
                setNote("");
                onClose();
              } catch (err) {
                setError(errorMessage(err));
              } finally {
                setBusy(false);
              }
            }}
          >
            Save version
          </Button>
        </>
      }
    >
      <div className="space-y-4">
        <Field label="What changed" hint="Recorded with the version and in the ledger.">
          {(id, d) => <Input id={id} aria-describedby={d} autoFocus value={note} onChange={(e) => setNote(e.target.value)} />}
        </Field>
        <Checkbox checked={activate} onChange={setActivate} label="Activate now" />
        {error && <p className="text-sm text-stop">{error}</p>}
      </div>
    </Dialog>
  );
}
