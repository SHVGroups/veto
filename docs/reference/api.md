# HTTP API

The complete API is described in [`api/openapi.yaml`](../../api/openapi.yaml),
an OpenAPI 3.1 document you can load into any OpenAPI viewer or client
generator. A test in the server fails if a route is missing from it. This
page covers what applies to every request.

## Base address

The console and the API share one origin. Use your `VETO_PUBLIC_URL`, such
as `https://veto.example.com/v1/actions`. Inside a cluster, services can
also call vetod directly on port 8080.

## Authentication

| Credential               | Header                              | Used for                                                                                                                   |
| ------------------------ | ----------------------------------- | -------------------------------------------------------------------------------------------------------------------------- |
| Agent key, `veto_sk_...` | `Authorization: Bearer veto_sk_...` | Proposing actions, reading their results, reporting outcomes, executing connection calls, the MCP proxy, verifying grants. |
| Admin key, `veto_ak_...` | `Authorization: Bearer veto_ak_...` | Everything the console does, with the admin role, for automation. Created by owners under **Settings**.                    |
| Session cookie           | `veto_session`                      | The console.                                                                                                               |

Keys are shown once when created and stored as SHA-256 hashes. Requests
with a session cookie that change anything must also send the header
`X-Veto-Requested-With` with any value, and an `Origin`, if the browser
sends one, that is allowed (see `VETO_PUBLIC_URL` and
`VETO_ALLOWED_ORIGINS`). Browsers cannot add that header to a cross-site
request without permission, which Veto never grants.

## Errors

```json
{
  "error": {
    "code": "invalid_request",
    "message": "tool must be 1 to 200 characters of letters, digits and _ . : / -",
    "details": null
  },
  "request_id": "req_6fa35e335541f6dc"
}
```

| Code              | Status | Meaning                                                                                              |
| ----------------- | ------ | ---------------------------------------------------------------------------------------------------- |
| `invalid_request` | 400    | The request is malformed or breaks a rule; the message says which. Unknown JSON fields are rejected. |
| `unauthorized`    | 401    | Missing, invalid or revoked credentials.                                                             |
| `forbidden`       | 403    | The credential cannot do this: wrong key type, role too low, or an invalid grant.                    |
| `not_found`       | 404    | No such resource in this workspace.                                                                  |
| `conflict`        | 409    | The state does not allow it, such as reviewing an action that is already decided.                    |
| `rate_limited`    | 429    | Too many requests for this key; wait for `Retry-After` seconds.                                      |
| `upstream_error`  | 502    | A connection or MCP server failed.                                                                   |
| `unavailable`     | 503    | Veto cannot serve the request right now.                                                             |

Every response carries `X-Request-Id`; include it when you report a
problem. Send your own `X-Request-Id` (up to 64 characters) to correlate
with your logs.

## Waiting

`POST /v1/actions` and `GET /v1/actions/{id}` accept `wait`, in seconds
(`25`) or as a duration (`25s`), capped by `VETO_MAX_WAIT`. The request
returns as soon as the action is decided, or when the wait ends, whichever
is first. Decisions are delivered through PostgreSQL notifications, so a
wait on one replica sees a decision made on another.

## Idempotency

`idempotency_key` on `POST /v1/actions` makes retries safe: the same key
from the same agent returns the existing action instead of creating a new
one. Keys are up to 200 characters.

## Pagination

`GET /v1/actions` returns `next_cursor`; pass it as `cursor` for the next
page. An empty cursor means the last page. `GET /v1/ledger` pages with
`before`, a sequence number, and returns `next_before`.

## Identifiers

Ids are prefixed ULIDs: `act_` actions, `agt_` agents, `key_` API keys,
`usr_` users, `ws_` workspaces, `evl_` evaluations, `grt_` grants, `out_`
outcomes, `aud_` audits, `con_` connections, `mcp_` MCP servers, `whk_`
webhooks, `dlv_` deliveries, `inv_` invitations. They sort by creation time.

## Times

All times are UTC, RFC 3339.

## Live events

`GET /v1/events` is a Server-Sent Events stream for the console. See
[Events](events.md).
