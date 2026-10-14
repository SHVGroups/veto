# Events

Veto reports changes in two ways: a live stream for the console, and
[webhooks](../guides/webhooks.md) for your systems.

## The live stream

`GET /v1/events` (session or admin key) is a Server-Sent Events stream.
Each event's name is its kind; its data is JSON:

```
event: action.held
data: {"ws":"ws_01m4...","kind":"action.held","action_id":"act_01m4...","status":"pending_review","subject":"gmail.send","at":"2026-10-14T09:26:05.4Z"}
```

| Kind               | When                             | `subject` |
| ------------------ | -------------------------------- | --------- |
| `action.received`  | An action arrived.               |           |
| `action.held`      | It was held for a person.        | the tool  |
| `action.decided`   | It was decided or expired.       | the tool  |
| `outcome.reported` | An outcome was reported.         |           |
| `audit.sampled`    | A decision was queued for audit. |           |
| `audit.recorded`   | An audit was labelled.           |           |
| `tool.registered`  | A new tool was seen.             | the tool  |

Events say what changed, not the new state; clients fetch what they show.
The server sends a comment every 25 seconds to keep proxies from closing
the connection, and asks clients to reconnect after 3 seconds if it drops.

Events travel between replicas through PostgreSQL `LISTEN` and `NOTIFY` on
the `veto_events` channel, so a console connected to one replica sees
actions handled by any other.

If a reverse proxy sits in front of Veto, disable response buffering for
`/v1/events` (vetod sends `X-Accel-Buffering: no` for nginx).
