# Webhooks

Veto sends a signed `POST` with a JSON body to your endpoints when things
happen. Use them to page someone when actions pile up, to mirror decisions
into your own systems, or to keep copies of ledger checkpoints.

## Events

| Event                        | Sent when                                                                                                        |
| ---------------------------- | ---------------------------------------------------------------------------------------------------------------- |
| `action.pending_review`      | An action is held for a person. Sent again, with `"reminder": true`, halfway to its deadline if still undecided. |
| `action.decided`             | An action is approved or stopped, by anyone.                                                                     |
| `action.expired`             | A held action reached its deadline and was stopped.                                                              |
| `outcome.reported`           | An agent reported how a call went.                                                                               |
| `audit.sampled`              | A decision was queued for audit.                                                                                 |
| `tool.discovered`            | An agent called a tool Veto had not seen.                                                                        |
| `ledger.verification_failed` | The daily ledger check found a broken chain.                                                                     |
| `webhook.test`               | You pressed **Send a test event**.                                                                               |

## Payload

```json
{
  "id": "dlv_01m4w2...",
  "event": "action.decided",
  "workspace_id": "ws_01m4vt...",
  "created_at": "2026-10-14T09:26:05.512Z",
  "data": {
    "id": "act_01m4vtszfwnerf4qm6anjfjjn1",
    "tool": "gmail.send",
    "agent_id": "agt_01m4vts8dw85hmb4hcgta3c8aq",
    "agent_name": "support-bot",
    "principal": { "id": "u_42", "display": "Jane Doe" },
    "request": "Reply to the customer about their ticket",
    "status": "approved",
    "source": "api",
    "received_at": "2026-10-14T09:26:05.101Z",
    "review_url": "https://veto.example.com/actions/act_01m4vtszfwnerf4qm6anjfjjn1",
    "decision": {
      "option": "proceed_safer",
      "outcome": "approved",
      "decided_by": "human",
      "reasons": [{ "code": "reviewer_decision", "detail": "console" }],
      "final_tool": "gmail.create_draft",
      "confidence": 0.66,
      "reviewer_id": "usr_01m4vts8c5cnqpejs3vk07jf3r"
    }
  }
}
```

`action.pending_review` carries `hold_reasons` and `review_deadline`
instead of `decision`. The request text is cut to 500 characters.
Arguments are never included; fetch them with the API if you need them.

## Headers

| Header           |                                                                        |
| ---------------- | ---------------------------------------------------------------------- |
| `Veto-Event`     | The event name.                                                        |
| `Veto-Delivery`  | The delivery id; the same on every retry. Use it to ignore duplicates. |
| `Veto-Signature` | `t=<unix seconds>,v1=<hex HMAC-SHA256>`.                               |
| `User-Agent`     | `veto-webhooks/1`                                                      |

## Verifying the signature

The signature is HMAC-SHA256, keyed with the endpoint's secret (the whole
`whsec_...` string), over the timestamp, a full stop, and the raw request
body. Reject requests older than five minutes, and compare in constant time.

Node.js:

```js
import crypto from "node:crypto";

export function verify(secret, header, rawBody) {
  const parts = Object.fromEntries(header.split(",").map((p) => p.trim().split("=")));
  if (!(Date.now() / 1000 - Number(parts.t) < 300)) return false;
  const expected = crypto.createHmac("sha256", secret).update(`${parts.t}.${rawBody}`).digest("hex");
  return parts.v1?.length === expected.length && crypto.timingSafeEqual(Buffer.from(expected), Buffer.from(parts.v1));
}
```

Python:

```python
import hashlib, hmac, time

def verify(secret: str, header: str, raw_body: bytes) -> bool:
    parts = dict(p.strip().split("=", 1) for p in header.split(","))
    if time.time() - int(parts.get("t", "0")) > 300:
        return False
    expected = hmac.new(secret.encode(), f"{parts['t']}.".encode() + raw_body, hashlib.sha256).hexdigest()
    return hmac.compare_digest(expected, parts.get("v1", ""))
```

Go:

```go
func verify(secret, header string, body []byte) bool {
	var ts, sig string
	for _, p := range strings.Split(header, ",") {
		k, v, _ := strings.Cut(strings.TrimSpace(p), "=")
		switch k {
		case "t":
			ts = v
		case "v1":
			sig = v
		}
	}
	t, err := strconv.ParseInt(ts, 10, 64)
	if err != nil || time.Since(time.Unix(t, 0)) > 5*time.Minute {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(ts + "."))
	mac.Write(body)
	return hmac.Equal([]byte(hex.EncodeToString(mac.Sum(nil))), []byte(sig))
}
```

Verify against the raw body bytes, before any JSON parsing.

## Delivery

- Answer with any 2xx within 20 seconds. Anything else is a failure.
- Failures are retried after 30 seconds, 2, 10 and 30 minutes, then 1, 3
  and 6 hours: eight attempts over about eleven hours. After that the
  delivery is marked failed.
- Events are delivered at least once and not necessarily in order. Use
  `Veto-Delivery` to drop duplicates and `created_at` to order.
- Endpoints must be reachable public addresses unless the operator allowed
  private networks.

Each endpoint's recent deliveries, with status codes, errors and payloads,
are listed on the **Webhooks** page. **Rotate signing secret** replaces the
secret at once; update your receiver with the new one.
