"use client";

import { useState } from "react";
import { AssessmentView } from "@/components/actions/assessment";
import { ReasonList } from "@/components/actions/reasons";
import { CodeEditor } from "@/components/editor/code-editor";
import { Icon } from "@/components/icons";
import { Button, IconButton } from "@/components/ui/button";
import { JsonDiff, JsonView } from "@/components/ui/code";
import { Field, Input, Select, Switch, Textarea } from "@/components/ui/form";
import { PageHeader, Panel } from "@/components/ui/layout";
import { OptionTag, Pill, RiskTag } from "@/components/ui/pills";
import { ErrorState, Notice } from "@/components/ui/states";
import { Tabs } from "@/components/ui/table";
import { api } from "@/lib/api";
import { formatProbability, optionLabel } from "@/lib/format";
import { parseJsonInput } from "@/lib/json";
import { useAgents, useMeta, useTools } from "@/lib/queries";
import type { SimulateResult, Untrusted } from "@/lib/types";

const gateLabel = {
  deny: "A policy stops this call",
  review: "A policy sends this call to a person",
  allow: "A policy allows this call without the model",
  continue: "No policy matched; the decision model decides",
} as const;

export function Playground() {
  const agents = useAgents();
  const tools = useTools();
  const meta = useMeta();
  const [agentId, setAgentId] = useState("");
  const [tool, setTool] = useState("gmail.send");
  const [args, setArgs] = useState(
    '{\n  "to": ["jane@globex.com"],\n  "subject": "Your refund",\n  "body": "We have refunded your order."\n}',
  );
  const [request, setRequest] = useState("Tell the customer their refund went through");
  const [principal, setPrincipal] = useState("");
  const [untrusted, setUntrusted] = useState<Untrusted[]>([]);
  const [callModel, setCallModel] = useState(true);
  const [result, setResult] = useState<SimulateResult | null>(null);
  const [askedModel, setAskedModel] = useState(true);
  const [error, setError] = useState<unknown>(null);
  const [busy, setBusy] = useState(false);
  const [tab, setTab] = useState<"decision" | "options" | "model">("decision");
  const agent = agentId || agents.data?.items[0]?.id || "";

  async function run() {
    const parsed = parseJsonInput(args);
    if (!parsed.ok) return setError(new Error(`Arguments: ${parsed.message}`));
    setBusy(true);
    setError(null);
    setAskedModel(callModel);
    try {
      setResult(
        await api.post<SimulateResult>("/v1/simulate", {
          agent_id: agent,
          tool,
          args: parsed.value,
          request,
          untrusted: untrusted.filter((u) => u.content.trim()),
          principal,
          call_model: callModel,
        }),
      );
    } catch (err) {
      setError(err);
    } finally {
      setBusy(false);
    }
  }

  const r = result;
  const offered = r?.options.filter((o) => o.available).map((o) => o.key) ?? [];
  const proposed = r?.options.find((o) => o.key === "proceed");
  const safer = r?.options.find((o) => o.key === "proceed_safer");

  return (
    <>
      <PageHeader
        title="Playground"
        description="Run a call through policies, the safer-version rules and the decision model without recording it or running anything. Use it to test a change before agents feel it."
      />
      <div className="grid items-start gap-5 xl:grid-cols-[440px_minmax(0,1fr)]">
        <Panel className="xl:sticky xl:top-6">
          <form
            className="space-y-4"
            onSubmit={(e) => {
              e.preventDefault();
              void run();
            }}
          >
            <div className="grid gap-4 sm:grid-cols-2">
              <Field label="Agent">
                {(id) => (
                  <Select id={id} value={agent} onChange={(e) => setAgentId(e.target.value)}>
                    {agents.data?.items.map((a) => (
                      <option key={a.id} value={a.id}>
                        {a.name}
                      </option>
                    ))}
                  </Select>
                )}
              </Field>
              <Field label="Tool">
                {(id) => (
                  <>
                    <Input id={id} list="playground-tools" value={tool} onChange={(e) => setTool(e.target.value.trim())} />
                    <datalist id="playground-tools">
                      {tools.data?.items.map((t) => (
                        <option key={t.name} value={t.name} />
                      ))}
                    </datalist>
                  </>
                )}
              </Field>
            </div>
            <div>
              <div className="mb-1.5 text-sm font-medium text-ink">Arguments</div>
              <CodeEditor language="json" label="Arguments" value={args} onChange={setArgs} minHeight={170} />
            </div>
            <Field label="What the agent was asked to do" optional>
              {(id) => <Textarea id={id} rows={2} value={request} onChange={(e) => setRequest(e.target.value)} />}
            </Field>
            <Field label="On behalf of" optional hint="The user id the agent acts for; policies can use it.">
              {(id, d) => <Input id={id} aria-describedby={d} value={principal} onChange={(e) => setPrincipal(e.target.value)} />}
            </Field>
            <div>
              <div className="mb-1.5 flex items-center justify-between">
                <span className="text-sm font-medium text-ink">Untrusted content</span>
                <Button
                  size="sm"
                  variant="ghost"
                  icon={<Icon.Plus size={14} />}
                  onClick={() => setUntrusted([...untrusted, { source: "email", content: "" }])}
                >
                  Add
                </Button>
              </div>
              {untrusted.length === 0 && (
                <p className="text-xs text-ink-3">Add text the agent read, such as an email, to test prompt injection.</p>
              )}
              <div className="space-y-2">
                {untrusted.map((u, i) => (
                  <div key={i} className="flex gap-2">
                    <div className="flex-1 space-y-1.5">
                      <Input
                        value={u.source}
                        placeholder="Source"
                        onChange={(e) => setUntrusted(untrusted.map((x, j) => (j === i ? { ...x, source: e.target.value } : x)))}
                        aria-label="Source"
                      />
                      <Textarea
                        rows={2}
                        value={u.content}
                        placeholder="Content"
                        onChange={(e) => setUntrusted(untrusted.map((x, j) => (j === i ? { ...x, content: e.target.value } : x)))}
                        aria-label="Content"
                      />
                    </div>
                    <IconButton label="Remove" onClick={() => setUntrusted(untrusted.filter((_, j) => j !== i))}>
                      <Icon.Trash size={15} />
                    </IconButton>
                  </div>
                ))}
              </div>
            </div>
            <Switch
              checked={callModel}
              onChange={setCallModel}
              disabled={meta.data?.model.configured === false}
              label="Ask the decision model"
              description={
                meta.data?.model.configured === false
                  ? "No decision model is configured on this server."
                  : "Off shows the exact request without sending it."
              }
            />
            <Button type="submit" variant="primary" size="lg" className="w-full" busy={busy} disabled={!agent || !tool}>
              Run simulation
            </Button>
          </form>
        </Panel>

        <div className="space-y-5">
          {error !== null && <ErrorState error={error} />}
          {!r ? (
            <Panel>
              <div className="py-16 text-center">
                <Icon.Playground size={28} className="mx-auto text-ink-3" />
                <p className="mt-3 text-sm text-ink-2">Results show what Veto would decide and why. Nothing is recorded.</p>
              </div>
            </Panel>
          ) : (
            <>
              <Panel>
                <div className="flex flex-wrap items-start justify-between gap-4">
                  <div>
                    <div className="text-sm text-ink-3">Veto would</div>
                    <div className="mt-1 text-2xl text-ink">
                      {r.verdict.route === "auto" && r.verdict.option
                        ? r.verdict.option === "stop"
                          ? "Stop this call"
                          : optionLabel[r.verdict.option]
                        : "Hold it for a person"}
                    </div>
                  </div>
                  <div className="flex items-center gap-2">
                    <RiskTag risk={r.tool.risk_class} />
                    {!r.tool.registered && <Pill tone="held">Tool not classified</Pill>}
                    {r.verdict.route === "auto" && r.verdict.option ? (
                      <OptionTag option={r.verdict.option} />
                    ) : (
                      <Pill tone="held" dot>
                        Held
                      </Pill>
                    )}
                  </div>
                </div>
                <div className="mt-5 grid gap-5 md:grid-cols-2">
                  <div>
                    <div className="mb-1 text-xs text-ink-3">Because</div>
                    <ReasonList reasons={r.verdict.reasons} className="space-y-0.5" />
                  </div>
                  <div>
                    <div className="mb-1 text-xs text-ink-3">Policy gate</div>
                    <p className="text-sm text-ink">{gateLabel[r.gate.outcome]}</p>
                    {r.gate.policies && r.gate.policies.length > 0 && (
                      <p className="text-xs text-ink-3">Matched: {r.gate.policies.join(", ")}</p>
                    )}
                    <p className="mt-2 text-xs text-ink-3">
                      Pack {r.pack}. Runs above {formatProbability(r.thresholds.auto_confidence)}, risk ceiling{" "}
                      {formatProbability(r.thresholds.risk_ceiling)}.
                    </p>
                  </div>
                </div>
                {!askedModel && r.gate.outcome === "continue" && (
                  <Notice className="mt-5" title="The model was not asked">
                    This verdict is what happens when no model answers. Turn on Ask the decision model to see a real decision.
                  </Notice>
                )}
                {r.model_error && (
                  <Notice tone="stop" className="mt-5" title="The decision model call failed">
                    {r.model_error}
                  </Notice>
                )}
              </Panel>
              <Panel>
                <Tabs
                  className="mb-5"
                  value={tab}
                  onChange={setTab}
                  tabs={[
                    { value: "decision", label: "Model's answer" },
                    { value: "options", label: "Options" },
                    { value: "model", label: "Request to the model" },
                  ]}
                />
                {tab === "decision" &&
                  (r.assessment ? (
                    <AssessmentView a={r.assessment} thresholds={r.thresholds} offered={offered} />
                  ) : (
                    <p className="text-sm text-ink-3">
                      {r.gate.outcome !== "continue"
                        ? "A policy decided before the model was needed."
                        : r.model_error
                          ? "The model did not answer. The error is shown above."
                          : "The model was not asked. Turn on Ask the decision model to see its answer."}
                    </p>
                  ))}
                {tab === "options" && (
                  <div className="space-y-5">
                    <div>
                      <div className="mb-1.5 text-sm text-ink">A. As proposed</div>
                      <JsonView value={proposed?.args ?? {}} maxHeight={220} />
                    </div>
                    <div>
                      <div className="mb-1.5 text-sm text-ink">
                        B. Safer version {safer?.available && safer.tool !== tool && <span className="text-ink-3">runs {safer.tool}</span>}
                      </div>
                      {safer?.available ? (
                        <>
                          <ul className="mb-2 space-y-1">
                            {safer.changes?.map((c) => (
                              <li key={c} className="flex gap-2 text-sm text-ink">
                                <Icon.Check size={15} className="mt-0.5 text-safer" />
                                {c}
                              </li>
                            ))}
                          </ul>
                          <JsonDiff before={proposed?.args ?? {}} after={safer.args ?? {}} />
                        </>
                      ) : (
                        <p className="text-sm text-ink-3">Not offered: the tool&rsquo;s rules change nothing for these arguments.</p>
                      )}
                    </div>
                  </div>
                )}
                {tab === "model" &&
                  (r.model_request ? (
                    <>
                      <p className="mb-3 text-sm text-ink-2">
                        The typed questions Veto sends to TypeSafe System One. The model answers with probabilities only.
                      </p>
                      <JsonView value={r.model_request} maxHeight={560} label="model request" />
                    </>
                  ) : (
                    <p className="text-sm text-ink-3">No model request: a policy decided first.</p>
                  ))}
              </Panel>
            </>
          )}
        </div>
      </div>
    </>
  );
}
