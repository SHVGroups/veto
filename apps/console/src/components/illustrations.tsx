/**
 * Line illustrations for empty and first-run states. Drawn in the interface's
 * ink colours so they sit quietly on any surface, in light or dark.
 */

const common = {
  fill: "none",
  stroke: "currentColor",
  strokeWidth: 1.5,
  strokeLinecap: "round" as const,
  strokeLinejoin: "round" as const,
};

/** Three options at rest on a line: nothing is waiting for a decision. */
export function QueueClear({ className }: { className?: string }) {
  return (
    <svg viewBox="0 0 160 96" className={className} aria-hidden="true">
      <g {...common} className="text-line-strong">
        <path d="M12 74h136" />
        <path d="M24 82h112" strokeDasharray="2 5" />
      </g>
      <circle cx="52" cy="60" r="14" className="fill-proceed-wash stroke-proceed" strokeWidth="1.5" />
      <circle cx="80" cy="56" r="18" className="fill-safer-wash stroke-safer" strokeWidth="1.5" />
      <circle cx="108" cy="60" r="14" className="fill-stop-wash stroke-stop" strokeWidth="1.5" />
      <path d="m73 56 5 5 9-10" {...common} className="text-ink" strokeWidth="2" />
    </svg>
  );
}

/** An empty table: rows waiting to be filled. */
export function EmptyRows({ className }: { className?: string }) {
  return (
    <svg viewBox="0 0 160 96" className={className} aria-hidden="true">
      <rect x="24" y="14" width="112" height="68" rx="12" className="fill-surface stroke-line-strong" strokeWidth="1.5" />
      <g {...common} className="text-line-strong">
        <path d="M24 34h112" />
        <path d="M38 48h30M38 60h48M38 72h22" />
        <path d="M104 48h18M96 60h26M110 72h12" />
      </g>
      <circle cx="38" cy="24" r="3" className="fill-line-strong" />
      <circle cx="48" cy="24" r="3" className="fill-line-strong" />
    </svg>
  );
}

/** Two plugs not yet joined. */
export function Unplugged({ className }: { className?: string }) {
  return (
    <svg viewBox="0 0 160 96" className={className} aria-hidden="true">
      <g {...common} className="text-ink-3">
        <path d="M14 48h34" />
        <rect x="48" y="36" width="22" height="24" rx="6" />
        <path d="M70 42h8M70 54h8" />
        <path d="M146 48h-34" />
        <rect x="90" y="36" width="22" height="24" rx="6" />
      </g>
      <path d="M80 30v-8M80 74v-8M72 33l-4-6M88 33l4-6" {...common} className="text-held" />
    </svg>
  );
}

/** A chain of linked records: the ledger. */
export function Chain({ className }: { className?: string }) {
  return (
    <svg viewBox="0 0 160 96" className={className} aria-hidden="true">
      <g {...common} className="text-line-strong">
        <rect x="14" y="34" width="36" height="28" rx="8" />
        <rect x="62" y="34" width="36" height="28" rx="8" />
        <rect x="110" y="34" width="36" height="28" rx="8" />
        <path d="M50 48h12M98 48h12" />
      </g>
      <g {...common} className="text-ink-2">
        <path d="M24 44h16M24 52h10M72 44h16M72 52h10M120 44h16M120 52h10" />
      </g>
      <circle cx="128" cy="22" r="8" className="fill-proceed-wash stroke-proceed" strokeWidth="1.5" />
      <path d="m124.5 22 2.5 2.5 4.5-5" {...common} className="text-proceed" />
    </svg>
  );
}

/** A magnifier over a document: nothing matched. */
export function NothingFound({ className }: { className?: string }) {
  return (
    <svg viewBox="0 0 160 96" className={className} aria-hidden="true">
      <rect x="40" y="12" width="60" height="74" rx="10" className="fill-surface stroke-line-strong" strokeWidth="1.5" />
      <g {...common} className="text-line-strong">
        <path d="M52 30h36M52 42h28M52 54h32" />
      </g>
      <circle cx="98" cy="58" r="16" className="fill-canvas stroke-ink-2" strokeWidth="1.5" />
      <path d="m110 70 14 14" {...common} className="text-ink-2" strokeWidth="2" />
    </svg>
  );
}

/** A labelled pair: a person and a model looking at the same thing. */
export function Calibrating({ className }: { className?: string }) {
  return (
    <svg viewBox="0 0 160 96" className={className} aria-hidden="true">
      <g {...common} className="text-line-strong">
        <path d="M22 80h116M22 80V16" />
        <path d="M22 80 138 16" strokeDasharray="3 5" />
      </g>
      {[
        [40, 70],
        [62, 58],
        [84, 47],
        [106, 33],
        [126, 24],
      ].map(([x, y]) => (
        <circle key={x} cx={x} cy={y} r="4" className="fill-surface stroke-ink" strokeWidth="1.5" />
      ))}
    </svg>
  );
}
