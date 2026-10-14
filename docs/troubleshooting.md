# Troubleshooting

Start with the symptom. Each entry gives the likely causes in the order
worth checking, and how to confirm and fix each one.

Two things help with almost everything:

- **The request id.** Every response carries `X-Request-Id`, and every
  error body has `request_id`. Search vetod's logs for it.
- **The action's page.** In the console, **Actions**, then the action:
  the reasons it was held or stopped, every evaluation with the exact model
  request and response, and the timeline from the ledger. Paste an action
  id into the command menu (`⌘K` or `Ctrl K`) to jump to it.

Contents:

- [vetod does not start](#vetod-does-not-start)
- [The console](#the-console)
- [Agents and the API](#agents-and-the-api)
- [Too many actions wait for a person](#too-many-actions-wait-for-a-person)
- [The decision model](#the-decision-model)
- [Grants](#grants)
- [The MCP proxy](#the-mcp-proxy)
- [Connections](#connections)
- [Webhooks](#webhooks)
- [Slack](#slack)
- [The ledger](#the-ledger)
- [Policies and question packs](#policies-and-question-packs)
- [Local development](#local-development)

## vetod does not start

vetod checks its whole configuration before doing anything and prints every
problem at once, then exits. Read the first lines of its output.

| Message                                                                          | Fix                                                                                                                               |
| -------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------- |
| `VETO_DATABASE_URL is required`                                                  | Set it to a `postgres://` URL.                                                                                                    |
| `VETO_MASTER_KEY is required; generate one with: vetod keygen`                   | Generate a key and set it. Keep a copy: see [Backup and restore](operations/backup-and-restore.md).                               |
| `VETO_MASTER_KEY: vault: master key must be base64` or `must be 32 bytes, got N` | The value was truncated or wrapped. A key is 44 characters ending in `=`. Quote it in `.env` files if your tooling splits on `=`. |
| `VETO_PUBLIC_URL: "..." is not an absolute http(s) URL`                          | Include the scheme: `https://veto.example.com`.                                                                                   |
| `VETO_TRUSTED_PROXIES: invalid CIDR`                                             | Use CIDR form, such as `10.0.0.0/8`. A single address is `203.0.113.7/32`.                                                        |
| `VETO_MAX_WAIT must be at most 5m`                                               | Long polls longer than five minutes are not supported; agents poll again instead.                                                 |
| `VETO_JEV_FALLBACKS: ...`                                                        | The value must be a JSON list; each entry needs `base_url` and `model`.                                                           |
| `... invalid duration`                                                           | Durations need a unit: `2.5s`, `90s`, `336h`.                                                                                     |

**It cannot reach the database.** The error names the host. Check the URL,
that the database accepts connections from vetod's network, and
`sslmode`: managed databases usually need `sslmode=require`, a local
container usually needs `sslmode=disable`.

**A migration fails.** The log names the migration file and PostgreSQL's
error. Common causes are a role without `CREATE` rights on the schema (run
`vetod migrate` with an owner role and set `VETO_AUTO_MIGRATE=false`), or
PostgreSQL older than 14. A failed migration is rolled back, so fix the
cause and start again.

**The container restarts in a loop although vetod logs `vetod listening`.**
The health check is failing. `vetod healthcheck` probes the port from
`VETO_LISTEN_ADDR` (or `PORT`); if you changed the listen address, make
sure the orchestrator's probes use the same port.

## The console

**Sign-in seems to work, then you are back at the sign-in page.**
The browser did not keep the session cookie.

- On plain `http`, set `VETO_COOKIE_SECURE=false`. Browsers drop Secure
  cookies over `http` (except on `localhost`). Never do this in a real
  deployment; put TLS in front instead.
- Behind a proxy that rewrites the host, make sure it passes `Host`
  unchanged or sets `X-Forwarded-Host`, and that its address is in
  `VETO_TRUSTED_PROXIES`.

**"request origin is not allowed".** A changing request came from a page
whose origin Veto does not recognise. It accepts the origin of
`VETO_PUBLIC_URL` and the request's own host. Set `VETO_PUBLIC_URL` to the
exact address in the browser bar (scheme, host and port), or list extra
origins in `VETO_ALLOWED_ORIGINS`. Note that setting
`VETO_ALLOWED_ORIGINS` replaces the defaults, so include the public URL in
it too.

**"missing X-Veto-Requested-With header".** Something other than the
console sent a cookie-authenticated request that changes data. Scripts
should use an admin API key (`veto_ak_...`) with `Authorization: Bearer`,
which needs no such header.

**"setup token is missing or incorrect".** `VETO_SETUP_TOKEN` is set on
the server; enter the same value on the setup page. It is only asked for
until the first workspace exists.

**"setup has already been completed".** A workspace already exists. Sign
in, or ask an owner for an invitation. To start over on a test
deployment, drop and recreate the database.

**The sidebar does not show Live, or pages stop updating.** The event
stream (`/v1/events`, Server-Sent Events) is not getting through.

- Your proxy is buffering it. Turn buffering off for `/v1/events`: with
  nginx, `proxy_buffering off;`; most cloud load balancers pass streams
  through by default.
- Your proxy closes idle connections quickly. Veto sends a heartbeat every
  25 seconds to keep the stream open; set the idle timeout above a minute.
- The console reconnects on its own. If it reconnects every few seconds,
  look for the proxy timing it out.

Pages still work without the stream; they refresh every 15 to 60 seconds
instead of at once.

**"session expired; sign in again"** after a while is expected: sessions
last `VETO_SESSION_TTL` (14 days by default). Changing your password signs
out your other sessions.

**A page shows "your role does not allow this".** The action needs a
higher role. The table in [The console](guides/console.md#roles) lists
what each role can do; an admin or owner can change roles under
**Members**.

## Agents and the API

| Status and message                                                  | Cause and fix                                                                                                                     |
| ------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------- |
| `401` `send an API key as: Authorization: Bearer <key>`             | No key was sent. Check `VETO_API_KEY` in the agent's environment.                                                                 |
| `401` `Authorization header must be: Bearer <api key>`              | The header is malformed: one space after `Bearer`, no quotes.                                                                     |
| `401` `API key is invalid or revoked`                               | Wrong key, a revoked key, or a key from another deployment. Issue a new one under **Agents**.                                     |
| `403` `this endpoint needs an agent API key (veto_sk_...)`          | Submitting actions needs an agent key, not an admin key or a session.                                                             |
| `400` `tool must be 1 to 200 characters of ...`                     | Tool names allow letters, digits and `_ . : / -`.                                                                                 |
| `400` `args must be a JSON object`                                  | Send arguments as an object, not an array or a string of JSON.                                                                    |
| `400` `args must be at most ... bytes`                              | Arguments are limited to 256 KiB. Send a reference (a file id, a URL) instead of the content.                                     |
| `409` `idempotency key "..." was already used for a different call` | The same key was reused with different arguments. Use a new key per distinct call; the SDKs generate one per call.                |
| `429` `rate limit exceeded; slow down`                              | The key exceeded `VETO_RATE_LIMIT_RPS`. Retry after `Retry-After`. If it is normal load, raise the limit; it applies per replica. |
| `502`                                                               | A call through a connection failed upstream. The message has the upstream's status.                                               |
| `500` `something went wrong; the error has been logged`             | An unexpected error, most often the database. Find the `request_id` in vetod's logs, and check `/readyz`.                         |

**The response is `202` and `status` is `pending_review`.** The action is
waiting for a person, which is working as designed. Poll
`GET /v1/actions/{id}?wait=25` until it is decided, or let the SDK's
`guard` wait (`waitMs`, 60 s by default) and handle `VetoPendingError`
(`VetoPending` in Python). To find out why it was held, see the next
section.

**The agent waited and got `expired`.** Nobody decided before the
workspace's review timeout (one hour by default). See
[Too many actions wait for a person](#too-many-actions-wait-for-a-person),
and check that reviewers get notified: [Slack](guides/slack.md) or a
[webhook](guides/webhooks.md) on `action.pending_review`.

**`?wait=` returns before the action is decided.** It waits at most
`VETO_MAX_WAIT` (60 s by default), and a proxy in between may close the
request sooner. Raise the proxy's read timeout above `VETO_MAX_WAIT`, or
use a shorter wait and poll again.

**The agent ran the original call although Veto chose the safer version.**
The agent must run `decision.tool` with `decision.args` from the response,
not the call it proposed. `guard` in both SDKs does this. A tool that
verifies grants will refuse the original call, because the grant covers
only the approved arguments.

**Outcome reports fail with "outcomes can only be reported for approved
actions".** Only approved actions ran, so only they have outcomes. For a
stopped or expired action there is nothing to report.

## Too many actions wait for a person

Open **Overview** and look at **Why actions were held**; then open a few of
the held actions. The reason codes tell you which rule held them:

| Reason                                                                        | Meaning                                                                             | What to do                                                                                                         |
| ----------------------------------------------------------------------------- | ----------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------ |
| `unregistered_tool` / `policy_review` from `unclassified-tools-need-a-person` | The tool has never been classified.                                                 | Open **Tools**, give it a risk class and a question pack. This is the most common cause on a new deployment.       |
| `policy_review`                                                               | A policy with `@veto("review")` matched.                                            | Check the policy named in the reason.                                                                              |
| `critical_tool`                                                               | The tool's risk class is critical, which always needs a person.                     | Lower the class if that is wrong.                                                                                  |
| `model_not_configured`                                                        | `VETO_JEV_API_KEY` is not set.                                                      | Set it. Until then only policies decide.                                                                           |
| `model_unavailable`                                                           | The model failed and the tool fails closed.                                         | See [The decision model](#the-decision-model).                                                                     |
| `low_confidence`                                                              | The model's confidence was below the tool's threshold.                              | See **Calibration** for the suggested threshold, send `request` with every call, and improve the pack's questions. |
| `risk_above_ceiling`                                                          | The composite risk was above the tool's ceiling.                                    | Check which signals drove the risk on the action's page.                                                           |
| `guard`                                                                       | A pack guard ruled out running the call, for example a likely injected instruction. | Usually correct. If not, the guard's signal or limit needs work.                                                   |
| `recheck_disagreed`                                                           | Asking with the options in reverse order gave a different answer.                   | The decision is genuinely close; a person should decide it.                                                        |
| `safer_unavailable`                                                           | The model chose the safer version, but no safer-version rule applies to the call.   | Add rules on the tool's page so option B exists.                                                                   |
| `rate_limited`                                                                | The tool's hourly limit was reached.                                                | Raise `max_per_hour` on the tool if the volume is expected.                                                        |

Agents that send no `request` get markedly less confident decisions, since
the most important question, whether the call matches what the user asked,
cannot be answered. See [Good practice](guides/agents.md#good-practice).

## The decision model

**Every model decision says `model_not_configured`.** `VETO_JEV_API_KEY` is
not set on the replica that handled the call. **Settings** shows whether the
model is configured. In Kubernetes, check that the secret has the
`jev-api-key` key and that the pods restarted after you added it.

**Decisions say `model_unavailable`.** Open the action and its evaluation:
the error is recorded there, and in the logs as
`decision model unavailable`.

| Error contains                     | Cause                                                                                                                                                                                                                              |
| ---------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `returned 401` or `returned 403`   | The TypeSafe key is wrong, revoked or lacks access to the model.                                                                                                                                                                   |
| `returned 404`                     | `VETO_JEV_MODEL` names a model that does not exist, or `VETO_JEV_BASE_URL` is wrong.                                                                                                                                               |
| `returned 400` or `returned 422`   | TypeSafe rejected the request itself. This usually means a question pack produced a question it cannot accept. The evaluation shows the exact request; check the pack's signals. Such errors are not retried or sent to fallbacks. |
| `returned 429` or `529`            | Rate limited or overloaded at TypeSafe. Retried with backoff, then fallbacks. Ask TypeSafe for more capacity or add fallbacks.                                                                                                     |
| `returned 5xx`                     | Server error at TypeSafe. Retried, then fallbacks.                                                                                                                                                                                 |
| `context deadline exceeded`        | The model took longer than `VETO_DECISION_TIMEOUT` (2.5 s).                                                                                                                                                                        |
| `dial tcp`, `no such host`, `tls:` | vetod cannot reach TypeSafe. Check egress rules, DNS and any outbound proxy.                                                                                                                                                       |

Tools that fail open run unjudged while the model is down, recorded with
the reason `fail_open`; high and critical tools always fail closed.

**Decisions are slow.** Compare `veto_model_request_seconds` with
`veto_decision_seconds`. If the model accounts for the time, check
TypeSafe's status and your region; packs with fewer signals are faster,
since each signal is a question. If the model is fast and decisions are
slow, the database is the suspect: check its latency and connection pool.

**Confidence is consistently low for one tool.** Read a few evaluations on
the action pages. Common causes: the pack does not fit the tool (an email
tool on `general` instead of `communication`), agents do not send
`request`, or the tool's description is missing, so the model cannot tell
what the call does. Then follow [Calibration and audits](guides/calibration.md).

## Grants

`POST /v1/grants/verify` returns why a grant was refused:

| Message or reason                                    | Cause                                                                                                                                                                                                                                                            |
| ---------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `grant token is malformed`                           | The token was truncated or altered. Pass it exactly as received; it starts with `vg1.`.                                                                                                                                                                          |
| `mismatch`                                           | The tool or arguments differ from what was approved. The agent must run `decision.tool` with `decision.args`. Arguments are compared after canonical JSON encoding, so key order and whitespace do not matter, but `1` and `1.0` and `"1"` are different values. |
| `expired`                                            | Grants last five minutes from approval. Run approved calls promptly, or submit again.                                                                                                                                                                            |
| `grant was already used, or this action already ran` | A grant authorises one run. A retry after a successful run is refused on purpose.                                                                                                                                                                                |
| `grant was signed by an unknown key`                 | The grant came from another deployment, or a database was restored to before the key existed.                                                                                                                                                                    |
| `grant was not issued by this workspace`             | The tool verified with an API key from a different workspace.                                                                                                                                                                                                    |
| `grants are verified with an API key`                | Verification needs an agent or admin key, not a console session.                                                                                                                                                                                                 |

For offline verification, fetch the public keys from `GET /v1/grants/keys`
and cache them; refresh the cache when you see an unknown key id, which
happens after an owner rotates the signing key.

## The MCP proxy

**The agent gets `MCP server "..." not found`.** The name in the URL
(`/mcp/<name>`) does not match a server under **MCP servers**, or the
server is disabled. Names are lowercase.

**The agent gets `the MCP proxy needs an agent API key`.** The client sent
an admin key or no key. Use the agent's `veto_sk_` key.

**The agent's MCP client says it cannot connect.** Check that the client
sends `Authorization: Bearer <agent key>`, that it uses the Streamable
HTTP transport (stdio servers cannot be proxied), and that the upstream
answers: the log shows `mcp upstream request failed` with the reason. An
upstream on a private, loopback or link-local address needs
`VETO_CONNECTOR_ALLOW_PRIVATE=true` on the server.

**Every call comes back as "Veto is holding this call for human review".**
The tool, `mcp.<server>.<tool>`, has not been classified, so the default
policy holds it. Classify it under **Tools**. Retrying the same call in the
same session after a reviewer approves runs it.

**"Veto rejected the call: ..."** The call itself was invalid, for example
arguments that are not an object. The rest of the message says why.

**Batched requests fail.** A JSON-RPC batch containing `tools/call` is
rejected. Configure the client to send requests one at a time.

## Connections

**"this server does not allow connections to private addresses (set
VETO_CONNECTOR_ALLOW_PRIVATE)".** The connection's host resolves to a
private, loopback or link-local address. This protects internal services
from being reached through Veto. If the target is meant to be internal, set
`VETO_CONNECTOR_ALLOW_PRIVATE=true` on the server and enable private
addresses on that connection.

**"connection ... is disabled".** Enable it under **Connections**.

**`502` from `POST /v1/actions/{id}/execute`.** The API behind the
connection failed; the message includes its status. Redirects are not
followed, so a `301` or `302` from the API means the base URL needs
updating (often `http` to `https`, or a trailing path).

**"only http.<connection> tools are executed by Veto".** `execute` runs
connection tools only. Other tools are run by the agent with the grant.

## Webhooks

**Deliveries fail.** Open **Webhooks** and the endpoint: each delivery shows
its attempts, the response status and the last error.

- No status, only a connection error: the endpoint is unreachable from
  vetod, or resolves to a private address (allowed only with
  `VETO_CONNECTOR_ALLOW_PRIVATE=true`).
- A `3xx` status: redirects are not followed. Use the final URL.
- A timeout: the endpoint must answer within 20 seconds. Acknowledge at
  once and do the work afterwards.

Failed deliveries are retried after 30 seconds, 2, 10 and 30 minutes, then
1, 3 and 6 hours, eight attempts in all. **Send a test event** sends a
`webhook.test` event at once.

**Your receiver rejects the signature.**

- Compute the HMAC over the raw request body bytes, before any JSON
  parsing. Re-serialising changes the bytes.
- The signed message is `<t>.<body>`, with `t` taken from the
  `Veto-Signature` header.
- Use the secret shown when the webhook was created or last rotated. After
  **Rotate signing secret**, update the receiver.
- Reject old timestamps with a tolerance of a few minutes, and make sure
  the receiver's clock is synchronised.

The guide has a working example: [Verifying the signature](guides/webhooks.md#verifying-the-signature).

**The same event arrives twice.** Delivery is at least once: if your
endpoint processed a delivery but the response was lost, it is retried.
Deduplicate on the `Veto-Delivery` header.

**"webhook secret cannot be opened with the current master key".** The
master key changed without a rotation. Restore the old key as
`VETO_MASTER_KEY_PREVIOUS`, or rotate the webhook's secret.

## Slack

**Nothing is posted when an action is held.**

1. **Slack** in the console must show the integration as configured, with
   a channel id (such as `C0123456789`), not a channel name.
2. The app must be in the channel: `/invite @Veto`. Otherwise Slack
   answers `not_in_channel`, which vetod logs as
   `could not post held action to Slack`.
3. Search the logs for that message; Slack's error code is at the end:
   `invalid_auth` or `token_revoked` means the bot token is no longer
   valid (reinstall the app and save the new token); `channel_not_found`
   means a wrong id or a private channel the app has not joined.

**Clicking a button shows an error in Slack, or nothing happens.**

- Slack must reach `VETO_PUBLIC_URL` over HTTPS from the internet. A
  deployment on a private network or on `localhost` cannot receive clicks;
  reviewers can still use the link in the message.
- The **Interactivity Request URL** in the Slack app must be exactly the one
  shown on Veto's Slack page, including the workspace id. If you changed
  `VETO_PUBLIC_URL`, update it there.
- `missing Slack signature` or `Slack signature does not match` in the
  logs: the signing secret in Veto is not the app's. Copy it again from
  **Basic Information** in the Slack app.
- `Slack request timestamp outside tolerance`: vetod's clock is more than
  five minutes off. Fix time synchronisation on the host.

**The click is refused with a private message.**

| Message                                                                    | Fix                                                                                                                              |
| -------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------- |
| `Veto could not read your Slack email address, ...`                        | The app lacks the `users:read.email` scope. Add it (the manifest includes it) and reinstall the app.                             |
| `No active Veto account uses <email>.`                                     | The reviewer's Slack email differs from their Veto email, or their Veto account is disabled. Invite them with their Slack email. |
| `<email> is not a member of this Veto workspace.`                          | Invite them to the workspace.                                                                                                    |
| `your role does not allow this`                                            | Viewers cannot decide. Give them the reviewer role.                                                                              |
| `this tool needs a reviewer to confirm with their password in the console` | The tool requires step-up. Use the link to decide in the console.                                                                |
| `action ... was already decided`                                           | Someone decided it first, in Slack or the console. The message updates shortly.                                                  |

**Messages are not updated after a decision.** vetod logs
`could not update Slack message` with Slack's error. A `message_not_found`
means the message was deleted; nothing else is affected.

## The ledger

**Verification fails.** **Ledger > Verify the chain**, `vetod ledger
verify`, and the daily job all report the first entry that does not match
and why:

| Reason                                          | Meaning                                                                                   |
| ----------------------------------------------- | ----------------------------------------------------------------------------------------- |
| `entry N is missing`                            | A ledger row was deleted.                                                                 |
| `previous hash does not match the prior entry`  | A row was inserted, reordered or replaced.                                                |
| `contents do not match the recorded hash`       | A row's contents were changed after it was written.                                       |
| `chain head does not match the last entry`      | The head pointer and the last entry disagree, usually after a partial restore.            |
| `signed checkpoint no longer matches the chain` | The chain was rewritten consistently, but the latest signed checkpoint does not match it. |

What to do:

1. Treat it as a security incident until you know otherwise. The ledger is
   written only by vetod, inside the transactions that make each change, so
   a break means someone changed the database directly, or a restore mixed
   data from different points in time.
2. Keep a copy of the database as it is now.
3. Compare the reported entry with your exported checkpoints, webhook
   history, or a backup from before the break to see what changed.
4. If the cause is a botched restore, restore again from a single
   consistent backup and verify.

Veto keeps running while the ledger is broken; new entries keep chaining
from the current head. It does not repair the chain, since that would hide
the evidence.

**Verification is slow.** It reads every entry. On large ledgers the
console's **Verify the chain** can take a while; the daily job and
`vetod ledger verify` are the better tools there.

## Policies and question packs

**A policy will not save.** The editor lists each error with its line and
column. Two Veto-specific rules catch people out: every rule needs an
`@id("...")`, and every `permit` needs `@veto("allow")` or
`@veto("review")`, while a `forbid` must have neither. See
[Policies](concepts/policies.md).

**A rule never matches.** Use the **Playground** with the exact call: it
shows the policy result and which rules matched. Common causes:

- reading an argument without `has` first: `context.args has amount &&
context.args.amount > 50000`;
- comparing a number sent as a string;
- matching an agent by name with `principal == Agent::"name"` instead of
  `principal.name == "name"`; entity ids are agent ids;
- a tool name with a different spelling from the one agents send.

**A `forbid` rule errored and the call was held.** When a `forbid` cannot
be evaluated for a call, Veto holds the call rather than let it through,
and records the error on the action. Guard optional fields with `has`.

**"version N no longer compiles".** An old version you tried to activate
fails to compile with the current Cedar rules, usually after an upgrade.
Copy its source into a new version, fix it, and save.

**A pack will not save.** Every problem is listed at once with where it
is, such as `signal 2 (audience): needs instructions`. The [format](concepts/question-packs.md#format)
describes each field. The name inside the definition must match the name
you save it under.

## Local development

**`portless` fails to start or asks for a newer Node.** portless needs
Node 24 (`nvm use` reads `.nvmrc`). The console itself also runs on Node
22.12; run it without portless with
`PORTLESS=0 pnpm --filter @veto/console dev:app`.

**The browser warns about the certificate on `https://veto.localhost`.**
portless trusts its certificate authority on first run, which may need
your password. If you declined, run portless again. Firefox may use its
own certificate store; import portless's authority there if Firefox alone
complains.

**`veto.localhost` does not resolve.** Most systems resolve every
`*.localhost` name to the loopback address. Some resolvers do not; add
`127.0.0.1 veto.localhost api.veto.localhost` to `/etc/hosts`.

**The console says "Veto could not be reached", or `/v1` requests fail
with `502`.**
The console forwards `/v1` to `VETO_API_URL`, `https://api.veto.localhost`
by default in development. Check that vetod is running (`make dev-server`)
and that the console was started by portless, which sets
`NODE_EXTRA_CA_CERTS` so Node trusts the local certificate. If you run the
console without portless, set `VETO_API_URL` to vetod's plain address, such
as `http://127.0.0.1:8080`.

**Tests that need PostgreSQL are skipped.** Set `VETO_TEST_DATABASE_URL`
to a database whose user may create databases, for example the one
`make db` starts. Each test creates its own database, migrates it, and
drops it afterwards.

**`next build` breaks the running dev server.** They share `.next`. Stop
the dev server, run the build, and remove `apps/console/.next` before
starting dev again.
