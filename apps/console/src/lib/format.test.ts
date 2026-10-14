import { describe, expect, it } from "vitest";
import { formatDuration, formatMs, formatPercent, formatRelative, shortHash, shortId } from "./format";

describe("formatPercent", () => {
  it("adds a decimal only for small shares", () => {
    expect(formatPercent(0.5)).toBe("50%");
    expect(formatPercent(0.0425)).toBe("4.3%");
    expect(formatPercent(0)).toBe("0%");
    expect(formatPercent(null)).toBe("–");
  });
});

describe("formatDuration", () => {
  it("uses the two largest units", () => {
    expect(formatDuration(42)).toBe("42s");
    expect(formatDuration(125)).toBe("2m 5s");
    expect(formatDuration(3600)).toBe("1h");
    expect(formatDuration(90061)).toBe("1d 1h");
  });
});

describe("formatMs", () => {
  it("switches to seconds above one second", () => {
    expect(formatMs(85.3)).toBe("85 ms");
    expect(formatMs(1534)).toBe("1.53 s");
  });
});

describe("formatRelative", () => {
  it("describes past and future times", () => {
    const now = Date.parse("2026-01-01T12:00:00Z");
    expect(formatRelative("2026-01-01T11:59:50Z", now)).toBe("just now");
    expect(formatRelative("2026-01-01T11:55:00Z", now)).toBe("5 minutes ago");
    expect(formatRelative("2026-01-01T14:00:00Z", now)).toBe("in 2 hours");
  });
});

describe("identifiers", () => {
  it("shortens hashes and ids", () => {
    expect(shortHash("0123456789abcdef0123456789abcdef")).toBe("01234567…cdef");
    expect(shortId("act_01m4vtszfwnerf4qm6anjfjjn1")).toBe("act_…jfjjn1");
  });
});
