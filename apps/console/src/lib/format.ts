import type { ActionStatus, DecidedBy, Option, OutcomeStatus, RiskClass, Role } from "./types";

const integer = new Intl.NumberFormat("en-US");
const compact = new Intl.NumberFormat("en-US", { notation: "compact", maximumFractionDigits: 1 });

export function formatCount(n: number | null | undefined): string {
  if (n === null || n === undefined) return "–";
  return Math.abs(n) >= 100_000 ? compact.format(n) : integer.format(n);
}

/** A share in 0..1 as a percentage with sensible precision. */
export function formatPercent(v: number | null | undefined, digits?: number): string {
  if (v === null || v === undefined || Number.isNaN(v)) return "–";
  const pct = v * 100;
  const d = digits ?? (pct !== 0 && Math.abs(pct) < 10 ? 1 : 0);
  return `${pct.toFixed(d)}%`;
}

/** A probability or confidence in 0..1, shown to two decimals. */
export function formatProbability(v: number | null | undefined): string {
  if (v === null || v === undefined || Number.isNaN(v)) return "–";
  return v.toFixed(2);
}

export function formatMs(ms: number | null | undefined): string {
  if (ms === null || ms === undefined) return "–";
  if (ms < 1) return "<1 ms";
  if (ms < 1000) return `${Math.round(ms)} ms`;
  return `${(ms / 1000).toFixed(ms < 10_000 ? 2 : 1)} s`;
}

/** A duration in seconds, in the largest two units that matter. */
export function formatDuration(seconds: number | null | undefined): string {
  if (seconds === null || seconds === undefined || Number.isNaN(seconds)) return "–";
  const s = Math.max(0, Math.round(seconds));
  if (s < 60) return `${s}s`;
  const m = Math.floor(s / 60);
  if (m < 60) return s % 60 ? `${m}m ${s % 60}s` : `${m}m`;
  const h = Math.floor(m / 60);
  if (h < 24) return m % 60 ? `${h}h ${m % 60}m` : `${h}h`;
  const d = Math.floor(h / 24);
  return h % 24 ? `${d}d ${h % 24}h` : `${d}d`;
}

const rtf = new Intl.RelativeTimeFormat("en-US", { numeric: "auto" });

/** "3 min ago", "in 12 min", relative to now. */
export function formatRelative(iso: string | null | undefined, now: number = Date.now()): string {
  if (!iso) return "–";
  const diff = (new Date(iso).getTime() - now) / 1000;
  const abs = Math.abs(diff);
  if (abs < 45) return diff < 0 ? "just now" : "in a moment";
  if (abs < 3600) return rtf.format(Math.round(diff / 60), "minute");
  if (abs < 86_400) return rtf.format(Math.round(diff / 3600), "hour");
  if (abs < 86_400 * 30) return rtf.format(Math.round(diff / 86_400), "day");
  return formatDate(iso);
}

const dateFmt = new Intl.DateTimeFormat("en-US", { month: "short", day: "numeric", year: "numeric" });
const dateTimeFmt = new Intl.DateTimeFormat("en-US", {
  month: "short",
  day: "numeric",
  hour: "numeric",
  minute: "2-digit",
});
const fullFmt = new Intl.DateTimeFormat("en-US", {
  year: "numeric",
  month: "short",
  day: "numeric",
  hour: "numeric",
  minute: "2-digit",
  second: "2-digit",
  timeZoneName: "short",
});
const timeFmt = new Intl.DateTimeFormat("en-US", { hour: "numeric", minute: "2-digit" });

export function formatDate(iso: string | null | undefined): string {
  return iso ? dateFmt.format(new Date(iso)) : "–";
}

export function formatDateTime(iso: string | null | undefined): string {
  return iso ? dateTimeFmt.format(new Date(iso)) : "–";
}

export function formatTimestamp(iso: string | null | undefined): string {
  return iso ? fullFmt.format(new Date(iso)) : "–";
}

export function formatTime(iso: string | null | undefined): string {
  return iso ? timeFmt.format(new Date(iso)) : "–";
}

/** Shortens a long identifier or hash for display: abcd…wxyz. */
export function shortHash(h: string | null | undefined, keep = 8): string {
  if (!h) return "–";
  return h.length <= keep * 2 + 1 ? h : `${h.slice(0, keep)}…${h.slice(-4)}`;
}

/** Shortens a prefixed id (act_01h…) to its prefix and the last characters. */
export function shortId(id: string): string {
  const i = id.indexOf("_");
  if (i < 0 || id.length < i + 10) return id;
  return `${id.slice(0, i + 1)}…${id.slice(-6)}`;
}

export const optionLabel: Record<Option, string> = {
  proceed: "Run as proposed",
  proceed_safer: "Run the safer version",
  stop: "Stop",
};

export const optionShort: Record<Option, string> = {
  proceed: "Proceed",
  proceed_safer: "Safer",
  stop: "Stop",
};

export const optionLetter: Record<Option, string> = {
  proceed: "A",
  proceed_safer: "B",
  stop: "C",
};

/** Each option as the object of a sentence, such as "rules out running it as proposed". */
export const optionPhrase: Record<Option, string> = {
  proceed: "running it as proposed",
  proceed_safer: "the safer version",
  stop: "stopping it",
};

export const statusLabel: Record<ActionStatus, string> = {
  evaluating: "Evaluating",
  pending_review: "Waiting for review",
  approved: "Approved",
  denied: "Stopped",
  expired: "Expired",
};

export const decidedByLabel: Record<DecidedBy | "pending", string> = {
  model: "Decision model",
  human: "Reviewer",
  policy: "Policy",
  failsafe: "Failsafe",
  expiry: "Review expired",
  pending: "Waiting",
};

export const outcomeLabel: Record<OutcomeStatus, string> = {
  succeeded: "Succeeded",
  failed: "Failed",
  reversed: "Reversed",
  complaint: "Complaint",
};

export const riskLabel: Record<RiskClass, string> = {
  read: "Read",
  low: "Low",
  medium: "Medium",
  high: "High",
  critical: "Critical",
};

export const roleLabel: Record<Role, string> = {
  owner: "Owner",
  admin: "Admin",
  reviewer: "Reviewer",
  viewer: "Viewer",
};

export const roleDescription: Record<Role, string> = {
  owner: "Everything, including ownership and data deletion.",
  admin: "Configures agents, tools, policies and integrations, and manages members.",
  reviewer: "Decides held actions and records audit verdicts.",
  viewer: "Reads everything; changes nothing.",
};

export const auditVerdictLabel: Record<string, string> = {
  correct: "Correct",
  should_be_safer: "Should have been the safer version",
  should_hold: "Should have waited for a person",
  should_block: "Should have been stopped",
};

/** Turns a snake_case code into a sentence-case phrase as a last resort. */
export function humanize(code: string): string {
  const s = code.replace(/[_.]+/g, " ").trim();
  return s.charAt(0).toUpperCase() + s.slice(1);
}

export function pluralize(n: number, one: string, many = `${one}s`): string {
  return `${formatCount(n)} ${n === 1 ? one : many}`;
}

const kindLabels: Record<string, string> = {
  "action.received": "Received",
  "gate.evaluated": "Policy checked",
  "model.evaluated": "Model assessed",
  "action.held": "Held for review",
  "action.decided": "Decided",
  "action.expired": "Review expired",
  "grant.issued": "Grant issued",
  "grant.consumed": "Grant used",
  "action.executed": "Executed by Veto",
  "outcome.reported": "Outcome reported",
  "audit.sampled": "Sampled for audit",
  "audit.recorded": "Audit labelled",
  "policy.created": "Policy version saved",
  "policy.activated": "Policy version activated",
  "pack.updated": "Question pack changed",
  "tool.updated": "Tool changed",
  "tool.deleted": "Tool deleted",
  "agent.created": "Agent added",
  "agent.updated": "Agent changed",
  "key.created": "API key created",
  "key.revoked": "API key revoked",
  "connection.changed": "Connection changed",
  "mcp_server.changed": "MCP server changed",
  "webhook.changed": "Webhook changed",
  "integration.changed": "Integration changed",
  "member.changed": "Member changed",
  "settings.updated": "Settings changed",
  "session.created": "Signed in",
  "workspace.created": "Workspace created",
  "signing_key.rotated": "Signing key rotated",
};

/** A ledger entry kind as a short phrase. */
export function ledgerKindLabel(kind: string): string {
  return kindLabels[kind] ?? humanize(kind);
}
