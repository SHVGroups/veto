# Grants

A grant is Veto's proof that one specific call was approved. Every approved
action's result includes one:

```json
"grant": { "token": "vg1.eyJnaWQiOi...ZXhwIjoxNz...", "expires_at": "2026-10-14T09:31:05Z" }
```

A grant:

- names one tool and the hash of its exact arguments;
- is signed with the workspace's Ed25519 key;
- expires five minutes after it is issued;
- can be consumed once.

If an agent asks for the result again after its grant expired unused, it
gets a fresh grant. After the action has run, it does not get another.

## Why check grants

An agent that is compromised, or simply buggy, can skip asking Veto. A tool
server that refuses calls without a valid grant closes that gap: the call
cannot run unless Veto approved exactly that call, and it cannot run twice.

## Verifying with the API

The simplest check is one request from the tool server, with any Veto API
key:

```http
POST /v1/grants/verify
Authorization: Bearer veto_sk_...

{ "token": "vg1....", "tool": "stripe.refund", "args": { "charge": "ch_3001", "amount": 4900 }, "consume": true }
```

`200` with `"valid": true` means: the signature is good, the grant has not
expired, it is bound to exactly this tool and these arguments, and, with
`consume`, it has now been used. Anything else is `403` with the reason.
Argument order and number formatting do not matter; values do.

## Verifying offline

Tool servers that cannot call Veto can verify the signature and binding
themselves; they cannot enforce single use without the API.

A token is `vg1.<payload>.<signature>`, both parts base64url without
padding. The signature is Ed25519 over the bytes `veto.grant.v1\n`
followed by the decoded payload. The payload is JSON:

| Field        | Meaning                                  |
| ------------ | ---------------------------------------- |
| `gid`        | grant id                                 |
| `aid`        | action id                                |
| `ws`         | workspace id                             |
| `tool`       | the approved tool                        |
| `ah`         | hex SHA-256 of the arguments, below      |
| `opt`        | the option, `proceed` or `proceed_safer` |
| `kid`        | the signing key's id                     |
| `iat`, `exp` | issue and expiry time, Unix seconds      |

The arguments hash is SHA-256 over `veto.args.v1\n`, the tool name, a
newline, and the canonical JSON of the arguments: RFC 8785, except that
integers are written exactly at any size instead of through IEEE 754
doubles.

Public keys are at `GET /v1/grants/keys`, hex encoded, with retired keys
kept so that grants issued before a rotation still verify. Pick the key
whose `key_id` matches `kid`.

## Key rotation

Owners rotate the signing key in the console under **Settings**, **System**,
or with `POST /v1/signing-keys/rotate`. New grants and ledger checkpoints use
the new key; the old one is retired, not deleted. Private keys are stored
encrypted with the master key.
