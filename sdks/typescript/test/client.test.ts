import { describe, expect, it } from "vitest";
import { Veto, VetoError, VetoPendingError, VetoStoppedError, type ActionResult } from "../src/index.js";

type Call = { method: string; url: string; body: unknown; headers: Record<string, string> };

/** A fetch that answers from a script and records what it was asked. */
function scripted(responses: Array<{ status: number; body?: unknown; headers?: Record<string, string> }>) {
  const calls: Call[] = [];
  const fn = (async (url: string, init: RequestInit) => {
    calls.push({
      method: init.method ?? "GET",
      url,
      body: init.body ? JSON.parse(init.body as string) : undefined,
      headers: init.headers as Record<string, string>,
    });
    const r = responses.shift();
    if (!r) throw new Error("no scripted response left");
    return new Response(r.body === undefined ? "" : JSON.stringify(r.body), { status: r.status, headers: r.headers });
  }) as unknown as typeof fetch;
  return { fn, calls };
}

const action = (over: Partial<ActionResult>): ActionResult => ({
  id: "act_1",
  status: "approved",
  tool: "gmail.send",
  decision: null,
  grant: null,
  hold_reasons: [],
  options: [],
  review_deadline: null,
  received_at: "2026-01-01T00:00:00Z",
  decided_at: null,
  ...over,
});

const client = (fn: typeof fetch) => new Veto({ apiKey: "veto_sk_test", baseUrl: "https://veto.test/", fetch: fn, maxRetries: 2 });

describe("Veto client", () => {
  it("sends the key and an idempotency key, and trims the base URL", async () => {
    const { fn, calls } = scripted([{ status: 200, body: action({}) }]);
    await client(fn).actions.submit({ tool: "gmail.send", args: { to: ["a@b.c"] } }, { waitSeconds: 10 });
    expect(calls[0]!.url).toBe("https://veto.test/v1/actions?wait=10");
    expect(calls[0]!.headers.Authorization).toBe("Bearer veto_sk_test");
    expect((calls[0]!.body as { idempotency_key: string }).idempotency_key).toMatch(/.{8,}/);
  });

  it("retries retryable errors with the same idempotency key", async () => {
    const { fn, calls } = scripted([
      { status: 503, body: { error: { code: "unavailable", message: "busy" } }, headers: { "Retry-After": "0" } },
      { status: 200, body: action({}) },
    ]);
    await client(fn).actions.submit({ tool: "t", args: {} });
    expect(calls).toHaveLength(2);
    expect((calls[0]!.body as { idempotency_key: string }).idempotency_key).toBe(
      (calls[1]!.body as { idempotency_key: string }).idempotency_key,
    );
  });

  it("does not retry client errors and exposes the API error", async () => {
    const { fn, calls } = scripted([
      { status: 400, body: { error: { code: "invalid_request", message: "tool is required" }, request_id: "req_1" } },
    ]);
    const err = await client(fn)
      .actions.submit({ tool: "", args: {} })
      .catch((e: unknown) => e);
    expect(err).toBeInstanceOf(VetoError);
    expect(err).toMatchObject({ status: 400, code: "invalid_request", requestId: "req_1", message: "tool is required" });
    expect(calls).toHaveLength(1);
  });
});

describe("guard", () => {
  it("runs the approved safer call and reports success", async () => {
    const { fn, calls } = scripted([
      {
        status: 200,
        body: action({
          decision: {
            option: "proceed_safer",
            approved: true,
            tool: "gmail.create_draft",
            args: { to: [] },
            decided_by: "model",
            reasons: [],
            confidence: 0.9,
          },
          grant: { token: "vg1.x.y", expires_at: "2026-01-01T00:05:00Z" },
        }),
      },
      { status: 201, body: {} },
    ]);
    const ran: unknown[] = [];
    const out = await client(fn).guard("gmail.send", { to: ["x@rival.io"] }, async (tool, args, grant) => {
      ran.push(tool, args, grant);
      return "sent";
    });
    expect(out).toBe("sent");
    expect(ran).toEqual(["gmail.create_draft", { to: [] }, "vg1.x.y"]);
    expect(calls[1]!.url).toBe("https://veto.test/v1/actions/act_1/outcome");
    expect(calls[1]!.body).toEqual({ status: "succeeded" });
  });

  it("throws VetoStoppedError and never runs a stopped call", async () => {
    const { fn } = scripted([
      {
        status: 200,
        body: action({
          status: "denied",
          decision: {
            option: "stop",
            approved: false,
            decided_by: "policy",
            reasons: [{ code: "policy_forbid", detail: "no-refunds" }],
            confidence: null,
          },
        }),
      },
    ]);
    let ran = false;
    const err = await client(fn)
      .guard("stripe.refund", {}, async () => {
        ran = true;
      })
      .catch((e: unknown) => e);
    expect(err).toBeInstanceOf(VetoStoppedError);
    expect((err as Error).message).toContain("policy_forbid (no-refunds)");
    expect(ran).toBe(false);
  });

  it("throws VetoPendingError when nobody decides in time", async () => {
    const pending = action({ status: "pending_review", review_url: "https://veto.test/actions/act_1" });
    const { fn } = scripted([
      { status: 202, body: pending },
      { status: 200, body: pending },
    ]);
    const err = await client(fn)
      .guard("github.merge", {}, async () => "merged", { waitMs: 0 })
      .catch((e: unknown) => e);
    expect(err).toBeInstanceOf(VetoPendingError);
    expect((err as VetoPendingError).action.id).toBe("act_1");
  });

  it("reports a failed outcome when the call throws", async () => {
    const { fn, calls } = scripted([
      {
        status: 200,
        body: action({
          decision: { option: "proceed", approved: true, tool: "t", args: {}, decided_by: "model", reasons: [], confidence: 0.95 },
        }),
      },
      { status: 201, body: {} },
    ]);
    await expect(
      client(fn).guard("t", {}, async () => {
        throw new Error("smtp down");
      }),
    ).rejects.toThrow("smtp down");
    expect(calls[1]!.body).toEqual({ status: "failed", detail: { error: "smtp down" } });
  });
});
