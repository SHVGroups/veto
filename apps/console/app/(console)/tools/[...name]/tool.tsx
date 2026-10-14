"use client";

import { useQueryClient } from "@tanstack/react-query";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState } from "react";
import { Icon } from "@/components/icons";
import { useCan } from "@/components/shell/session";
import { fromDrafts, RulesEditor, toDraft, type DraftRule } from "@/components/tools/rules-editor";
import { Button } from "@/components/ui/button";
import { ConfirmDialog } from "@/components/ui/dialog";
import { Field, Input, Select, Switch, Textarea } from "@/components/ui/form";
import { PageHeader, Panel, PanelHeader } from "@/components/ui/layout";
import { Pill } from "@/components/ui/pills";
import { ErrorState, LoadingPanel, Notice } from "@/components/ui/states";
import { useToast } from "@/components/ui/toast";
import { ApiError, api, errorMessage, toolPath } from "@/lib/api";
import { cx } from "@/lib/cx";
import { formatProbability, riskLabel } from "@/lib/format";
import { useMeta, usePacks, useTool } from "@/lib/queries";
import type { FailMode, RiskClass, Thresholds, Tool, ToolInput } from "@/lib/types";

const RISK_HELP: Record<RiskClass, string> = {
  read: "Reads data and changes nothing. Allowed by the default policy without asking the model.",
  low: "Small, easily undone changes, such as posting to an internal channel.",
  medium: "Changes people notice, such as sending an email to a customer.",
  high: "Hard to undo or costly, such as refunds or merges. The model is asked twice.",
  critical: "Irreversible or dangerous. Always held for a person.",
};

interface Form {
  description: string;
  risk_class: RiskClass;
  reversible: boolean;
  pack: string;
  fail_mode: FailMode;
  step_up: boolean;
  max_per_hour: string;
  auto_confidence: string;
  risk_ceiling: string;
  block_confidence: string;
  double_check: "" | "on" | "off";
}

function toForm(t: Tool | null): Form {
  return {
    description: t?.description ?? "",
    risk_class: t?.registered ? t.risk_class : "medium",
    reversible: t?.reversible ?? false,
    pack: t?.pack || "general",
    fail_mode: t?.fail_mode ?? "closed",
    step_up: t?.step_up ?? false,
    max_per_hour: t?.max_per_hour?.toString() ?? "",
    auto_confidence: t?.auto_confidence?.toString() ?? "",
    risk_ceiling: t?.risk_ceiling?.toString() ?? "",
    block_confidence: t?.block_confidence?.toString() ?? "",
    double_check: t?.double_check === null || t?.double_check === undefined ? "" : t.double_check ? "on" : "off",
  };
}

function optionalNumber(v: string, name: string): number | null {
  if (v.trim() === "") return null;
  const n = Number(v);
  if (Number.isNaN(n) || n < 0 || n > 1) throw new Error(`${name} must be between 0 and 1, or empty to use the default.`);
  return n;
}

export function ToolEditor({ name }: { name: string }) {
  const q = useTool(name);
  const isNew = q.error instanceof ApiError && q.error.status === 404;
  if (q.isError && !isNew) return <ErrorState error={q.error} onRetry={() => void q.refetch()} />;
  if (q.isPending) return <LoadingPanel rows={10} />;
  const tool = q.data?.tool ?? null;
  // A saved tool comes back with a new updated_at, which resets the form to it.
  return <Editor key={tool?.updated_at ?? "new"} name={name} tool={tool} />;
}

function Editor({ name, tool }: { name: string; tool: Tool | null }) {
  const router = useRouter();
  const qc = useQueryClient();
  const toast = useToast();
  const meta = useMeta();
  const packs = usePacks();
  const canConfigure = useCan("configure");
  const [form, setForm] = useState<Form>(() => toForm(tool));
  const [drafts, setDrafts] = useState<DraftRule[]>(() => (tool?.safer ?? []).map(toDraft));
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [deleting, setDeleting] = useState(false);

  const defaults: Thresholds | undefined = meta.data?.risk_classes.find((r) => r.name === form.risk_class)?.thresholds;
  const set = <K extends keyof Form>(k: K, v: Form[K]) => setForm((f) => ({ ...f, [k]: v }));
  const disabled = !canConfigure;
  const highRisk = form.risk_class === "high" || form.risk_class === "critical";

  async function save() {
    setError(null);
    let input: ToolInput;
    try {
      const rules = fromDrafts(drafts);
      if ("error" in rules) throw new Error(rules.error);
      const maxPerHour = form.max_per_hour.trim() === "" ? null : Number(form.max_per_hour);
      if (maxPerHour !== null && (!Number.isInteger(maxPerHour) || maxPerHour < 1))
        throw new Error("The hourly limit must be a whole number above zero, or empty for no limit.");
      input = {
        description: form.description,
        risk_class: form.risk_class,
        reversible: form.reversible,
        pack: form.pack,
        safer: rules.rules,
        auto_confidence: optionalNumber(form.auto_confidence, "Runs above"),
        risk_ceiling: optionalNumber(form.risk_ceiling, "Risk ceiling"),
        block_confidence: optionalNumber(form.block_confidence, "Stops above"),
        double_check: form.double_check === "" ? null : form.double_check === "on",
        fail_mode: highRisk ? "closed" : form.fail_mode,
        max_per_hour: maxPerHour,
        step_up: form.step_up,
      };
    } catch (err) {
      setError(errorMessage(err));
      return;
    }
    setBusy(true);
    try {
      await api.put(`/v1/tools/${toolPath(name)}`, input);
      toast.success(tool?.registered ? "Tool saved" : `${name} classified as ${riskLabel[input.risk_class].toLowerCase()} risk`);
      void qc.invalidateQueries({ queryKey: ["tools"] });
      void qc.invalidateQueries({ queryKey: ["overview"] });
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  }

  const packOptions = Array.from(new Set([...(meta.data?.packs ?? []), ...(packs.data?.items.map((p) => p.name) ?? []), form.pack]));

  return (
    <>
      <PageHeader
        meta={
          <Link href="/tools" className="inline-flex items-center gap-1 hover:text-ink">
            <Icon.ChevronLeft size={14} />
            Tools
          </Link>
        }
        title={
          <span className="flex flex-wrap items-center gap-3">
            {name}
            {!tool ? (
              <Pill tone="held">New</Pill>
            ) : (
              !tool.registered && (
                <Pill tone="held" dot>
                  Needs a risk class
                </Pill>
              )
            )}
          </span>
        }
        description={tool ? "How Veto handles calls to this tool." : "Classify this tool before agents call it."}
        actions={
          canConfigure && (
            <>
              {tool && (
                <Button variant="ghost" icon={<Icon.Trash size={15} />} onClick={() => setDeleting(true)}>
                  Delete
                </Button>
              )}
              <Button variant="primary" busy={busy} onClick={() => void save()}>
                {tool?.registered ? "Save changes" : "Save classification"}
              </Button>
            </>
          )
        }
      />
      {error && (
        <p role="alert" className="mb-5 rounded-[12px] bg-stop-wash px-4 py-3 text-sm text-stop">
          {error}
        </p>
      )}
      {tool && !tool.registered && (
        <Notice tone="held" className="mb-5" title="Agents already call this tool">
          Until you save a risk class, every call to it waits for a person.
        </Notice>
      )}

      <div className="grid items-start gap-5 xl:grid-cols-[minmax(0,1fr)_400px]">
        <div className="space-y-5">
          <Panel>
            <PanelHeader title="Risk" description="The class sets default thresholds. Pick by the worst thing a single call can do." />
            <div className="grid gap-2 sm:grid-cols-5" role="radiogroup" aria-label="Risk class">
              {(["read", "low", "medium", "high", "critical"] as RiskClass[]).map((r) => (
                <button
                  key={r}
                  type="button"
                  role="radio"
                  aria-checked={form.risk_class === r}
                  disabled={disabled}
                  onClick={() => set("risk_class", r)}
                  className={cx(
                    "rounded-[12px] px-3 py-2.5 text-left text-sm transition-colors",
                    form.risk_class === r
                      ? "bg-ink text-surface"
                      : "bg-sunken text-ink-2 shadow-[inset_0_0_0_1px_var(--line)] hover:text-ink",
                  )}
                >
                  {riskLabel[r]}
                </button>
              ))}
            </div>
            <p className="mt-3 text-sm text-ink-2">{RISK_HELP[form.risk_class]}</p>
            <div className="mt-6 grid gap-5 sm:grid-cols-2">
              <Field label="Description" optional hint="Shown to the decision model. Say what the tool does in one sentence.">
                {(id, d) => (
                  <Textarea
                    id={id}
                    aria-describedby={d}
                    rows={3}
                    disabled={disabled}
                    value={form.description}
                    onChange={(e) => set("description", e.target.value)}
                    maxLength={500}
                  />
                )}
              </Field>
              <Field label="Question pack" hint="The questions the decision model answers about each call.">
                {(id, d) => (
                  <Select id={id} aria-describedby={d} disabled={disabled} value={form.pack} onChange={(e) => set("pack", e.target.value)}>
                    {packOptions.map((p) => (
                      <option key={p} value={p}>
                        {p}
                      </option>
                    ))}
                  </Select>
                )}
              </Field>
            </div>
          </Panel>

          <Panel>
            <PanelHeader
              title="Safer version"
              description="Rules that turn a proposed call into option B. They run in order, in code: the model never writes arguments."
            />
            <RulesEditor tool={name} drafts={drafts} onChange={setDrafts} disabled={disabled} />
          </Panel>
        </div>

        <div className="space-y-5">
          <Panel>
            <PanelHeader title="Thresholds" description="Leave empty to use the class default, shown in grey." />
            <div className="space-y-4">
              <Field label="Runs without a person above" hint="Minimum confidence for the model to approve alone.">
                {(id, d) => (
                  <Input
                    id={id}
                    aria-describedby={d}
                    inputMode="decimal"
                    disabled={disabled || defaults?.auto_proceed === false}
                    placeholder={
                      defaults ? (defaults.auto_proceed ? formatProbability(defaults.auto_confidence) : "Never for this class") : ""
                    }
                    value={form.auto_confidence}
                    onChange={(e) => set("auto_confidence", e.target.value)}
                  />
                )}
              </Field>
              <Field label="Risk ceiling" hint="Above this composite risk, a person decides.">
                {(id, d) => (
                  <Input
                    id={id}
                    aria-describedby={d}
                    inputMode="decimal"
                    disabled={disabled}
                    placeholder={defaults ? formatProbability(defaults.risk_ceiling) : ""}
                    value={form.risk_ceiling}
                    onChange={(e) => set("risk_ceiling", e.target.value)}
                  />
                )}
              </Field>
              <Field label="Stops without a person above" hint="Minimum confidence for the model to stop a call alone.">
                {(id, d) => (
                  <Input
                    id={id}
                    aria-describedby={d}
                    inputMode="decimal"
                    disabled={disabled}
                    placeholder={defaults ? formatProbability(defaults.block_confidence) : ""}
                    value={form.block_confidence}
                    onChange={(e) => set("block_confidence", e.target.value)}
                  />
                )}
              </Field>
              <Field label="Ask twice with options reversed" hint={`Class default: ${defaults?.double_check ? "on" : "off"}.`}>
                {(id, d) => (
                  <Select
                    id={id}
                    aria-describedby={d}
                    disabled={disabled}
                    value={form.double_check}
                    onChange={(e) => set("double_check", e.target.value as Form["double_check"])}
                  >
                    <option value="">Class default</option>
                    <option value="on">Always</option>
                    <option value="off">Never</option>
                  </Select>
                )}
              </Field>
            </div>
          </Panel>
          <Panel>
            <PanelHeader title="Safeguards" />
            <div className="space-y-5">
              <Field label="Calls per hour, per agent" optional hint="Over the limit, calls wait for a person.">
                {(id, d) => (
                  <Input
                    id={id}
                    aria-describedby={d}
                    inputMode="numeric"
                    disabled={disabled}
                    placeholder="No limit"
                    value={form.max_per_hour}
                    onChange={(e) => set("max_per_hour", e.target.value)}
                  />
                )}
              </Field>
              <Switch
                checked={form.step_up}
                disabled={disabled}
                onChange={(v) => set("step_up", v)}
                label="Ask reviewers for their password"
                description="Approving a held call needs the reviewer to confirm it is them."
              />
              <Switch
                checked={form.reversible}
                disabled={disabled}
                onChange={(v) => set("reversible", v)}
                label="Calls can be undone"
                description="Tells the decision model that mistakes can be reversed."
              />
              <Switch
                checked={!highRisk && form.fail_mode === "open"}
                disabled={disabled || highRisk}
                onChange={(v) => set("fail_mode", v ? "open" : "closed")}
                label="Allow calls when the model is unavailable"
                description={
                  highRisk
                    ? "High and critical tools always wait for a person when the model cannot answer."
                    : "Off means calls wait for a person when the model cannot answer."
                }
              />
            </div>
          </Panel>
        </div>
      </div>

      <ConfirmDialog
        open={deleting}
        onClose={() => setDeleting(false)}
        title={`Delete ${name}?`}
        body="Its classification and rules are removed. The next call to it registers it again as a tool that needs a risk class."
        confirmLabel="Delete tool"
        onConfirm={async () => {
          await api.del(`/v1/tools/${toolPath(name)}`);
          toast.success("Tool deleted");
          void qc.invalidateQueries({ queryKey: ["tools"] });
          router.replace("/tools");
        }}
      />
    </>
  );
}
