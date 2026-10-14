/** The three things Veto can decide for a proposed call. */
export type Option = "proceed" | "proceed_safer" | "stop";

export type ActionStatus = "evaluating" | "pending_review" | "approved" | "denied" | "expired";

export type DecidedBy = "model" | "human" | "policy" | "failsafe" | "expiry";

export type OutcomeStatus = "succeeded" | "failed" | "reversed" | "complaint";

export interface Reason {
  /** A stable code, such as low_confidence or policy_forbid. */
  code: string;
  detail?: string;
}

/** Who the agent is acting for. Policies can match on the id. */
export interface Principal {
  id: string;
  display?: string;
}

/** Text the agent read from a source it does not control, such as an email. */
export interface Untrusted {
  source: string;
  content: string;
}

export interface OptionSpec {
  key: Option;
  available: boolean;
  tool?: string;
  args?: unknown;
  changes?: string[];
}

export interface SubmitRequest<A = unknown> {
  /** The tool the agent wants to call, such as gmail.send. */
  tool: string;
  /** The arguments it wants to call it with. */
  args: A;
  /** What the agent was asked to do, in the words it was given. Improves decisions. */
  request?: string;
  principal?: Principal;
  untrusted?: Untrusted[];
  /** Extra facts for policies, as a JSON object. */
  context?: Record<string, unknown>;
  /**
   * Makes the submission safe to retry: the same key returns the same
   * action. The client sets a random one when you do not.
   */
  idempotency_key?: string;
}

export interface Decision {
  option: Option;
  /** True when something may run: the call as proposed or its safer version. */
  approved: boolean;
  /** The tool to run. For the safer version it can differ from the one proposed. */
  tool?: string;
  /** The arguments to run it with. Use these, not the ones you proposed. */
  args?: unknown;
  decided_by: DecidedBy;
  reasons: Reason[];
  confidence: number | null;
  note?: string;
}

export interface Grant {
  /** A signed, single-use token bound to the approved tool and arguments. */
  token: string;
  expires_at: string;
}

export interface ActionResult {
  id: string;
  status: ActionStatus;
  tool: string;
  decision: Decision | null;
  grant: Grant | null;
  hold_reasons: Reason[];
  options: OptionSpec[];
  /** Where a person can review the action in the console. */
  review_url?: string;
  review_deadline: string | null;
  received_at: string;
  decided_at: string | null;
}

export interface Outcome {
  id: string;
  action_id: string;
  status: OutcomeStatus;
  detail: unknown;
  reported_by: string;
  created_at: string;
}

/** What an HTTP connection returned when Veto ran the call. */
export interface ExecuteResponse {
  status: number;
  headers: Record<string, string>;
  body?: unknown;
  body_text?: string;
  truncated?: boolean;
  latency_ms: number;
}

export interface VerifyRequest {
  token: string;
  tool: string;
  args: unknown;
  /** Marks the grant used, so it cannot be presented again. */
  consume: boolean;
}

export interface VerifyResult {
  valid: boolean;
  grant_id: string;
  action_id: string;
  option: Option;
  expires_at: string;
  consumed: boolean;
}

export interface PublicKey {
  key_id: string;
  public_key: string;
  created_at: string;
  retired_at: string | null;
}

export interface PublicKeys {
  workspace_id: string;
  algorithm: "Ed25519";
  keys: PublicKey[];
}
