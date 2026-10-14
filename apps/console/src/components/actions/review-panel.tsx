"use client";

import { useQueryClient } from "@tanstack/react-query";
import { useEffect, useMemo, useState } from "react";
import { api, errorMessage } from "@/lib/api";
import { cx } from "@/lib/cx";
import { optionLabel, optionLetter } from "@/lib/format";
import type { ActionDetail, Option, OptionSpec } from "@/lib/types";
import { Icon } from "../icons";
import { useCan } from "../shell/session";
import { Button } from "../ui/button";
import { JsonDiff, JsonView } from "../ui/code";
import { Field, Input, Textarea } from "../ui/form";
import { Kbd } from "../ui/layout";
import { useToast } from "../ui/toast";

const toneRing: Record<Option, string> = {
  proceed: "shadow-[inset_0_0_0_2px_var(--proceed)]",
  proceed_safer: "shadow-[inset_0_0_0_2px_var(--safer)]",
  stop: "shadow-[inset_0_0_0_2px_var(--stop)]",
};
const toneBg: Record<Option, string> = { proceed: "bg-proceed", proceed_safer: "bg-safer", stop: "bg-stop" };
const confirmVariant: Record<Option, "proceed" | "safer" | "stop"> = { proceed: "proceed", proceed_safer: "safer", stop: "stop" };
const doneMessage: Record<Option, string> = {
  proceed: "Approved as proposed",
  proceed_safer: "Approved the safer version",
  stop: "Stopped",
};

/**
 * The decision a reviewer makes on a held action: pick one of the three
 * options, optionally say why, and confirm. Keys 1, 2 and 3 pick an option
 * and Enter confirms when keyboard shortcuts are enabled.
 */
export function ReviewPanel({
  detail,
  onDecided,
  keyboard = false,
}: {
  detail: ActionDetail;
  onDecided?: (option: Option) => void;
  keyboard?: boolean;
}) {
  const a = detail.action;
  const qc = useQueryClient();
  const toast = useToast();
  const canReview = useCan("review");
  const options = useMemo(() => a.options.filter((o) => o.available), [a.options]);
  const recommended = a.ranking.find((r) => options.some((o) => o.key === r));
  const [choice, setChoice] = useState<Option | null>(null);
  const [note, setNote] = useState("");
  const [password, setPassword] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const proposed = a.options.find((o) => o.key === "proceed");

  async function submit(option: Option | null = choice) {
    if (!option || busy) return;
    const stepUp = !!detail.tool?.step_up && option !== "stop";
    if (stepUp && !password) {
      setChoice(option);
      setError("This tool asks you to confirm your password before approving.");
      return;
    }
    setBusy(true);
    setError(null);
    try {
      await api.post(`/v1/actions/${a.id}/review`, { option, note, password: stepUp ? password : undefined });
      toast.success(`${doneMessage[option]}: ${a.tool}`);
      void qc.invalidateQueries({ queryKey: ["actions"] });
      void qc.invalidateQueries({ queryKey: ["action", a.id] });
      void qc.invalidateQueries({ queryKey: ["overview"] });
      onDecided?.(option);
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  }

  useEffect(() => {
    if (!keyboard || !canReview) return;
    function onKey(e: KeyboardEvent) {
      const t = e.target as HTMLElement;
      if (t.closest("input,textarea,select,[contenteditable=true]") || e.metaKey || e.ctrlKey || e.altKey) return;
      const idx = ["1", "2", "3"].indexOf(e.key);
      const letter = ["a", "b", "c"].indexOf(e.key.toLowerCase());
      const pick = idx >= 0 ? idx : letter;
      if (pick >= 0) {
        const key = (["proceed", "proceed_safer", "stop"] as Option[])[pick]!;
        if (options.some((o) => o.key === key)) {
          e.preventDefault();
          setChoice(key);
        }
      } else if (e.key === "Enter" && choice) {
        e.preventDefault();
        void submit();
      }
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
    // submit reads current state through the closure re-created each render.
  });

  if (a.status !== "pending_review") return null;

  return (
    <div>
      <div className="space-y-2.5" role="radiogroup" aria-label="Choose an option">
        {(["proceed", "proceed_safer", "stop"] as Option[]).map((key) => {
          const spec = a.options.find((o) => o.key === key);
          return (
            <OptionCard
              key={key}
              spec={spec}
              option={key}
              proposed={proposed}
              selected={choice === key}
              recommended={recommended === key}
              disabled={!spec?.available || !canReview}
              onSelect={() => setChoice(key)}
            />
          );
        })}
      </div>

      {canReview ? (
        <div className="mt-5 space-y-4">
          <Field label="Note" optional hint="Saved with the decision and sent to the agent.">
            {(id, d) => (
              <Textarea id={id} aria-describedby={d} rows={2} value={note} onChange={(e) => setNote(e.target.value)} maxLength={2000} />
            )}
          </Field>
          {detail.tool?.step_up && choice !== "stop" && (
            <Field label="Your password" hint="Approving this tool needs you to confirm it is you.">
              {(id, d) => (
                <Input
                  id={id}
                  aria-describedby={d}
                  type="password"
                  autoComplete="current-password"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                />
              )}
            </Field>
          )}
          {error && (
            <p role="alert" className="rounded-[12px] bg-stop-wash px-3.5 py-2.5 text-sm text-stop">
              {error}
            </p>
          )}
          <div className="flex flex-wrap items-center justify-between gap-3">
            {keyboard ? (
              <p className="hidden items-center gap-1.5 text-xs text-ink-3 sm:flex">
                <Kbd>1</Kbd>
                <Kbd>2</Kbd>
                <Kbd>3</Kbd> choose, <Kbd>Enter</Kbd> confirm, <Kbd>J</Kbd>
                <Kbd>K</Kbd> move
              </p>
            ) : (
              <span />
            )}
            <Button
              variant={choice ? confirmVariant[choice] : "primary"}
              size="lg"
              disabled={!choice}
              busy={busy}
              onClick={() => void submit()}
            >
              {choice
                ? choice === "stop"
                  ? "Stop this action"
                  : choice === "proceed_safer"
                    ? "Approve the safer version"
                    : "Approve as proposed"
                : "Choose an option"}
            </Button>
          </div>
        </div>
      ) : (
        <p className="mt-4 text-sm text-ink-3">Your role can see held actions but not decide them.</p>
      )}
    </div>
  );
}

function OptionCard({
  spec,
  option,
  proposed,
  selected,
  recommended,
  disabled,
  onSelect,
}: {
  spec: OptionSpec | undefined;
  option: Option;
  proposed: OptionSpec | undefined;
  selected: boolean;
  recommended: boolean;
  disabled: boolean;
  onSelect: () => void;
}) {
  const [open, setOpen] = useState(option !== "stop");
  const unavailable = !spec?.available;
  return (
    <div
      className={cx(
        "rounded-[16px] bg-surface transition-shadow",
        selected ? toneRing[option] : "shadow-[inset_0_0_0_1px_var(--line-strong)]",
        unavailable && "opacity-55",
      )}
    >
      <button
        type="button"
        role="radio"
        aria-checked={selected}
        disabled={disabled}
        onClick={onSelect}
        className="flex w-full items-start gap-3.5 rounded-[16px] p-4 text-left disabled:cursor-not-allowed"
      >
        <span
          className={cx(
            "mt-0.5 inline-flex size-7 shrink-0 items-center justify-center rounded-[9px] text-sm font-semibold text-white",
            unavailable ? "bg-ink-3" : toneBg[option],
          )}
        >
          {optionLetter[option]}
        </span>
        <span className="min-w-0 flex-1">
          <span className="flex flex-wrap items-center gap-2">
            <span className="text-base font-medium text-ink">{optionLabel[option]}</span>
            {recommended && !unavailable && <span className="text-xs text-ink-3">Ranked first by Veto</span>}
          </span>
          <span className="mt-0.5 block text-sm text-ink-2">
            {option === "proceed" && (
              <>
                Runs <code className="text-ink">{spec?.tool}</code> with the arguments the agent sent.
              </>
            )}
            {option === "proceed_safer" &&
              (unavailable ? (
                "No safer version: this tool has no rules that change this call."
              ) : (
                <>
                  Runs <code className="text-ink">{spec?.tool}</code> with changes made by the tool&rsquo;s rules.
                </>
              ))}
            {option === "stop" && "Nothing runs. The agent is told the action was stopped."}
          </span>
          {option === "proceed_safer" && spec?.changes && spec.changes.length > 0 && (
            <span className="mt-2 flex flex-col gap-1">
              {spec.changes.map((c) => (
                <span key={c} className="flex items-start gap-2 text-sm text-ink">
                  <Icon.Check size={15} className="mt-0.5 shrink-0 text-safer" />
                  {c}
                </span>
              ))}
            </span>
          )}
        </span>
        <span
          className={cx(
            "mt-1 inline-flex size-5 shrink-0 items-center justify-center rounded-full",
            selected ? toneBg[option] : "shadow-[inset_0_0_0_1.5px_var(--line-strong)]",
          )}
        >
          {selected && <span className="size-2 rounded-full bg-white" />}
        </span>
      </button>
      {!unavailable && option !== "stop" && (
        <div className="px-4 pb-4 pl-[58px]">
          <button type="button" onClick={() => setOpen((o) => !o)} className="flex items-center gap-1 text-xs text-ink-3 hover:text-ink">
            <Icon.ChevronRight size={13} className={cx("transition-transform", open && "rotate-90")} />
            {option === "proceed" ? "Arguments" : "Changes to the arguments"}
          </button>
          {open && (
            <div className="mt-2">
              {option === "proceed" ? (
                <JsonView value={spec?.args ?? {}} maxHeight={240} label="arguments" />
              ) : (
                <JsonDiff before={proposed?.args ?? {}} after={spec?.args ?? {}} className="max-h-60" />
              )}
            </div>
          )}
        </div>
      )}
    </div>
  );
}
