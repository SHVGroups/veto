/** JSON helpers for showing arguments and comparing the options' calls. */

/** Pretty JSON with object keys sorted, so equal values print identically. */
export function stableStringify(value: unknown, indent = 2): string {
  return JSON.stringify(sortKeys(value), null, indent) ?? "null";
}

function sortKeys(v: unknown): unknown {
  if (Array.isArray(v)) return v.map(sortKeys);
  if (v && typeof v === "object") {
    return Object.fromEntries(
      Object.keys(v as Record<string, unknown>)
        .sort()
        .map((k) => [k, sortKeys((v as Record<string, unknown>)[k])]),
    );
  }
  return v;
}

export type DiffLine = { kind: "same" | "add" | "del"; text: string };

/**
 * A line diff of two texts using the longest common subsequence. Inputs are
 * argument objects of a single call, so quadratic cost is not a concern;
 * very large inputs fall back to showing both sides whole.
 */
export function diffLines(a: string, b: string): DiffLine[] {
  const x = a.split("\n");
  const y = b.split("\n");
  if (x.length * y.length > 250_000) {
    return [...x.map((text) => ({ kind: "del" as const, text })), ...y.map((text) => ({ kind: "add" as const, text }))];
  }
  const n = x.length;
  const m = y.length;
  const lcs: number[][] = Array.from({ length: n + 1 }, () => new Array<number>(m + 1).fill(0));
  for (let i = n - 1; i >= 0; i--) {
    for (let j = m - 1; j >= 0; j--) {
      lcs[i]![j] = x[i] === y[j] ? lcs[i + 1]![j + 1]! + 1 : Math.max(lcs[i + 1]![j]!, lcs[i]![j + 1]!);
    }
  }
  const out: DiffLine[] = [];
  let i = 0;
  let j = 0;
  while (i < n && j < m) {
    if (x[i] === y[j]) {
      out.push({ kind: "same", text: x[i]! });
      i++;
      j++;
    } else if (lcs[i + 1]![j]! >= lcs[i]![j + 1]!) {
      out.push({ kind: "del", text: x[i++]! });
    } else {
      out.push({ kind: "add", text: y[j++]! });
    }
  }
  while (i < n) out.push({ kind: "del", text: x[i++]! });
  while (j < m) out.push({ kind: "add", text: y[j++]! });
  return out;
}

export type JsonToken = { t: "key" | "string" | "number" | "literal" | "punct" | "space"; v: string };

const tokenRe = /("(?:[^"\\]|\\.)*")(\s*:)?|(-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?)|(true|false|null)|([{}[\],:])|(\s+)/g;

/** Splits JSON text into tokens for syntax colouring. */
export function tokenizeJson(text: string): JsonToken[] {
  const out: JsonToken[] = [];
  let last = 0;
  for (const m of text.matchAll(tokenRe)) {
    if (m.index! > last) out.push({ t: "punct", v: text.slice(last, m.index) });
    if (m[1] !== undefined) {
      out.push({ t: m[2] ? "key" : "string", v: m[1] });
      if (m[2]) out.push({ t: "punct", v: m[2] });
    } else if (m[3] !== undefined) out.push({ t: "number", v: m[3] });
    else if (m[4] !== undefined) out.push({ t: "literal", v: m[4] });
    else if (m[5] !== undefined) out.push({ t: "punct", v: m[5] });
    else out.push({ t: "space", v: m[0] });
    last = m.index! + m[0].length;
  }
  if (last < text.length) out.push({ t: "punct", v: text.slice(last) });
  return out;
}

/** Parses JSON typed by a person, returning a message instead of throwing. */
export function parseJsonInput(text: string): { ok: true; value: unknown } | { ok: false; message: string } {
  if (!text.trim()) return { ok: false, message: "Enter a JSON value." };
  try {
    return { ok: true, value: JSON.parse(text) };
  } catch (err) {
    return { ok: false, message: err instanceof Error ? err.message : "This is not valid JSON." };
  }
}
