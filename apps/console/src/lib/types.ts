/**
 * Types for the Veto API, mirroring the JSON the server writes. Field names
 * stay in the API's snake_case so a response can be read against the API
 * reference without translation.
 */

export type RiskClass = "read" | "low" | "medium" | "high" | "critical";
export type Option = "proceed" | "proceed_safer" | "stop";
export type ActionStatus = "evaluating" | "pending_review" | "approved" | "denied" | "expired";
export type DecidedBy = "model" | "human" | "policy" | "failsafe" | "expiry";
export type Source = "api" | "mcp" | "connection";
export type OutcomeStatus = "succeeded" | "failed" | "reversed" | "complaint";
export type AuditVerdict = "correct" | "should_be_safer" | "should_hold" | "should_block";
export type FailMode = "closed" | "open";
export type Role = "owner" | "admin" | "reviewer" | "viewer";
export type Range = "1h" | "24h" | "7d" | "30d" | "90d";
export type QuestionType = "choice" | "score" | "noul";

export interface Thresholds {
  auto_confidence: number;
  risk_ceiling: number;
  block_confidence: number;
  double_check: boolean;
  auto_proceed: boolean;
}

export interface Reason {
  code: string;
  detail?: string;
}

export interface Principal {
  id: string;
  display: string;
}

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

export interface SaferRule {
  op: string;
  tool?: string;
  path?: string;
  max?: number | string;
  allow?: string[];
  value?: unknown;
}

export interface Tool {
  name: string;
  description: string;
  risk_class: RiskClass;
  reversible: boolean;
  pack: string;
  safer: SaferRule[] | null;
  auto_confidence: number | null;
  risk_ceiling: number | null;
  block_confidence: number | null;
  double_check: boolean | null;
  fail_mode: FailMode;
  max_per_hour: number | null;
  step_up: boolean;
  registered: boolean;
  created_at: string;
  updated_at: string;
}

export interface ToolStats extends Tool {
  calls_7d: number;
  held_7d: number;
  denied_7d: number;
  safer_taken_7d: number;
  last_called_at: string | null;
  avg_confidence_7d: number | null;
}

export interface ToolInput {
  description: string;
  risk_class: RiskClass;
  reversible: boolean;
  pack: string;
  safer: SaferRule[] | null;
  auto_confidence: number | null;
  risk_ceiling: number | null;
  block_confidence: number | null;
  double_check: boolean | null;
  fail_mode: FailMode;
  max_per_hour: number | null;
  step_up: boolean;
}

export interface User {
  id: string;
  email: string;
  name: string;
  created_at: string;
  last_login_at: string | null;
  disabled_at: string | null;
}

export interface Settings {
  review_timeout_seconds: number;
  audit_sample_rate: number;
  retention_days: number;
  untrusted_max_chars: number;
  calibration_target: number;
}

export interface Workspace {
  id: string;
  name: string;
  settings: Settings;
  active_policy_version: number | null;
  created_at: string;
}

export interface Me {
  user: User;
  role: Role;
  workspace: Workspace;
}

export interface SessionView {
  user: User;
  role: Role;
  workspace_id: string;
  expires_at: string;
}

export interface SetupState {
  needs_setup: boolean;
  setup_token_required: boolean;
}

export interface InvitationInfo {
  email: string;
  role: Role;
  workspace_name: string;
  expires_at: string;
  account_exists: boolean;
}

export interface Member extends User {
  role: Role;
  joined_at: string;
}

export interface Invitation {
  id: string;
  email: string;
  role: Role;
  created_by: string | null;
  created_at: string;
  expires_at: string;
  accepted_at: string | null;
  revoked_at: string | null;
}

export interface Agent {
  id: string;
  name: string;
  description: string;
  vendor: string;
  created_at: string;
  disabled_at: string | null;
}

export interface AgentStats extends Agent {
  actions_7d: number;
  held_7d: number;
  denied_7d: number;
  last_seen_at: string | null;
  active_keys: number;
  pending_now: number;
  auto_rate_7d: number | null;
  top_tools_7d: string[] | null;
}

export interface APIKey {
  id: string;
  agent_id: string | null;
  agent_name: string | null;
  name: string;
  kind: "agent" | "admin";
  prefix: string;
  created_by: string | null;
  created_at: string;
  last_used_at: string | null;
  revoked_at: string | null;
}

export interface IssuedKey extends APIKey {
  secret: string;
}

export interface AgentDetail extends AgentStats {
  keys: APIKey[];
  recent: ActionSummary[];
}

export interface ActionSummary {
  id: string;
  agent_id: string;
  agent_name: string;
  tool: string;
  principal_display: string;
  request: string;
  status: ActionStatus;
  source: Source;
  hold_reasons: Reason[];
  ranking: Option[];
  received_at: string;
  decided_at: string | null;
  review_deadline: string | null;
  option: Option | null;
  decided_by: DecidedBy | null;
  confidence: number | null;
  risk: number | null;
  reviewer_name: string | null;
  outcome: OutcomeStatus | null;
  decision_ms: number | null;
}

export interface ActionPage {
  items: ActionSummary[];
  next_cursor: string;
}

export interface Action {
  id: string;
  agent_id: string;
  agent_name: string;
  tool: string;
  args: unknown;
  principal: Principal;
  request: string;
  untrusted: Untrusted[];
  context: unknown;
  source: Source;
  idempotency_key: string | null;
  status: ActionStatus;
  options: OptionSpec[];
  ranking: Option[];
  hold_reasons: Reason[];
  policy_version: number | null;
  received_at: string;
  decided_at: string | null;
  review_deadline: string | null;
  redacted_at: string | null;
}

export interface Decision {
  action_id: string;
  option: Option;
  outcome: "approved" | "denied" | "expired";
  decided_by: DecidedBy;
  reviewer_id: string | null;
  reviewer_name: string | null;
  final_tool: string;
  final_args: unknown;
  confidence: number | null;
  risk: number | null;
  model_choice: Option | null;
  model_confidence: number | null;
  reasons: Reason[];
  thresholds: Thresholds;
  note: string;
  review_seconds: number | null;
  created_at: string;
}

export interface JevAnswer {
  type: QuestionType;
  choice?: string;
  score?: number;
  legend?: Record<string, string>;
  noul?: number;
  probabilities?: Record<string, number>;
  confidence?: number;
}

export interface SignalValue {
  type: QuestionType;
  value: number;
  risk: number;
  weight: number;
  choice?: string;
  confidence?: number;
  answer: JevAnswer;
  levels?: string[];
}

export interface TrippedGuard {
  signal: string;
  value: number;
  forbid: Option[];
  reason: string;
}

export interface Recheck {
  choice: Option;
  confidence: number;
  order: Option[];
  agrees: boolean;
}

export interface Assessment {
  pack: string;
  pack_version: number;
  choice: Option;
  confidence: number;
  probabilities: Partial<Record<Option, number>>;
  order: Option[];
  signals: Record<string, SignalValue>;
  risk: number;
  guards: TrippedGuard[];
  recheck?: Recheck;
  skipped?: string[];
}

export interface Evaluation {
  id: string;
  action_id: string;
  stage: "gate" | "model" | "model_recheck" | string;
  provider: string;
  model: string;
  request: unknown;
  response: unknown;
  result: unknown;
  latency_ms: number;
  input_tokens: number;
  error: string;
  created_at: string;
}

export interface GrantView {
  id: string;
  tool: string;
  args_hash: string;
  key_id: string;
  expires_at: string;
  consumed_at: string | null;
  created_at: string;
}

export interface Outcome {
  id: string;
  action_id: string;
  status: OutcomeStatus;
  detail: unknown;
  reported_by: string;
  created_at: string;
}

export interface Audit {
  id: string;
  action_id: string;
  reason: string;
  verdict: AuditVerdict | null;
  reviewer_id: string | null;
  reviewer_name: string | null;
  note: string;
  created_at: string;
  reviewed_at: string | null;
}

export interface AuditItem extends Audit {
  tool: string;
  agent_name: string;
  request: string;
  option: Option;
  decided_by: DecidedBy;
  confidence: number | null;
  risk: number | null;
  final_tool: string;
  final_args: unknown;
  received_at: string;
  outcome: OutcomeStatus | null;
}

export interface LedgerEntry {
  seq: number;
  kind: string;
  action_id?: string;
  actor: string;
  payload: unknown;
  created_at: string;
  prev_hash: string;
  hash: string;
}

export interface ActionDetail {
  action: Action;
  args_hash: string;
  agent: Agent;
  tool: Tool | null;
  decision: Decision | null;
  assessment: Assessment | null;
  evaluations: Evaluation[];
  grants: GrantView[];
  outcomes: Outcome[];
  audit: Audit | null;
  ledger: LedgerEntry[];
  review_url?: string;
}

export interface Percentiles {
  p50: number | null;
  p90: number | null;
  p95: number | null;
  p99: number | null;
}

export interface SeriesPoint {
  bucket: string;
  auto_approved: number;
  human_approved: number;
  denied: number;
  expired: number;
  pending: number;
  held: number;
}

export interface LatencyPoint {
  bucket: string;
  p50: number | null;
  p95: number | null;
  calls: number;
  errors: number;
}

export interface Ranked {
  key: string;
  label: string;
  count: number;
  held?: number;
}

export interface Flow {
  decided_by: DecidedBy | "pending";
  option: Option | "pending";
  count: number;
}

export interface Proportion {
  value: number;
  low: number;
  high: number;
  n: number;
}

export interface ModelStatus {
  configured: boolean;
  name: string;
}

export interface Overview {
  from: string;
  to: string;
  bucket_seconds: number;
  counts: {
    received: number;
    decided: number;
    held: number;
    by_model: number;
    by_policy: number;
    by_human: number;
    by_failsafe: number;
    by_expiry: number;
    approved: number;
    denied: number;
    expired: number;
    opt_proceed: number;
    opt_safer: number;
    opt_stop: number;
    safer_offered: number;
    outcome_failed: number;
    outcome_reversed: number;
    outcome_complaint: number;
    model_calls: number;
    model_errors: number;
    input_tokens: number;
    audits_reviewed: number;
    audits_wrong: number;
  };
  review_seconds: Percentiles;
  model_latency_ms: Percentiles;
  auto_decision_ms: Percentiles;
  series: SeriesPoint[];
  latency: LatencyPoint[];
  top_tools: Ranked[];
  top_agents: Ranked[];
  hold_reasons: Ranked[];
  risk_histogram: number[];
  flows: Flow[];
  live: {
    pending: number;
    oldest_pending_at: string | null;
    next_deadline: string | null;
    open_audits: number;
    unregistered_tools: number;
    active_agents_24h: number;
  };
  range: Range;
  rates: {
    auto_resolution: number | null;
    hold_rate: number | null;
    safer_take: number | null;
    safer_offered: number | null;
    false_approval: Proportion | null;
    model_error_rate: number | null;
    stop_rate: number | null;
  };
  model: ModelStatus;
}

export interface CalibrationBand {
  low: number;
  high: number;
  labeled: number;
  agreed: number;
  auto: number;
  held: number;
  agreement: Proportion | null;
}

export interface ToolCalibration {
  tool: string;
  risk_class: RiskClass;
  current: Thresholds;
  labeled: number;
  agreed: number;
  agreement: Proportion | null;
  bands: CalibrationBand[];
  suggested: number | null;
  note: string;
}

export interface Calibration {
  range: Range;
  target: number;
  min_n: number;
  overall: CalibrationBand[];
  tools: ToolCalibration[];
}

export interface PolicyVersion {
  version: number;
  source: string;
  note: string;
  created_by: string | null;
  created_at: string;
  activated_at: string | null;
  active: boolean;
}

export interface Diagnostic {
  policy_id?: string;
  line?: number;
  column?: number;
  message: string;
}

export interface PolicySummary {
  id: string;
  effect: "deny" | "allow" | "review";
  line: number;
}

export interface PolicyCheck {
  valid: boolean;
  diagnostics: Diagnostic[] | null;
  policies: PolicySummary[] | null;
}

export interface GateResult {
  outcome: "deny" | "review" | "allow" | "continue";
  policies: string[] | null;
  errors?: Diagnostic[];
}

export interface PackSignalChoice {
  key: string;
  description: string;
  risk: number;
}

export interface PackSignal {
  id: string;
  type: QuestionType;
  instructions: string;
  levels?: string[];
  choices?: PackSignalChoice[];
  yes?: string;
  no?: string;
  requires?: string[];
  risk?: { weight: number; invert?: boolean };
}

export interface PackGuard {
  signal: string;
  above?: number;
  below?: number;
  forbid: Option[];
  reason: string;
}

export interface Pack {
  name: string;
  version: number;
  title: string;
  description: string;
  disposition: {
    instructions: string;
    criteria: Partial<Record<Option, string>>;
  };
  signals: PackSignal[];
  guards: PackGuard[];
}

export interface PackInfo {
  name: string;
  title: string;
  builtin: boolean;
  overridden: boolean;
  version: number;
  definition: Pack | null;
  tools: string[] | null;
}

export interface PackVersion {
  name: string;
  version: number;
  definition: Pack;
  created_by: string | null;
  created_at: string;
  active: boolean;
}

export interface Verdict {
  route: "auto" | "human";
  option?: Option;
  decided_by?: DecidedBy;
  reasons: Reason[];
  ranking: Option[];
}

export interface SimulateRequest {
  agent_id: string;
  tool: string;
  args: unknown;
  request: string;
  untrusted: Untrusted[];
  principal: string;
  call_model: boolean;
}

export interface SimulateResult {
  tool: Tool;
  gate: GateResult;
  options: OptionSpec[];
  pack: string;
  model_request: unknown;
  assessment: Assessment | null;
  model_error?: string;
  verdict: Verdict;
  thresholds: Thresholds;
}

export interface Connection {
  id: string;
  name: string;
  description: string;
  base_url: string;
  auth_scheme: "bearer" | "header" | "basic" | "none" | string;
  header_name: string;
  has_secret: boolean;
  allow_methods: string[];
  allow_private: boolean;
  created_at: string;
  updated_at: string;
  rotated_at: string | null;
  disabled_at: string | null;
}

export interface ConnectionInput {
  name: string;
  description: string;
  base_url: string;
  auth_scheme: string;
  header_name: string;
  secret?: string;
  allow_methods: string[];
  allow_private: boolean;
  disabled: boolean;
}

export interface MCPServer {
  id: string;
  name: string;
  upstream_url: string;
  connection_id: string | null;
  created_at: string;
  updated_at: string;
  disabled_at: string | null;
}

export interface MCPServerInput {
  name: string;
  upstream_url: string;
  connection_id: string | null;
  disabled: boolean;
}

export interface Webhook {
  id: string;
  url: string;
  description: string;
  events: string[];
  created_at: string;
  disabled_at: string | null;
}

export interface CreatedWebhook extends Webhook {
  secret: string;
}

export interface WebhookInput {
  url: string;
  description: string;
  events: string[];
  disabled: boolean;
}

export interface Delivery {
  id: string;
  webhook_id: string;
  event: string;
  payload: unknown;
  status: "pending" | "delivered" | "failed" | string;
  attempts: number;
  response_status: number | null;
  last_error: string;
  next_attempt_at: string;
  created_at: string;
  delivered_at: string | null;
}

export interface SlackSettings {
  channel: string;
  mention_held: boolean;
}

export interface SlackStatus {
  configured: boolean;
  settings: SlackSettings;
  has_secrets: boolean;
  interaction_url: string;
}

export interface CheckpointRef {
  seq: number;
  hash: string;
  key_id: string;
  signature: string;
  valid: boolean;
  created_at: string;
}

export interface LedgerPage {
  entries: LedgerEntry[];
  checkpoints: CheckpointRef[];
  head_seq: number;
  next_before: number;
}

export interface VerifyReport {
  entries: number;
  head_seq: number;
  head_hash: string;
  break: { seq: number; reason: string } | null;
  checkpoint: CheckpointRef | null;
  verified_at: string;
  duration_ms: number;
}

export interface KeyView {
  key_id: string;
  public_key: string;
  created_at: string;
  retired_at: string | null;
}

export interface SystemStatus {
  version: string;
  schema_version: string;
  model: ModelStatus;
  public_url: string;
  slack_available: boolean;
  private_connections_allowed: boolean;
  decision_timeout_ms: number;
  server_time: string;
  signing_keys: KeyView[];
}

export interface Meta {
  version: string;
  risk_classes: { name: RiskClass; thresholds: Thresholds }[];
  reasons: Record<string, string>;
  options: Record<Option, string>;
  packs: string[];
  webhook_events: string[];
  safer_ops: string[];
  roles: Role[];
  model: ModelStatus;
}

/** A change pushed over the event stream. The console refetches what it shows. */
export interface LiveEvent {
  kind: string;
  action_id?: string;
  status?: string;
  subject?: string;
  at: string;
}

export interface Items<T> {
  items: T[];
}
