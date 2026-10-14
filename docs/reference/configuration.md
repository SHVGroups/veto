# Configuration

vetod is configured entirely with environment variables. It checks them
all at start and reports every problem at once.

## Required

| Variable            | Description                                                                                                                                                                            |
| ------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `VETO_DATABASE_URL` | PostgreSQL connection URL, such as `postgres://veto:secret@db:5432/veto?sslmode=require`. PostgreSQL 14 or newer.                                                                      |
| `VETO_MASTER_KEY`   | 32 random bytes, base64. Encrypts signing keys, connection secrets, webhook secrets and Slack credentials with AES-256-GCM. Generate with `vetod keygen` or `openssl rand -base64 32`. |

## Recommended

| Variable           | Default | Description                                                                                                                                                                                                     |
| ------------------ | ------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `VETO_PUBLIC_URL`  | none    | Where people open the console, such as `https://veto.example.com`. Used for review links in webhooks and Slack, the Slack interaction address, invitation links, and as an allowed origin for console requests. |
| `VETO_JEV_API_KEY` | none    | TypeSafe API key. Without it, every call policies do not decide waits for a person.                                                                                                                             |
| `VETO_SETUP_TOKEN` | none    | When set, first-run setup asks for it, so nobody else can claim a new deployment.                                                                                                                               |

## Decision model

| Variable                | Default                   | Description                                                                                                                                                                                                                                        |
| ----------------------- | ------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `VETO_JEV_BASE_URL`     | `https://api.typesafe.ai` | TypeSafe API host.                                                                                                                                                                                                                                 |
| `VETO_JEV_MODEL`        | `jev-1.13.0`              | Model to call. Pin a version; do not use an alias that can change under you.                                                                                                                                                                       |
| `VETO_JEV_FALLBACKS`    | none                      | JSON list of further endpoints tried in order when the primary fails with a server error or times out: `[{"name": "eu", "base_url": "https://...", "api_key": "...", "model": "jev-1.13.0"}]`. Client errors (400, 422) are not retried elsewhere. |
| `VETO_DECISION_TIMEOUT` | `2.5s`                    | Time limit for the model, including retries and fallbacks. A timeout counts as the model being unavailable.                                                                                                                                        |

## HTTP

| Variable                | Default                                 | Description                                                                                                                                                                                              |
| ----------------------- | --------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `VETO_LISTEN_ADDR`      | `:8080`, or `:$PORT` when `PORT` is set | Address to listen on.                                                                                                                                                                                    |
| `VETO_ALLOWED_ORIGINS`  | none                                    | Comma-separated extra origins allowed to make cookie-authenticated requests, besides `VETO_PUBLIC_URL`.                                                                                                  |
| `VETO_TRUSTED_PROXIES`  | none                                    | Comma-separated CIDRs whose `X-Forwarded-For` and `X-Forwarded-Host` headers are believed. Set it to your load balancer's and the console's networks, or per-client rate limits see the proxy's address. |
| `VETO_COOKIE_SECURE`    | `true`                                  | Marks the session cookie Secure. Set `false` only for plain-http local testing.                                                                                                                          |
| `VETO_SESSION_TTL`      | `336h`                                  | How long a console session lasts.                                                                                                                                                                        |
| `VETO_MAX_WAIT`         | `60s`                                   | Upper bound for `?wait=` on action requests. At most `5m`.                                                                                                                                               |
| `VETO_RATE_LIMIT_RPS`   | `50`                                    | Requests per second allowed per API key, per replica.                                                                                                                                                    |
| `VETO_RATE_LIMIT_BURST` | `100`                                   | Burst allowed per API key, per replica.                                                                                                                                                                  |
| `VETO_METRICS_TOKEN`    | none                                    | When set, `/metrics` requires `Authorization: Bearer <token>`.                                                                                                                                           |
| `VETO_MCP_HOLD_TIMEOUT` | `50s`                                   | How long the MCP proxy waits for a reviewer before answering that a call is held.                                                                                                                        |

## Security and secrets

| Variable                       | Default | Description                                                                                                                                                |
| ------------------------------ | ------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `VETO_MASTER_KEY_PREVIOUS`     | none    | Comma-separated previous master keys, still accepted for reading during a key rotation. See [Security](../operations/security.md#rotating-the-master-key). |
| `VETO_CONNECTOR_ALLOW_PRIVATE` | `false` | Lets connections, MCP servers and webhooks reach private, loopback and link-local addresses. Each connection must also opt in.                             |

## Operations

| Variable            | Default | Description                                                                                                                                                                                                              |
| ------------------- | ------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `VETO_WORKERS`      | `true`  | Run background jobs in this process: expiring held actions, reminders, webhook delivery, checkpoints, retention and ledger verification. Jobs coordinate through database locks, so any number of replicas can run them. |
| `VETO_AUTO_MIGRATE` | `true`  | Apply database migrations on start. Set `false` to run `vetod migrate` as a separate step.                                                                                                                               |
| `VETO_LOG_LEVEL`    | `info`  | `debug`, `info`, `warn` or `error`.                                                                                                                                                                                      |
| `VETO_LOG_FORMAT`   | `json`  | `json` or `text`.                                                                                                                                                                                                        |

## Workspace settings

These are set per workspace in the console under **Settings**, not in the
environment:

| Setting                 | Default           | Range              |
| ----------------------- | ----------------- | ------------------ |
| Time to review          | 1 hour            | 1 minute to 7 days |
| Audit sample            | 2%                | 0 to 100%          |
| Keep arguments for      | 90 days           | 1 to 3650 days     |
| Untrusted content limit | 12,000 characters | 0 to 100,000       |
| Calibration target      | 98%               | 50 to 100%         |

## The console

The console has one setting:

| Variable       | Description                                                                                                                                                                              |
| -------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `VETO_API_URL` | Where the console forwards `/v1` and `/mcp`, such as `http://vetod:8080`. Read on every request, so one image works everywhere. Defaults to `https://api.veto.localhost` in development. |

It listens on `PORT` (3000 in the image) and `HOSTNAME`.
