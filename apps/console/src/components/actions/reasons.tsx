"use client";

import { humanize } from "@/lib/format";
import { useMeta } from "@/lib/queries";
import type { Reason } from "@/lib/types";

/** The label for a reason code, from the server's vocabulary. */
export function useReasonLabel() {
  const meta = useMeta();
  return (code: string) => {
    const label = meta.data?.reasons[code];
    return label ? label.charAt(0).toUpperCase() + label.slice(1) : humanize(code);
  };
}

/** Why an action was held or decided as it was, one reason per line. */
export function ReasonList({ reasons, className }: { reasons: Reason[] | null | undefined; className?: string }) {
  const label = useReasonLabel();
  if (!reasons?.length) return <span className="text-ink-3">–</span>;
  return (
    <ul className={className}>
      {reasons.map((r, i) => (
        <li key={i} className="line-clamp-3 text-sm break-words">
          <span className="text-ink">{label(r.code)}</span>
          {r.detail && <span className="text-ink-3">: {r.detail}</span>}
        </li>
      ))}
    </ul>
  );
}

/** The first reason as a short inline phrase, for table rows. */
export function ReasonInline({ reasons }: { reasons: Reason[] | null | undefined }) {
  const label = useReasonLabel();
  const r = reasons?.[0];
  if (!r) return <span className="text-ink-3">–</span>;
  return (
    <span className="text-ink-2" title={reasons!.map((x) => label(x.code) + (x.detail ? `: ${x.detail}` : "")).join("\n")}>
      {label(r.code)}
      {reasons!.length > 1 && <span className="text-ink-3"> and {reasons!.length - 1} more</span>}
    </span>
  );
}
