import { describe, expect, it } from "vitest";
import { can } from "./permissions";

describe("can", () => {
  it("matches the server's role ceilings", () => {
    expect(can("viewer", "view")).toBe(true);
    expect(can("viewer", "review")).toBe(false);
    expect(can("reviewer", "review")).toBe(true);
    expect(can("reviewer", "configure")).toBe(false);
    expect(can("admin", "manage_members")).toBe(true);
    expect(can("admin", "own")).toBe(false);
    expect(can("owner", "own")).toBe(true);
    expect(can(undefined, "view")).toBe(false);
  });
});
