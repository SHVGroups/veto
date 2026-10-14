# @shvgroups/veto

The TypeScript client for [Veto](https://github.com/shvgroups/veto), the
approval layer between AI agents and the actions they take. Ask Veto before
each tool call, and run only what it approves.

Works in Node.js 18.17 and newer, Deno, Bun, and edge runtimes. No
dependencies.

```sh
npm install @shvgroups/veto
```

## Guard a tool call

```ts
import { Veto, VetoPendingError, VetoStoppedError } from "@shvgroups/veto";

const veto = new Veto(); // reads VETO_API_KEY and VETO_URL

try {
  const sent = await veto.guard(
    "gmail.send",
    { to: ["jane@example.com"], subject: "Your refund", body: "..." },
    (tool, args, grant) => gmail.call(tool, args, { grant }),
    {
      request: userMessage, // what the user asked the agent to do
      principal: { id: user.id, display: user.name },
      untrusted: [{ source: "email:inbound", content: inboundEmail }],
    },
  );
} catch (err) {
  if (err instanceof VetoStoppedError) {
    // Veto decided the call must not run. err.reasons says why.
  } else if (err instanceof VetoPendingError) {
    // Still waiting for a person after waitMs. err.action.review_url links to it.
  } else {
    throw err;
  }
}
```

`guard`:

1. submits the call with a fresh idempotency key and waits for a decision,
   up to `waitMs` (60 seconds by default);
2. runs your function with the tool and arguments Veto approved, which are
   the safer version when Veto chose it, and the grant token;
3. reports the outcome: `succeeded`, or `failed` with the error message if
   your function throws (the error is rethrown).

Always run the tool and arguments your function receives, not the ones you
proposed.

## Options

```ts
const veto = new Veto({
  apiKey: "veto_sk_...", // default: process.env.VETO_API_KEY
  baseUrl: "https://veto.example.com", // default: process.env.VETO_URL
  timeoutMs: 30_000, // per request, excluding long-poll waits
  maxRetries: 3, // for network errors, 429 and 5xx
  fetch: customFetch, // default: globalThis.fetch
});
```

Retries back off exponentially with jitter, respect `Retry-After`, and keep
the same idempotency key, so a retried submission never creates a second
action.

## Lower-level methods

```ts
// Propose a call and wait up to 25 seconds for a decision.
const action = await veto.actions.submit({ tool, args, request }, { waitSeconds: 25 });

// Read it again, optionally waiting.
await veto.actions.get(action.id, { waitSeconds: 25 });

// Wait as long as you like; polls in 25-second steps.
await veto.actions.waitForDecision(action.id, { timeoutMs: 10 * 60_000 });

// Report how it went. Anything but "succeeded" queues the decision for audit.
await veto.actions.reportOutcome(action.id, "reversed", { reason: "customer complained" });

// Have Veto run an approved http.<connection> call with the stored credential.
const response = await veto.actions.execute(action.id);
```

Every method accepts an `AbortSignal` where it waits.

## Verifying grants in a tool

A tool that receives calls from agents can refuse anything Veto did not
approve:

```ts
const veto = new Veto({ apiKey: process.env.VETO_TOOL_KEY });

const check = await veto.grants.verify({ token: grant, tool: "stripe.refund", args, consume: true });
// Throws VetoError with status 403 (mismatch, expired) or 409 (already used).
```

`consume: true` marks the grant used, so the same approval cannot run
twice. To verify offline, fetch the Ed25519 public keys with
`veto.grants.keys()` and check the signature yourself; see
[Grants](https://github.com/shvgroups/veto/blob/main/docs/concepts/grants.md).

## Errors

| Class              | When                                                                                                                 |
| ------------------ | -------------------------------------------------------------------------------------------------------------------- |
| `VetoStoppedError` | The action was stopped or expired. `action` holds the full result; `reasons` lists why.                              |
| `VetoPendingError` | The action is still waiting for a person after `waitMs`.                                                             |
| `VetoError`        | Any error response, or a network failure (`status` 0). Has `status`, `code`, `details`, `requestId` and `retryable`. |

## Documentation

- [Connecting agents](https://github.com/shvgroups/veto/blob/main/docs/guides/agents.md)
- [HTTP API](https://github.com/shvgroups/veto/blob/main/docs/reference/api.md)
- [How Veto works](https://github.com/shvgroups/veto/blob/main/docs/concepts/how-veto-works.md)

## Licence

Apache 2.0.
