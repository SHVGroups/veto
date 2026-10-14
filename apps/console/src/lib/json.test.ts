import { describe, expect, it } from "vitest";
import { diffLines, parseJsonInput, stableStringify, tokenizeJson } from "./json";

describe("stableStringify", () => {
  it("sorts keys at every depth", () => {
    expect(stableStringify({ b: 1, a: { d: 2, c: 3 } }, 0)).toBe('{"a":{"c":3,"d":2},"b":1}');
  });
});

describe("diffLines", () => {
  it("marks changed lines", () => {
    const d = diffLines(stableStringify({ to: ["a@x.io", "b@acme.com"] }), stableStringify({ to: ["b@acme.com"] }));
    expect(d.filter((l) => l.kind === "del").map((l) => l.text.trim())).toEqual(['"a@x.io",']);
    expect(d.some((l) => l.kind === "add")).toBe(false);
  });
});

describe("tokenizeJson", () => {
  it("separates keys from string values", () => {
    const kinds = tokenizeJson('{"a": "b", "n": 2, "t": true}')
      .filter((t) => t.t !== "space")
      .map((t) => t.t);
    expect(kinds).toEqual([
      "punct",
      "key",
      "punct",
      "string",
      "punct",
      "key",
      "punct",
      "number",
      "punct",
      "key",
      "punct",
      "literal",
      "punct",
    ]);
  });
});

describe("parseJsonInput", () => {
  it("reports errors instead of throwing", () => {
    expect(parseJsonInput("{").ok).toBe(false);
    expect(parseJsonInput('{"a":1}')).toEqual({ ok: true, value: { a: 1 } });
  });
});
