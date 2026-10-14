import { cx } from "@/lib/cx";
import { formatProbability, optionLetter, optionShort } from "@/lib/format";
import type { Option } from "@/lib/types";

const ORDER: Option[] = ["proceed", "proceed_safer", "stop"];

const fill: Record<Option, string> = {
  proceed: "var(--proceed)",
  proceed_safer: "var(--safer)",
  stop: "var(--stop)",
};

/**
 * The three options and how likely the decision model found each one: a
 * single bar split by probability, the chosen option drawn at full strength.
 * Options that were not offered for this call are listed but take no space.
 */
export function Triad({
  probabilities,
  choice,
  offered,
  className,
  animate = true,
}: {
  probabilities: Partial<Record<Option, number>>;
  choice?: Option | null;
  offered?: Option[];
  className?: string;
  animate?: boolean;
}) {
  const present = ORDER.filter((o) => (offered ? offered.includes(o) : probabilities[o] !== undefined));
  const total = present.reduce((s, o) => s + (probabilities[o] ?? 0), 0) || 1;
  return (
    <div className={className}>
      <div
        className={cx(
          "flex h-3 w-full origin-left gap-[3px] overflow-hidden rounded-full",
          animate && "animate-[veto-fill_520ms_cubic-bezier(.2,.7,.2,1)]",
        )}
        role="img"
        aria-label={present.map((o) => `${optionShort[o]} ${formatProbability(probabilities[o] ?? 0)}`).join(", ")}
      >
        {present.map((o) => {
          const p = (probabilities[o] ?? 0) / total;
          return (
            <div
              key={o}
              className="h-full first:rounded-l-full last:rounded-r-full"
              style={{
                flexGrow: Math.max(p, 0.004),
                flexBasis: 0,
                background: fill[o],
                opacity: choice && choice !== o ? 0.28 : 1,
              }}
            />
          );
        })}
      </div>
      <div className="mt-3 grid grid-cols-3 gap-3">
        {ORDER.map((o) => {
          const isOffered = present.includes(o);
          const chosen = choice === o;
          return (
            <div key={o} className={cx("min-w-0", !isOffered && "opacity-45")}>
              <div className="flex items-center gap-1.5 text-xs text-ink-3">
                <span
                  className="inline-flex size-4 items-center justify-center rounded-[5px] text-[10px] font-semibold text-white"
                  style={{ background: isOffered ? fill[o] : "var(--ink-3)" }}
                >
                  {optionLetter[o]}
                </span>
                <span className="truncate">{optionShort[o]}</span>
              </div>
              <div className={cx("numeral mt-0.5 text-xl", chosen ? "text-ink" : "text-ink-3")}>
                {isOffered ? formatProbability(probabilities[o] ?? 0) : "–"}
              </div>
              {!isOffered && <div className="text-2xs text-ink-3">Not offered</div>}
              {chosen && isOffered && <div className="text-2xs text-ink-2">Model&rsquo;s pick</div>}
            </div>
          );
        })}
      </div>
    </div>
  );
}

/** A small inline version for tables: the bar only. */
export function TriadMini({
  probabilities,
  choice,
  className,
}: {
  probabilities: Partial<Record<Option, number>>;
  choice?: Option | null;
  className?: string;
}) {
  const present = ORDER.filter((o) => probabilities[o] !== undefined);
  const total = present.reduce((s, o) => s + (probabilities[o] ?? 0), 0) || 1;
  return (
    <div className={cx("flex h-1.5 w-16 gap-px overflow-hidden rounded-full", className)} aria-hidden="true">
      {present.map((o) => (
        <div
          key={o}
          style={{
            flexGrow: (probabilities[o] ?? 0) / total,
            flexBasis: 0,
            background: fill[o],
            opacity: choice && choice !== o ? 0.3 : 1,
          }}
        />
      ))}
    </div>
  );
}

/**
 * A 0 to 1 scale with a filled value and labelled thresholds, used for
 * confidence against the auto threshold and risk against the ceiling.
 */
export function Gauge({
  value,
  marks,
  tone = "ink",
  label,
  className,
}: {
  value: number | null;
  marks: { at: number; label: string }[];
  tone?: "ink" | "stop" | "safer" | "proceed";
  label: string;
  className?: string;
}) {
  const color = { ink: "var(--ink)", stop: "var(--stop)", safer: "var(--safer)", proceed: "var(--proceed)" }[tone];
  const v = value === null ? 0 : Math.min(1, Math.max(0, value));
  return (
    <div className={className}>
      <div className="flex items-baseline justify-between">
        <span className="text-sm text-ink-3">{label}</span>
        <span className="numeral text-lg text-ink">{value === null ? "–" : formatProbability(value)}</span>
      </div>
      <div
        className="relative mt-2 h-2 rounded-full bg-hover"
        role="meter"
        aria-valuemin={0}
        aria-valuemax={1}
        aria-valuenow={v}
        aria-label={label}
      >
        <div className="absolute inset-y-0 left-0 rounded-full" style={{ width: `${v * 100}%`, background: color }} />
        {marks.map((m) => (
          <div
            key={m.label}
            className="absolute -top-1 -bottom-1 w-[2px] rounded-full bg-ink"
            style={{ left: `calc(${Math.min(1, m.at) * 100}% - 1px)` }}
          />
        ))}
      </div>
      <div className="relative mt-1.5 h-4 text-2xs text-ink-3">
        {marks.map((m) => (
          <span
            key={m.label}
            className="figures absolute whitespace-nowrap"
            style={m.at > 0.7 ? { right: `${(1 - Math.min(1, m.at)) * 100}%` } : { left: `${m.at * 100}%` }}
          >
            {m.label} {formatProbability(m.at)}
          </span>
        ))}
      </div>
    </div>
  );
}
