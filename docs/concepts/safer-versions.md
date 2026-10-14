# Safer versions

Option B, the safer version, is built from the proposed call by rules you
set on each tool. The rules run in code, in order, on a copy of the
arguments; the decision model only chooses whether B is the right option,
it never writes it. When the rules change nothing for a call, B is not
offered.

Every rule that changes something adds a one-line description, such as
"Caps amount at 5000 (was 490000)". Reviewers see these lines and a diff of
the arguments; the model sees them in `safer_variant.changes`.

## Rules

| Rule             | Fields          | What it does                                                                                                                                                                                                      |
| ---------------- | --------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `swap_tool`      | `tool`          | Runs a different tool with the same arguments, such as `gmail.create_draft` instead of `gmail.send`.                                                                                                              |
| `cap_number`     | `path`, `max`   | Lowers a numeric argument to `max` when it is above it. Exact for integers of any size.                                                                                                                           |
| `filter_domains` | `path`, `allow` | Removes email addresses whose domain is not in `allow` (subdomains count). The argument can be a list, a single address or a comma-separated string. Display names such as `Jane <jane@acme.com>` are understood. |
| `set`            | `path`, `value` | Sets an argument to a fixed JSON value, such as `"read_only": true` or `"environment": "staging"`.                                                                                                                |
| `remove`         | `path`          | Deletes an argument, such as `force` or `cc`.                                                                                                                                                                     |
| `limit_items`    | `path`, `max`   | Keeps the first `max` items of a list.                                                                                                                                                                            |

`path` is a dotted path into the arguments object: `amount`,
`options.cc`, `message.recipients`. A rule whose path does not exist in a
call does nothing for that call.

## Examples

Draft instead of send, and only to your own domain:

```json
[
  { "op": "filter_domains", "path": "to", "allow": ["acme.com"] },
  { "op": "swap_tool", "tool": "gmail.create_draft" }
]
```

Refund at most $50:

```json
[{ "op": "cap_number", "path": "amount", "max": 5000 }]
```

Deploy to staging instead of production:

```json
[{ "op": "set", "path": "environment", "value": "staging" }]
```

Share read-only, and only inside the company:

```json
[
  { "op": "filter_domains", "path": "emails", "allow": ["acme.com"] },
  { "op": "set", "path": "role", "value": "reader" }
]
```

## Choosing rules

A good safer version still does useful work and can be undone: a draft
instead of a sent message, a pull request instead of a merge, a smaller
refund, staging instead of production, read-only access. If there is no
such version for a tool, leave it without rules; reviewers and the model
then choose between running the call and stopping it.

The tool page in the console has a **Try the rules** panel that applies
your rules to sample arguments before you save. Through the API,
`POST /v1/safer/preview` does the same.

## What actually runs

When B is chosen, `decision.tool` and `decision.args` in the action result
are the safer call, and the grant is bound to them. Agents must run the
call from the decision, not the one they proposed; the SDKs' `guard`
helpers do this for you. The MCP proxy and connections rewrite the call
themselves.
