# Connecting agents

An agent asks Veto before each tool call and runs only what Veto approves.
There are four ways to do that; pick the one that fits how your agent calls
tools.

| Your agent                               | Use                                                       |
| ---------------------------------------- | --------------------------------------------------------- |
| Is your own code in TypeScript or Python | an SDK's `guard` helper                                   |
| Is your own code in another language     | the HTTP API                                              |
| Uses MCP servers                         | the [MCP proxy](mcp.md); no code changes                  |
| Calls HTTP APIs with a credential        | a [connection](connections.md); Veto holds the credential |

Every agent needs its own key: create the agent under **Agents** and copy
the key it shows. Set it where the agent runs as `VETO_API_KEY`, and set
`VETO_URL` to your Veto address.

## TypeScript

```sh
npm install @veto/sdk
```

```ts
import { Veto, VetoPendingError, VetoStoppedError } from "@veto/sdk";

const veto = new Veto(); // reads VETO_API_KEY and VETO_URL

async function sendEmail(args: { to: string[]; subject: string; body: string }, userMessage: string) {
  try {
    return await veto.guard(
      "gmail.send",
      args,
      // Runs only if approved, with the approved tool and arguments.
      (tool, approvedArgs) => gmail.call(tool, approvedArgs),
      { request: userMessage, principal: { id: user.id, display: user.name } },
    );
  } catch (err) {
    if (err instanceof VetoStoppedError) return `Not sent: ${err.message}`;
    if (err instanceof VetoPendingError) return `Waiting for approval: ${err.action.review_url}`;
    throw err;
  }
}
```

`guard` submits the call, waits up to `waitMs` (default 60 s) for a
decision, runs your function with the tool and arguments Veto approved,
which are the safer version when that was chosen, and reports the outcome:
`succeeded`, or `failed` with the error message if your function throws.

Lower-level methods are on `veto.actions`: `submit`, `get`,
`waitForDecision`, `reportOutcome` and `execute`; and on `veto.grants`:
`verify` and `keys`.

## Python

```sh
pip install veto-sdk
```

```python
from veto import Veto, VetoPending, VetoStopped

veto = Veto()  # reads VETO_API_KEY and VETO_URL

def send_email(args: dict, user_message: str):
    try:
        return veto.guard(
            "gmail.send",
            args,
            lambda tool, approved_args, grant: gmail.call(tool, approved_args),
            request=user_message,
            principal={"id": user.id, "display": user.name},
        )
    except VetoStopped as e:
        return f"Not sent: {e}"
    except VetoPending as e:
        return f"Waiting for approval: {e.action.get('review_url')}"
```

`AsyncVeto` has the same methods as coroutines; its `guard` awaits your
function.

## HTTP

### Propose a call

```http
POST /v1/actions?wait=25
Authorization: Bearer veto_sk_...
Content-Type: application/json

{
  "tool": "stripe.refund",
  "args": { "charge": "ch_3001", "amount": 4900, "currency": "usd" },
  "request": "Refund the duplicate $49 charge for Jane",
  "principal": { "id": "u_42", "display": "Jane Doe" },
  "untrusted": [{ "source": "email:inbound", "content": "Hi, I was charged twice..." }],
  "idempotency_key": "9d0c3c9e-3d1b-4e2c-9a51-7d2f2c3b9a10"
}
```

| Field             |                                                                                                                                                                   |
| ----------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `tool`            | Required. Letters, digits and `_ . : / -`, up to 200 characters. Use stable names, such as `gmail.send`.                                                          |
| `args`            | Required. The call's arguments, up to 256 KiB.                                                                                                                    |
| `request`         | What the agent was asked to do, in the words it was given. The single most useful input for a good decision.                                                      |
| `principal`       | Who the agent acts for. Policies can match on `id`; `display` is shown to reviewers.                                                                              |
| `untrusted`       | Text the agent read from sources it does not control: emails, web pages, documents, tool results. Veto checks whether the call follows instructions hidden in it. |
| `context`         | Extra facts for policies, as a JSON object up to 16 KiB.                                                                                                          |
| `idempotency_key` | Makes retries safe: the same key from the same agent returns the same action. The SDKs set one for you.                                                           |

`wait` holds the request until the action is decided, up to `VETO_MAX_WAIT`
(60 s by default). The response is `200` when decided and `202` when it is
waiting for a person.

### Read the result

```json
{
  "id": "act_01m4vtszfwnerf4qm6anjfjjn1",
  "status": "approved",
  "tool": "stripe.refund",
  "decision": {
    "option": "proceed_safer",
    "approved": true,
    "tool": "stripe.refund",
    "args": { "amount": 4900, "charge": "ch_3001", "currency": "usd" },
    "decided_by": "model",
    "reasons": [{ "code": "model_confident", "detail": "proceed_safer at confidence 0.91, risk 0.12" }],
    "confidence": 0.91
  },
  "grant": { "token": "vg1....", "expires_at": "2026-10-14T09:31:05Z" },
  "hold_reasons": [],
  "options": [...],
  "received_at": "2026-10-14T09:26:05Z",
  "decided_at": "2026-10-14T09:26:05Z"
}
```

`status` is `evaluating`, `pending_review`, `approved`, `denied` or
`expired`. While it is pending, poll `GET /v1/actions/{id}?wait=25`: each
request returns as soon as the action is decided, or after the wait.

When it is approved, **run `decision.tool` with `decision.args`**, not the
call you proposed. Pass `grant.token` to your tool if it checks grants.

### Report the outcome

```http
POST /v1/actions/{id}/outcome
{ "status": "succeeded" }
```

`status` is `succeeded`, `failed`, `reversed` (it ran but was undone) or
`complaint` (someone objected). Anything but `succeeded` queues the decision
for audit. `detail` can carry any JSON object.

## Errors and retries

Errors look like `{"error": {"code": "...", "message": "..."}, "request_id": "req_..."}`.
Retry `429` (respect `Retry-After`), `5xx` and network failures with
backoff, keeping the same `idempotency_key`. Do not retry `4xx` otherwise:
the request itself needs fixing. Each agent key is rate limited (50
requests per second with bursts of 100 by default, per server replica).

## Good practice

- Always send `request`. Decisions without it are less confident, so more
  of them wait for a person.
- Send untrusted content whenever the call was prompted by something the
  agent read.
- Name tools consistently across agents, so their profiles and statistics
  are shared.
- Report outcomes. They are how Veto finds the decisions that went wrong.
- Have the tools that matter verify grants, so skipping Veto does not work.
