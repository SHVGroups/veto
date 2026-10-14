import type { ReactNode } from "react";
import { cx } from "@/lib/cx";
import { decidedByLabel, optionShort, outcomeLabel, riskLabel } from "@/lib/format";
import type { ActionStatus, DecidedBy, Option, OutcomeStatus, RiskClass } from "@/lib/types";

type Tone = "proceed" | "safer" | "stop" | "held" | "neutral" | "muted";

const tones: Record<Tone, { text: string; wash: string; dot: string }> = {
  proceed: { text: "text-proceed", wash: "bg-proceed-wash", dot: "bg-proceed" },
  safer: { text: "text-safer", wash: "bg-safer-wash", dot: "bg-safer" },
  stop: { text: "text-stop", wash: "bg-stop-wash", dot: "bg-stop" },
  held: { text: "text-held", wash: "bg-held-wash", dot: "bg-held" },
  neutral: { text: "text-ink-2", wash: "bg-sunken shadow-[inset_0_0_0_1px_var(--line)]", dot: "bg-ink-2" },
  muted: { text: "text-ink-3", wash: "bg-sunken", dot: "bg-ink-3" },
};

export const optionTone: Record<Option, Tone> = { proceed: "proceed", proceed_safer: "safer", stop: "stop" };

export function Pill({
  tone = "neutral",
  children,
  dot,
  className,
}: {
  tone?: Tone;
  children: ReactNode;
  dot?: boolean;
  className?: string;
}) {
  const t = tones[tone];
  return (
    <span
      className={cx(
        "inline-flex h-6 items-center gap-1.5 rounded-full px-2.5 text-xs font-medium whitespace-nowrap",
        t.wash,
        t.text,
        className,
      )}
    >
      {dot && <span className={cx("size-1.5 rounded-full", t.dot)} />}
      {children}
    </span>
  );
}

/** What happened to an action, in one chip: waiting, which option ran, or stopped. */
export function DecisionChip({ status, option }: { status: ActionStatus; option: Option | null }) {
  switch (status) {
    case "pending_review":
      return (
        <Pill tone="held" dot>
          Waiting
        </Pill>
      );
    case "evaluating":
      return <Pill tone="muted">Evaluating</Pill>;
    case "expired":
      return (
        <Pill tone="stop" dot>
          Expired
        </Pill>
      );
    case "denied":
      return (
        <Pill tone="stop" dot>
          Stopped
        </Pill>
      );
    case "approved":
      return (
        <Pill tone={option === "proceed_safer" ? "safer" : "proceed"} dot>
          {option === "proceed_safer" ? "Safer version" : "Proceeded"}
        </Pill>
      );
  }
}

export function OptionTag({ option, children }: { option: Option; children?: ReactNode }) {
  return (
    <Pill tone={optionTone[option]} dot>
      {children ?? optionShort[option]}
    </Pill>
  );
}

const riskTone: Record<RiskClass, Tone> = { read: "muted", low: "neutral", medium: "neutral", high: "safer", critical: "stop" };

export function RiskTag({ risk }: { risk: RiskClass }) {
  return <Pill tone={riskTone[risk]}>{riskLabel[risk]} risk</Pill>;
}

export function DecidedByText({ by, reviewer }: { by: DecidedBy | null; reviewer?: string | null }) {
  if (!by) return <span className="text-ink-3">–</span>;
  if (by === "human" && reviewer) return <span>{reviewer}</span>;
  return <span>{decidedByLabel[by]}</span>;
}

const outcomeTone: Record<OutcomeStatus, Tone> = { succeeded: "neutral", failed: "stop", reversed: "safer", complaint: "stop" };

export function OutcomeTag({ outcome }: { outcome: OutcomeStatus }) {
  return <Pill tone={outcomeTone[outcome]}>{outcomeLabel[outcome]}</Pill>;
}

/** A small live indicator dot that breathes while connected. */
export function LiveDot({ state }: { state: "live" | "connecting" | "offline" }) {
  return (
    <span className="relative inline-flex size-2">
      <span
        className={cx(
          "absolute inset-0 rounded-full",
          state === "live" ? "bg-proceed" : state === "connecting" ? "bg-safer" : "bg-ink-3",
          state !== "offline" && "animate-[veto-pulse_2.4s_ease-in-out_infinite]",
        )}
      />
    </span>
  );
}
