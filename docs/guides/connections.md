# Connections

A connection lets Veto hold an API credential and make approved HTTP calls
with it. The agent proposes a request; Veto decides; if approved, Veto runs
it. The agent never sees the secret, so it cannot call the API without
Veto, and every request that runs was approved first.

## Create one

In **Connections**, add:

| Field           |                                                                                               |
| --------------- | --------------------------------------------------------------------------------------------- |
| Name            | Lowercase letters, digits, `-` and `_`. The tool becomes `http.<name>`.                       |
| Base URL        | Every request path is joined to it.                                                           |
| Authentication  | None, a bearer token, a custom header, or basic authentication (`user:password`).             |
| Secret          | Stored encrypted with the master key. It cannot be read back; enter a new value to rotate it. |
| Allowed methods | Requests with other methods are rejected before they reach the decision.                      |

Creating a connection registers its tool. Classify it on the **Tools** page
like any other, with safer-version rules if useful (for example, `set`
`method` to `GET`).

## Use one

```http
POST /v1/actions?wait=25
Authorization: Bearer veto_sk_...

{
  "tool": "http.zendesk",
  "args": {
    "method": "POST",
    "path": "/tickets/42/comments",
    "query": { "notify": "true" },
    "headers": { "Accept": "application/json" },
    "body": { "text": "Your refund was issued today." }
  },
  "request": "Tell the customer on ticket 42 that the refund went out"
}
```

When the action is approved:

```http
POST /v1/actions/{id}/execute
Authorization: Bearer veto_sk_...
```

The response has the upstream `status`, `headers`, `body` (JSON) or
`body_text`, `truncated` when the body was too large, and `latency_ms`. Veto
records the outcome itself: `succeeded` for 2xx and 3xx, `failed` otherwise.
An action runs once; a second `execute` is refused.

Agents may not set `Authorization`, `Cookie`, `Host` or other headers that
would override the connection's credential or the transport.

## Network safety

Connections cannot reach private, loopback or link-local addresses,
including cloud metadata endpoints, so a request cannot be pointed at your
internal network. The check is made on the resolved address of each
connection, so DNS cannot be used to get around it, and redirects are not
followed.

To allow a connection to reach internal services, the operator must set
`VETO_CONNECTOR_ALLOW_PRIVATE=true`, and then an admin must turn on
**Allow private network addresses** for that connection.
