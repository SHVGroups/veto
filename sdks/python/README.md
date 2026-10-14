# shvgroups-veto

The Python client for [Veto](https://github.com/shvgroups/veto), the
approval layer between AI agents and the actions they take. Ask Veto before
each tool call, and run only what it approves.

Python 3.10 or newer. Synchronous and asyncio clients, fully typed.

```sh
pip install shvgroups-veto
```

The package is published as `shvgroups-veto` and imported as `veto`.

## Guard a tool call

```python
from veto import Veto, VetoPending, VetoStopped

veto = Veto()  # reads VETO_API_KEY and VETO_URL

try:
    sent = veto.guard(
        "gmail.send",
        {"to": ["jane@example.com"], "subject": "Your refund", "body": "..."},
        lambda tool, args, grant: gmail.call(tool, args, grant=grant),
        request=user_message,  # what the user asked the agent to do
        principal={"id": user.id, "display": user.name},
        untrusted=[{"source": "email:inbound", "content": inbound_email}],
    )
except VetoStopped as e:
    ...  # Veto decided the call must not run; str(e) says why
except VetoPending as e:
    ...  # still waiting for a person; e.action["review_url"] links to it
```

`guard`:

1. submits the call with a fresh idempotency key and waits for a decision,
   up to `timeout` seconds (60 by default);
2. calls your function with the tool and arguments Veto approved, which
   are the safer version when Veto chose it, and the grant token;
3. reports the outcome: `succeeded`, or `failed` with the error message if
   your function raises (the exception is re-raised).

Always run the tool and arguments your function receives, not the ones you
proposed.

## Async

```python
from veto import AsyncVeto

async with AsyncVeto() as veto:
    result = await veto.guard("gmail.send", args, send_async, request=user_message)
```

`AsyncVeto` has the same methods as `Veto`, as coroutines. Its `guard`
awaits your function.

## Options

```python
veto = Veto(
    api_key="veto_sk_...",               # default: VETO_API_KEY
    base_url="https://veto.example.com",  # default: VETO_URL
    timeout=30.0,                         # per request, excluding long-poll waits
    max_retries=3,                        # for network errors, 429 and 5xx
    http_client=httpx.Client(proxy=...),  # optional, to configure proxies or TLS
)
```

Retries back off exponentially with jitter, respect `Retry-After`, and keep
the same idempotency key, so a retried submission never creates a second
action. Use the client as a context manager, or call `close()`, to release
its connections.

## Lower-level methods

```python
# Propose a call and wait up to 25 seconds for a decision.
action = veto.submit("stripe.refund", args, request=user_message, wait=25)

# Read it again, optionally waiting.
action = veto.get(action["id"], wait=25)

# Wait as long as you like; polls in 25-second steps.
action = veto.wait_for_decision(action["id"], timeout=600)

# Report how it went. Anything but "succeeded" queues the decision for audit.
veto.report_outcome(action["id"], "reversed", {"reason": "customer complained"})

# Have Veto run an approved http.<connection> call with the stored credential.
response = veto.execute(action["id"])
```

Results are plain dictionaries, typed with `TypedDict`s in `veto.types`.

## Verifying grants in a tool

A tool that receives calls from agents can refuse anything Veto did not
approve:

```python
from veto import Veto, VetoError

veto = Veto(api_key=os.environ["VETO_TOOL_KEY"])

try:
    veto.verify_grant(grant, "stripe.refund", args, consume=True)
except VetoError as e:
    # 403: wrong tool or arguments, or expired; 409: already used
    raise PermissionError(e.message)
```

`consume=True` marks the grant used, so the same approval cannot run twice.

## Errors

| Class         | When                                                                                                                             |
| ------------- | -------------------------------------------------------------------------------------------------------------------------------- |
| `VetoStopped` | The action was stopped or expired. `action` holds the full result.                                                               |
| `VetoPending` | The action is still waiting for a person after `timeout`.                                                                        |
| `VetoError`   | Any error response, or a network failure (`status` 0). Has `status`, `code`, `message`, `details`, `request_id` and `retryable`. |

## Documentation

- [Connecting agents](https://github.com/shvgroups/veto/blob/main/docs/guides/agents.md)
- [HTTP API](https://github.com/shvgroups/veto/blob/main/docs/reference/api.md)
- [How Veto works](https://github.com/shvgroups/veto/blob/main/docs/concepts/how-veto-works.md)

## Licence

Apache 2.0.
