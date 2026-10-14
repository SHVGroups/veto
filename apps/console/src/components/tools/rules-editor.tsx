"use client";

import { useState } from "react";
import { api, errorMessage } from "@/lib/api";
import { cx } from "@/lib/cx";
import { parseJsonInput } from "@/lib/json";
import type { OptionSpec, SaferRule } from "@/lib/types";
import { Icon } from "../icons";
import { Button, IconButton } from "../ui/button";
import { JsonDiff } from "../ui/code";
import { Input, Select, Textarea } from "../ui/form";

export const OPS: { op: string; label: string; help: string }[] = [
  { op: "swap_tool", label: "Run a different tool", help: "Same arguments, safer tool. For example create a draft instead of sending." },
  { op: "cap_number", label: "Cap a number", help: "Lowers a numeric argument to a maximum, such as a refund amount." },
  {
    op: "filter_domains",
    label: "Keep only allowed email domains",
    help: "Removes addresses outside the listed domains from a string or list.",
  },
  { op: "set", label: "Set an argument", help: "Forces an argument to a fixed value, such as read_only true." },
  { op: "remove", label: "Remove an argument", help: "Deletes an argument, such as cc or force." },
  { op: "limit_items", label: "Limit a list", help: "Keeps only the first N items of a list argument." },
];

/** A rule as edited in the form: every field is a string until it is saved. */
export interface DraftRule {
  op: string;
  tool: string;
  path: string;
  max: string;
  allow: string;
  value: string;
}

export function toDraft(r: SaferRule): DraftRule {
  return {
    op: r.op,
    tool: r.tool ?? "",
    path: r.path ?? "",
    max: r.max !== undefined ? String(r.max) : "",
    allow: (r.allow ?? []).join(", "),
    value: r.value !== undefined ? JSON.stringify(r.value) : "",
  };
}

/** Converts drafts to rules, or explains the first one that is incomplete. */
export function fromDrafts(drafts: DraftRule[]): { rules: SaferRule[] } | { error: string } {
  const rules: SaferRule[] = [];
  for (const [i, d] of drafts.entries()) {
    const n = `Rule ${i + 1}`;
    const r: SaferRule = { op: d.op };
    if (d.op === "swap_tool") {
      if (!d.tool.trim()) return { error: `${n}: name the tool to run instead.` };
      r.tool = d.tool.trim();
    } else {
      if (!d.path.trim()) return { error: `${n}: give the argument path, such as amount or options.cc.` };
      r.path = d.path.trim();
    }
    if (d.op === "cap_number" || d.op === "limit_items") {
      if (d.max.trim() === "" || Number.isNaN(Number(d.max))) return { error: `${n}: the maximum must be a number.` };
      r.max = Number(d.max);
    }
    if (d.op === "filter_domains") {
      const allow = d.allow.split(/[\s,]+/).filter(Boolean);
      if (!allow.length) return { error: `${n}: list at least one allowed domain.` };
      r.allow = allow;
    }
    if (d.op === "set") {
      const v = parseJsonInput(d.value);
      if (!v.ok) return { error: `${n}: the value must be JSON, such as true, 100 or "draft".` };
      r.value = v.value;
    }
    rules.push(r);
  }
  return { rules };
}

const blank = (op: string): DraftRule => ({ op, tool: "", path: "", max: "", allow: "", value: "" });

/**
 * Edits the ordered rules that build a tool's safer version, with a preview
 * against sample arguments. Rules apply top to bottom.
 */
export function RulesEditor({
  tool,
  drafts,
  onChange,
  disabled,
}: {
  tool: string;
  drafts: DraftRule[];
  onChange: (d: DraftRule[]) => void;
  disabled?: boolean;
}) {
  const update = (i: number, patch: Partial<DraftRule>) => onChange(drafts.map((d, j) => (j === i ? { ...d, ...patch } : d)));
  const move = (i: number, by: number) => {
    const next = [...drafts];
    const [r] = next.splice(i, 1);
    next.splice(i + by, 0, r!);
    onChange(next);
  };

  return (
    <div className="space-y-3">
      {drafts.length === 0 && (
        <p className="rounded-[12px] bg-sunken px-4 py-3 text-sm text-ink-3 shadow-[inset_0_0_0_1px_var(--line)]">
          No rules. Reviewers and the model choose only between running the call as proposed and stopping it.
        </p>
      )}
      {drafts.map((d, i) => {
        const meta = OPS.find((o) => o.op === d.op);
        return (
          <div key={i} className="rounded-[14px] bg-sunken p-4 shadow-[inset_0_0_0_1px_var(--line)]">
            <div className="flex items-start gap-3">
              <span className="figures mt-2 w-5 shrink-0 text-xs text-ink-3">{i + 1}</span>
              <div className="min-w-0 flex-1 space-y-3">
                <Select value={d.op} disabled={disabled} onChange={(e) => update(i, { ...blank(e.target.value) })} aria-label="Rule type">
                  {OPS.map((o) => (
                    <option key={o.op} value={o.op}>
                      {o.label}
                    </option>
                  ))}
                </Select>
                {meta && <p className="text-xs text-ink-3">{meta.help}</p>}
                <div className="grid gap-3 sm:grid-cols-2">
                  {d.op === "swap_tool" ? (
                    <Input
                      placeholder="Tool to run instead"
                      value={d.tool}
                      disabled={disabled}
                      onChange={(e) => update(i, { tool: e.target.value })}
                      aria-label="Tool"
                    />
                  ) : (
                    <Input
                      placeholder="Argument path, such as to or options.cc"
                      value={d.path}
                      disabled={disabled}
                      onChange={(e) => update(i, { path: e.target.value })}
                      aria-label="Path"
                    />
                  )}
                  {(d.op === "cap_number" || d.op === "limit_items") && (
                    <Input
                      inputMode="decimal"
                      placeholder={d.op === "cap_number" ? "Maximum value" : "Maximum items"}
                      value={d.max}
                      disabled={disabled}
                      onChange={(e) => update(i, { max: e.target.value })}
                      aria-label="Maximum"
                    />
                  )}
                  {d.op === "filter_domains" && (
                    <Input
                      placeholder="acme.com, acme.io"
                      value={d.allow}
                      disabled={disabled}
                      onChange={(e) => update(i, { allow: e.target.value })}
                      aria-label="Allowed domains"
                    />
                  )}
                  {d.op === "set" && (
                    <Input
                      placeholder='JSON value, such as true or "draft"'
                      value={d.value}
                      disabled={disabled}
                      onChange={(e) => update(i, { value: e.target.value })}
                      aria-label="Value"
                      className="font-mono"
                    />
                  )}
                </div>
              </div>
              {!disabled && (
                <div className="flex shrink-0 flex-col">
                  <IconButton label="Move up" disabled={i === 0} onClick={() => move(i, -1)}>
                    <Icon.ChevronDown size={15} className="rotate-180" />
                  </IconButton>
                  <IconButton label="Move down" disabled={i === drafts.length - 1} onClick={() => move(i, 1)}>
                    <Icon.ChevronDown size={15} />
                  </IconButton>
                  <IconButton label="Remove rule" onClick={() => onChange(drafts.filter((_, j) => j !== i))}>
                    <Icon.Trash size={15} />
                  </IconButton>
                </div>
              )}
            </div>
          </div>
        );
      })}
      {!disabled && (
        <Button
          size="sm"
          icon={<Icon.Plus size={14} />}
          onClick={() => onChange([...drafts, blank(tool.includes("send") ? "swap_tool" : "set")])}
        >
          Add rule
        </Button>
      )}
      <Preview tool={tool} drafts={drafts} />
    </div>
  );
}

function Preview({ tool, drafts }: { tool: string; drafts: DraftRule[] }) {
  const [args, setArgs] = useState('{\n  "to": ["someone@example.com"]\n}');
  const [res, setRes] = useState<{ before: unknown; spec: OptionSpec } | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function run() {
    setError(null);
    const parsed = parseJsonInput(args);
    if (!parsed.ok) return setError(`Sample arguments: ${parsed.message}`);
    const converted = fromDrafts(drafts);
    if ("error" in converted) return setError(converted.error);
    setBusy(true);
    try {
      const spec = await api.post<OptionSpec>("/v1/safer/preview", { tool, args: parsed.value, rules: converted.rules });
      setRes({ before: parsed.value, spec });
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="mt-2 rounded-[14px] shadow-[inset_0_0_0_1px_var(--line)]">
      <div className="border-b border-line px-4 py-3">
        <div className="text-sm font-medium text-ink">Try the rules</div>
        <p className="text-xs text-ink-3">Paste arguments an agent might send and see the safer version Veto would offer.</p>
      </div>
      <div className="grid gap-4 p-4 lg:grid-cols-2">
        <div>
          <Textarea
            rows={7}
            value={args}
            onChange={(e) => setArgs(e.target.value)}
            className="font-mono text-xs"
            aria-label="Sample arguments"
            spellCheck={false}
          />
          <Button size="sm" className="mt-2" busy={busy} onClick={() => void run()}>
            Preview safer version
          </Button>
          {error && <p className="mt-2 text-sm text-stop">{error}</p>}
        </div>
        <div className={cx(!res && "flex items-center justify-center rounded-[12px] bg-sunken p-6 text-center text-sm text-ink-3")}>
          {!res ? (
            "The result appears here."
          ) : !res.spec.available ? (
            <p className="text-sm text-ink-2">These rules change nothing for these arguments, so no safer version would be offered.</p>
          ) : (
            <div className="space-y-2">
              {res.spec.tool !== tool && (
                <p className="text-sm text-ink-2">
                  Runs <code className="text-ink">{res.spec.tool}</code>
                </p>
              )}
              <ul className="space-y-1">
                {res.spec.changes?.map((c) => (
                  <li key={c} className="flex gap-2 text-sm text-ink">
                    <Icon.Check size={15} className="mt-0.5 text-safer" />
                    {c}
                  </li>
                ))}
              </ul>
              <JsonDiff before={res.before} after={res.spec.args} className="max-h-56" />
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
