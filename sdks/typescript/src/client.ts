import { VetoError, VetoPendingError, VetoStoppedError } from "./errors.js";
import type {
  ActionResult,
  ExecuteResponse,
  Outcome,
  OutcomeStatus,
  PublicKeys,
  SubmitRequest,
  VerifyRequest,
  VerifyResult,
} from "./types.js";

export interface VetoOptions {
  /** An agent API key (veto_sk_...). Defaults to the VETO_API_KEY environment variable. */
  apiKey?: string;
  /** The Veto server, such as https://veto.example.com. Defaults to VETO_URL. */
  baseUrl?: string;
  /** Time limit for one HTTP request, excluding long-poll waits. Default 30 seconds. */
  timeoutMs?: number;
  /** How many times to retry a request that failed with a retryable error. Default 3. */
  maxRetries?: number;
  /** A fetch implementation, for runtimes without a global fetch or for tests. */
  fetch?: typeof fetch;
}

export interface WaitOptions {
  /** How long to wait for a decision, in milliseconds. */
  timeoutMs?: number;
  signal?: AbortSignal;
}

/** The longest single long-poll the server allows; longer waits poll repeatedly. */
const MAX_POLL_SECONDS = 25;

const env = (name: string): string | undefined => (typeof process !== "undefined" && process.env ? process.env[name] : undefined);

const sleep = (ms: number, signal?: AbortSignal) =>
  new Promise<void>((resolve, reject) => {
    if (signal?.aborted) return reject(signal.reason);
    const t = setTimeout(resolve, ms);
    signal?.addEventListener(
      "abort",
      () => {
        clearTimeout(t);
        reject(signal.reason);
      },
      { once: true },
    );
  });

const settled = (a: ActionResult) => a.status !== "pending_review" && a.status !== "evaluating";

/**
 * A client for Veto's agent API.
 *
 * ```ts
 * const veto = new Veto();
 * const result = await veto.guard("gmail.send", args, (tool, approvedArgs) => gmail.call(tool, approvedArgs), {
 *   request: userMessage,
 * });
 * ```
 */
export class Veto {
  private readonly apiKey: string;
  private readonly baseUrl: string;
  private readonly timeoutMs: number;
  private readonly maxRetries: number;
  private readonly fetchImpl: typeof fetch;

  constructor(options: VetoOptions = {}) {
    const apiKey = options.apiKey ?? env("VETO_API_KEY");
    const baseUrl = options.baseUrl ?? env("VETO_URL");
    if (!apiKey) throw new Error("Veto: pass apiKey or set VETO_API_KEY");
    if (!baseUrl) throw new Error("Veto: pass baseUrl or set VETO_URL");
    this.apiKey = apiKey;
    this.baseUrl = baseUrl.replace(/\/+$/, "");
    this.timeoutMs = options.timeoutMs ?? 30_000;
    this.maxRetries = options.maxRetries ?? 3;
    const f = options.fetch ?? globalThis.fetch;
    if (!f) throw new Error("Veto: no fetch implementation is available; pass one in options.fetch");
    this.fetchImpl = f.bind(globalThis);
  }

  readonly actions = {
    /**
     * Proposes a call. With waitSeconds, the server holds the request until
     * the action is decided or the wait ends, whichever comes first.
     */
    submit: <A>(req: SubmitRequest<A>, opts: { waitSeconds?: number; signal?: AbortSignal } = {}) =>
      this.request<ActionResult>("POST", `/v1/actions${waitQuery(opts.waitSeconds)}`, {
        body: { ...req, idempotency_key: req.idempotency_key ?? randomKey() },
        wait: opts.waitSeconds,
        signal: opts.signal,
      }),

    /** Reads an action's current result, optionally waiting for a decision. */
    get: (id: string, opts: { waitSeconds?: number; signal?: AbortSignal } = {}) =>
      this.request<ActionResult>("GET", `/v1/actions/${encodeURIComponent(id)}${waitQuery(opts.waitSeconds)}`, {
        wait: opts.waitSeconds,
        signal: opts.signal,
      }),

    /** Waits until the action is decided or the timeout passes, and returns its latest state. */
    waitForDecision: async (id: string, opts: WaitOptions = {}): Promise<ActionResult> => {
      const deadline = Date.now() + (opts.timeoutMs ?? 60_000);
      for (;;) {
        const left = Math.max(0, Math.floor((deadline - Date.now()) / 1000));
        const a = await this.actions.get(id, { waitSeconds: Math.min(left, MAX_POLL_SECONDS), signal: opts.signal });
        if (settled(a) || Date.now() >= deadline) return a;
      }
    },

    /** Reports how a call went. Bad outcomes queue the decision for audit. */
    reportOutcome: (id: string, status: OutcomeStatus, detail?: Record<string, unknown>) =>
      this.request<Outcome>("POST", `/v1/actions/${encodeURIComponent(id)}/outcome`, { body: { status, detail } }),

    /**
     * Has Veto run an approved call through one of its HTTP connections,
     * using the stored credential. Veto records the outcome itself.
     */
    execute: (id: string) => this.request<ExecuteResponse>("POST", `/v1/actions/${encodeURIComponent(id)}/execute`, { body: {} }),
  };

  readonly grants = {
    /**
     * Checks a grant before running a call. Tool servers use this with any
     * API key to refuse calls Veto did not approve.
     */
    verify: (req: VerifyRequest) => this.request<VerifyResult>("POST", "/v1/grants/verify", { body: req }),

    /** The workspace's public signing keys. */
    keys: () => this.request<PublicKeys>("GET", "/v1/grants/keys", {}),
  };

  /**
   * Asks Veto, waits for the decision, and runs the call only if it is
   * approved, using the tool and arguments Veto approved, which are the
   * safer version when that was chosen. Reports the outcome afterwards.
   *
   * Throws VetoStoppedError when the call must not run and
   * VetoPendingError when a person has not decided within waitMs.
   */
  async guard<A, R>(
    tool: string,
    args: A,
    run: (tool: string, args: A, grant: string | undefined) => Promise<R>,
    opts: Omit<SubmitRequest<A>, "tool" | "args"> & { waitMs?: number; signal?: AbortSignal } = {},
  ): Promise<R> {
    const { waitMs = 60_000, signal, ...rest } = opts;
    const started = Date.now();
    let action = await this.actions.submit({ ...rest, tool, args }, { waitSeconds: Math.min(MAX_POLL_SECONDS, waitMs / 1000), signal });
    if (!settled(action)) {
      action = await this.actions.waitForDecision(action.id, { timeoutMs: Math.max(0, waitMs - (Date.now() - started)), signal });
    }
    if (!settled(action)) throw new VetoPendingError(action);
    const d = action.decision;
    if (!d || !d.approved) throw new VetoStoppedError(action);
    const approvedTool = d.tool ?? tool;
    const approvedArgs = (d.args ?? args) as A;
    let result: R;
    try {
      result = await run(approvedTool, approvedArgs, action.grant?.token);
    } catch (err) {
      await this.actions
        .reportOutcome(action.id, "failed", { error: err instanceof Error ? err.message : String(err) })
        .catch(() => undefined);
      throw err;
    }
    await this.actions.reportOutcome(action.id, "succeeded").catch(() => undefined);
    return result;
  }

  private async request<T>(method: string, path: string, opts: { body?: unknown; wait?: number; signal?: AbortSignal }): Promise<T> {
    let attempt = 0;
    for (;;) {
      try {
        return await this.once<T>(method, path, opts);
      } catch (err) {
        const retry = err instanceof VetoError && err.retryable && attempt < this.maxRetries;
        if (!retry) throw err;
        attempt++;
        const after = (err as VetoError & { retryAfterMs?: number }).retryAfterMs;
        await sleep(after ?? Math.min(8000, 250 * 2 ** attempt) * (0.5 + Math.random() / 2), opts.signal);
      }
    }
  }

  private async once<T>(method: string, path: string, opts: { body?: unknown; wait?: number; signal?: AbortSignal }): Promise<T> {
    const timeout = this.timeoutMs + (opts.wait ?? 0) * 1000;
    const ctl = new AbortController();
    const timer = setTimeout(() => ctl.abort(new Error(`Veto: request timed out after ${timeout} ms`)), timeout);
    const onAbort = () => ctl.abort(opts.signal?.reason);
    opts.signal?.addEventListener("abort", onAbort, { once: true });
    let res: Response;
    try {
      res = await this.fetchImpl(this.baseUrl + path, {
        method,
        headers: {
          Authorization: `Bearer ${this.apiKey}`,
          Accept: "application/json",
          ...(opts.body !== undefined ? { "Content-Type": "application/json" } : {}),
          "User-Agent": "veto-sdk-typescript/1.0.0",
        },
        body: opts.body !== undefined ? JSON.stringify(opts.body) : undefined,
        signal: ctl.signal,
      });
    } catch (err) {
      if (opts.signal?.aborted) throw opts.signal.reason;
      throw new VetoError(0, "network", `Veto could not be reached: ${err instanceof Error ? err.message : String(err)}`);
    } finally {
      clearTimeout(timer);
      opts.signal?.removeEventListener("abort", onAbort);
    }
    const text = await res.text();
    let data: unknown;
    try {
      data = text ? JSON.parse(text) : undefined;
    } catch {
      throw new VetoError(res.status, "bad_response", `Veto answered ${res.status} with a body that is not JSON`);
    }
    if (!res.ok) {
      const body = data as { error?: { code?: string; message?: string; details?: unknown }; request_id?: string } | undefined;
      const e = new VetoError(
        res.status,
        body?.error?.code ?? "unknown",
        body?.error?.message ?? `Veto answered ${res.status}`,
        body?.error?.details,
        body?.request_id,
      );
      const ra = Number(res.headers.get("Retry-After"));
      if (Number.isFinite(ra) && ra > 0) Object.assign(e, { retryAfterMs: ra * 1000 });
      throw e;
    }
    return data as T;
  }
}

function waitQuery(seconds?: number): string {
  return seconds && seconds > 0 ? `?wait=${Math.floor(seconds)}` : "";
}

function randomKey(): string {
  const c = globalThis.crypto;
  if (c?.randomUUID) return c.randomUUID();
  return `${Date.now().toString(36)}-${Math.random().toString(36).slice(2)}${Math.random().toString(36).slice(2)}`;
}
