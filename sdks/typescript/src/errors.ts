import type { ActionResult, Reason } from "./types.js";

/** An error response from the Veto API, or a failure to reach it (status 0). */
export class VetoError extends Error {
  readonly status: number;
  readonly code: string;
  readonly details: unknown;
  readonly requestId: string | undefined;

  constructor(status: number, code: string, message: string, details?: unknown, requestId?: string) {
    super(message);
    this.name = "VetoError";
    this.status = status;
    this.code = code;
    this.details = details;
    this.requestId = requestId;
  }

  /** Whether trying the same request again may succeed. */
  get retryable(): boolean {
    return this.status === 0 || this.status === 429 || this.status >= 500;
  }
}

/** Veto decided the action must not run. */
export class VetoStoppedError extends Error {
  readonly action: ActionResult;

  constructor(action: ActionResult) {
    const reasons = (action.decision?.reasons ?? action.hold_reasons).map(describe).join("; ");
    super(`Veto stopped ${action.tool}${reasons ? `: ${reasons}` : ""}`);
    this.name = "VetoStoppedError";
    this.action = action;
  }

  get reasons(): Reason[] {
    return this.action.decision?.reasons ?? [];
  }
}

/** The action is still waiting for a person after the time the caller allowed. */
export class VetoPendingError extends Error {
  readonly action: ActionResult;

  constructor(action: ActionResult) {
    super(
      `${action.tool} is waiting for review${action.review_url ? ` at ${action.review_url}` : ""}; check again later with actions.get("${action.id}")`,
    );
    this.name = "VetoPendingError";
    this.action = action;
  }
}

function describe(r: Reason): string {
  return r.detail ? `${r.code} (${r.detail})` : r.code;
}
