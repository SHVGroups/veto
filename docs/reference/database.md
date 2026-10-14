# Database

Veto stores everything in PostgreSQL 14 or newer. A deployment holds the
workspace created at first-run setup. Every row carries its workspace id,
and every query filters by it.

## Migrations

Migrations are SQL files embedded in the binary
(`server/internal/store/migrations`). `vetod serve` applies pending ones at
start unless `VETO_AUTO_MIGRATE=false`; `vetod migrate` applies them and
exits. A PostgreSQL advisory lock makes concurrent runs safe, so every
replica can start at once. Applied versions are recorded in
`schema_migrations`.

## Tables

| Table                                                  | Holds                                                                                                                                     |
| ------------------------------------------------------ | ----------------------------------------------------------------------------------------------------------------------------------------- |
| `workspaces`                                           | Name, settings and active policy version.                                                                                                 |
| `users`, `memberships`, `invitations`, `sessions`      | People, their roles per workspace, pending invitations, and console sessions (token hashes only).                                         |
| `agents`, `api_keys`                                   | Agents and their keys (SHA-256 hashes and display prefixes).                                                                              |
| `tools`                                                | Tool profiles: risk class, pack, rules, thresholds, limits.                                                                               |
| `policy_versions`                                      | Every saved version of the Cedar source.                                                                                                  |
| `question_packs`                                       | Workspace versions of question packs.                                                                                                     |
| `actions`                                              | Every proposed call: arguments, request, principal, untrusted content, options, ranking, hold reasons, status, deadline.                  |
| `evaluations`                                          | Each policy and model evaluation, with the exact model request and response, latency and token count.                                     |
| `decisions`                                            | One per decided action: option, who decided, reviewer, the approved call, confidence, risk, the model's choice, reasons, thresholds used. |
| `grants`                                               | Issued grants and when each was consumed.                                                                                                 |
| `outcomes`                                             | Outcome reports.                                                                                                                          |
| `audits`                                               | Decisions queued for audit and their verdicts.                                                                                            |
| `signing_keys`                                         | Ed25519 keys per workspace; private keys encrypted with the master key.                                                                   |
| `ledger_entries`, `ledger_heads`, `ledger_checkpoints` | The hash chain, its head per workspace, and signed checkpoints.                                                                           |
| `connections`                                          | HTTP connections; secrets encrypted with the master key.                                                                                  |
| `mcp_servers`                                          | Proxied MCP servers.                                                                                                                      |
| `webhooks`, `webhook_deliveries`                       | Endpoints (secrets encrypted) and the delivery queue with attempts and results.                                                           |
| `integrations`, `slack_messages`                       | Slack settings and credentials (encrypted), and the messages posted for held actions.                                                     |

## Encryption at rest

Secrets that Veto must read back (signing keys, connection secrets, webhook
signing secrets, Slack credentials) are encrypted with AES-256-GCM under the
master key, with the row they belong to as associated data, so a ciphertext
copied to another row does not decrypt. Credentials Veto only needs to
check (passwords, API keys, session tokens) are stored as hashes:
argon2id for passwords, SHA-256 for random tokens.

Use your platform's disk encryption as well; arguments and request text
are stored in plain form until retention removes them.

## Size and growth

Each action writes one row to `actions`, one to `decisions`, one or more
to `evaluations` (model requests and responses dominate, typically a few
kilobytes) and several to `ledger_entries`. Retention clears the large
fields after the workspace's retention period; ledger rows are kept.

Indexes cover the console's queries: actions by time, status, agent and
tool; pending actions by deadline; deliveries due for sending.

## Direct access

The schema is internal and may change between versions; use the API for
integrations. Read-only SQL for analysis is fine. Never update
`ledger_entries` by hand: verification will report it.
