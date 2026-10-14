# Changelog

All notable changes to Veto are recorded here. Veto follows
[semantic versioning](https://semver.org); see
[Upgrading](docs/operations/upgrading.md) for what that covers.

## 1.0.0 - 2026-10-14

The first release.

### Decisions

- Every proposed tool call is decided as one of three options: run it as
  proposed, run a safer version, or stop it.
- Cedar policies run first and can stop a call, send it to a person or run
  it without the model. New workspaces start with policies for read-only,
  critical and unclassified tools.
- Safer versions are built in code from per-tool rules: switch to another
  tool, cap a number, keep only allowed email domains, set or remove an
  argument, and limit a list.
- A typed decision model (Jev on TypeSafe System One) answers each tool's
  question pack with calibrated probabilities. Confidence, composite risk
  and guards are computed from the answers; no text is generated.
- Per-tool thresholds by risk class, an optional double check with the
  options reversed, hourly limits, step-up for sensitive tools, and fail
  open or closed when the model is unavailable.
- Six built-in question packs: general, communication, payment,
  code change, data write and file share. Packs can be edited per
  workspace, with history.
- Untrusted content the agent read is passed to the model as data, with a
  signal for calls that follow instructions hidden in it.

### Agents

- The HTTP API with long-poll waits and idempotent submission, described
  by an OpenAPI 3.1 document.
- TypeScript (`@shvgroups/veto`) and Python (`shvgroups-veto`) SDKs with a `guard`
  helper, retries and sync and async clients.
- An MCP proxy for Streamable HTTP servers: agents point at Veto instead of
  the server, without code changes.
- Connections: Veto holds API credentials and runs approved calls itself.
- Ed25519-signed, single-use grants bound to the exact tool and arguments,
  verifiable online or offline.
- Outcome reports, with bad outcomes queued for audit.

### People

- The console: an overview with decision flow, volume, model latency and
  error charts; the review queue with keyboard use; a searchable action
  explorer with CSV export and a complete record of each action; audits;
  calibration with suggested thresholds; and pages for agents, tools,
  policies (with a Cedar editor that checks as you type), question packs,
  a playground, connections, MCP servers, webhooks, Slack, the ledger,
  members and settings.
- Live updates across every page through a server-sent event stream.
- Roles: viewer, reviewer, admin and owner. Invitations by link.
- Slack: held actions posted with a button per option, decided in place
  and updated when decided elsewhere.
- Signed webhooks for held, decided and expired actions, outcomes, audits,
  new tools and ledger verification failures, retried for about 11 hours.

### Record and operations

- A per-workspace hash-chained ledger of every decision and change, with
  signed checkpoints every 15 minutes and daily verification.
- Secrets sealed with AES-256-GCM under an operator-held master key, with
  online key rotation.
- Retention that removes arguments and model exchanges after a set period
  while keeping decisions and hashes.
- One `vetod` binary: API, MCP proxy, background jobs and operator
  commands (`migrate`, `keygen`, `vault rekey`, `ledger verify`, `eval`).
- PostgreSQL as the only dependency; any number of replicas.
- Prometheus metrics and alerting rules, structured logs, health and
  readiness endpoints.
- Docker images, Docker Compose, and a Helm chart with optional dedicated
  worker pods and a ServiceMonitor.
- `vetod eval` for scoring question packs against labelled cases, with
  cases for each built-in pack.
