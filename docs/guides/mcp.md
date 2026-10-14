# The MCP proxy

If your agent uses MCP servers over Streamable HTTP, Veto can sit in front of
them without any change to the agent: point the agent's MCP client at Veto
instead of the server.

## Set up

1. In the console, open **MCP servers** and add the server: a name, such as
   `github`, and its Streamable HTTP URL. If the server needs a credential,
   create a [connection](connections.md) for it first and select it; Veto
   attaches it to forwarded requests, so the agent never holds it.
2. Point the agent's MCP client at `https://<your veto>/mcp/github` with
   the agent's Veto key:

```json
{
  "mcpServers": {
    "github": {
      "type": "http",
      "url": "https://veto.example.com/mcp/github",
      "headers": { "Authorization": "Bearer ${VETO_API_KEY}" }
    }
  }
}
```

## What happens

Every request is forwarded to the server unchanged, except `tools/call`:

- Veto submits the call as the tool `mcp.<server>.<tool>`, such as
  `mcp.github.merge_pull_request`. Classify these tools like any other.
- **Approved**: the call is forwarded. When the safer version was chosen,
  the forwarded call carries the safer arguments. The server's answer goes
  back to the agent as usual, and Veto records the outcome from it.
- **Stopped**: Veto answers with a tool result that has `isError: true` and
  says why, including any reviewer note. The agent's model sees an ordinary
  tool failure it can explain or work around.
- **Held**: Veto waits up to `VETO_MCP_HOLD_TIMEOUT` (50 s by default) for
  a reviewer. If none decides in time, it answers with a tool error saying
  the call is waiting for review, with a link. When the agent retries the
  same call in the same MCP session, it gets the same action, and runs if a
  reviewer has approved it by then.

A `tools/call` inside a JSON-RPC batch is rejected, because Veto could not
answer the calls in it separately.

## Giving Veto context

MCP requests do not say what the user asked for. Clients that can add
headers should send:

| Header                   | Meaning                                                 |
| ------------------------ | ------------------------------------------------------- |
| `Veto-Request`           | What the agent was asked to do.                         |
| `Veto-Principal`         | The id of the person the agent acts for.                |
| `Veto-Principal-Display` | Their name, for reviewers.                              |
| `Veto-Idempotency-Key`   | A key that makes a retried call map to the same action. |

Values may be percent-encoded. Without `Veto-Idempotency-Key`, Veto derives
a key from the MCP session id, the tool and the arguments.

## Limits

- Only Streamable HTTP servers can be proxied. For stdio servers, run them
  behind an HTTP bridge or call Veto from the agent with an SDK.
- A safer version must stay on the same MCP server. A `swap_tool` rule that
  points elsewhere is refused at run time.
