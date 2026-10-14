"use client";

import { useState, type ReactNode } from "react";
import { cx } from "@/lib/cx";
import { diffLines, stableStringify, tokenizeJson, type JsonToken } from "@/lib/json";
import { Icon } from "../icons";

const tokenClass: Record<JsonToken["t"], string> = {
  key: "text-ink",
  string: "text-proceed",
  number: "text-held",
  literal: "text-safer",
  punct: "text-ink-3",
  space: "",
};

function Tokens({ text }: { text: string }) {
  return (
    <>
      {tokenizeJson(text).map((tok, i) => (
        <span key={i} className={tokenClass[tok.t]}>
          {tok.v}
        </span>
      ))}
    </>
  );
}

/** Formatted, coloured JSON in a scrollable well. */
export function JsonView({
  value,
  className,
  maxHeight = 360,
  label,
}: {
  value: unknown;
  className?: string;
  maxHeight?: number;
  label?: string;
}) {
  const text = stableStringify(value);
  return (
    <div className={cx("group relative rounded-[12px] bg-sunken shadow-[inset_0_0_0_1px_var(--line)]", className)}>
      <div className="absolute top-2 right-2 opacity-0 transition-opacity group-hover:opacity-100 focus-within:opacity-100">
        <CopyButton value={text} label={label ? `Copy ${label}` : "Copy JSON"} />
      </div>
      <pre className="scroll-quiet overflow-auto px-4 py-3 text-xs leading-[1.7]" style={{ maxHeight }}>
        <Tokens text={text} />
      </pre>
    </div>
  );
}

/**
 * Shows how one call's arguments differ from another's: removed lines in the
 * stop colour, added lines in the safer colour.
 */
export function JsonDiff({ before, after, className }: { before: unknown; after: unknown; className?: string }) {
  const lines = diffLines(stableStringify(before), stableStringify(after));
  return (
    <pre
      className={cx(
        "scroll-quiet overflow-auto rounded-[12px] bg-sunken py-3 text-xs leading-[1.7] shadow-[inset_0_0_0_1px_var(--line)]",
        className,
      )}
    >
      {lines.map((l, i) => (
        <div key={i} className={cx("flex px-4", l.kind === "add" && "bg-safer-wash", l.kind === "del" && "bg-stop-wash")}>
          <span
            className={cx("mr-3 w-3 shrink-0 select-none", l.kind === "add" ? "text-safer" : l.kind === "del" ? "text-stop" : "text-ink-3")}
          >
            {l.kind === "add" ? "+" : l.kind === "del" ? "-" : " "}
          </span>
          <span className={cx(l.kind === "del" && "line-through decoration-stop/40")}>
            <Tokens text={l.text} />
          </span>
        </div>
      ))}
    </pre>
  );
}

export function CopyButton({ value, label = "Copy", className }: { value: string; label?: string; className?: string }) {
  const [copied, setCopied] = useState(false);
  return (
    <button
      type="button"
      aria-label={label}
      title={label}
      onClick={async () => {
        await navigator.clipboard.writeText(value);
        setCopied(true);
        window.setTimeout(() => setCopied(false), 1400);
      }}
      className={cx(
        "inline-flex size-7 items-center justify-center rounded-[8px] bg-surface text-ink-3 shadow-[inset_0_0_0_1px_var(--line)] hover:text-ink",
        className,
      )}
    >
      {copied ? <Icon.Check size={14} className="text-proceed" /> : <Icon.Copy size={14} />}
    </button>
  );
}

/** An identifier or hash in monospace, with a copy control on hover. */
export function Mono({ children, copy, className, title }: { children: ReactNode; copy?: string; className?: string; title?: string }) {
  return (
    <span className={cx("group/mono inline-flex max-w-full items-center gap-1", className)} title={title}>
      <code className="truncate text-[12px] text-ink-2">{children}</code>
      {copy && (
        <button
          type="button"
          aria-label="Copy"
          onClick={() => void navigator.clipboard.writeText(copy)}
          className="shrink-0 text-ink-3 opacity-0 transition-opacity group-hover/mono:opacity-100 hover:text-ink focus:opacity-100"
        >
          <Icon.Copy size={13} />
        </button>
      )}
    </span>
  );
}

/** A block of example code, such as a curl command, with a copy control. */
export function CodeBlock({ code, className }: { code: string; className?: string }) {
  return (
    <div className={cx("group relative rounded-[12px] bg-sunken shadow-[inset_0_0_0_1px_var(--line)]", className)}>
      <div className="absolute top-2 right-2">
        <CopyButton value={code} />
      </div>
      <pre className="scroll-quiet overflow-x-auto px-4 py-3 pr-12 text-xs leading-[1.7] text-ink">{code}</pre>
    </div>
  );
}
