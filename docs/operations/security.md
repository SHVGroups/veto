# Security

Veto decides what AI agents may do, so it is a high-value target. This page
describes what it protects against, how, and what you must do as an
operator. To report a vulnerability, see [SECURITY.md](../../SECURITY.md).

## What Veto defends against

| Threat                                                       | Defence                                                                                                                                                                                                           |
| ------------------------------------------------------------ | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| An agent misled by injected instructions in an email or page | Untrusted content is sent to the model as data, an `injected` signal asks whether the call follows it, and guards rule out running such calls. The model returns probabilities, not text an attacker could shape. |
| An agent that skips Veto                                     | Tools verify grants; connections and the MCP proxy hold the credentials, so a call that skips Veto cannot run.                                                                                                    |
| An approved call replayed or altered                         | Grants are bound to the tool and the exact arguments hash, expire in five minutes and can be consumed once.                                                                                                       |
| A leaked agent key                                           | Keys are per agent, hashed at rest, rate limited, and revocable at once; disabling the agent stops everything it sends.                                                                                           |
| A stolen console session making approvals                    | Sessions are HttpOnly, Secure, SameSite=Lax cookies; changing requests need a custom header and an allowed Origin; step-up asks for the password before approving sensitive tools.                                |
| Requests to internal services through Veto                   | Connections, MCP servers and webhooks cannot reach private, loopback or link-local addresses unless explicitly allowed; the check uses the resolved address and redirects are not followed.                       |
| Someone rewriting history in the database                    | The ledger is a hash chain with signed checkpoints, verified daily.                                                                                                                                               |
| A database dump                                              | Secrets are encrypted with the master key; passwords and tokens are hashed.                                                                                                                                       |

## What it does not defend against

- An attacker with the master key and database access can decrypt stored
  secrets and forge checkpoints. Keep the master key in a secret manager,
  separate from database backups.
- A tool that does not verify grants trusts the agent. Verify grants in the
  tools that matter.
- Veto judges calls, not their consequences elsewhere. A call approved as
  harmless can still be one step of a harmful sequence; policies on
  sequences (such as hourly limits) narrow this, they do not remove it.

## Accounts and sessions

- Passwords: at least 12 characters, hashed with argon2id (19 MiB, 2
  iterations). Sign-in is rate limited per address and per email, and takes
  the same time whether or not the account exists.
- Sessions: random 256-bit tokens, stored hashed, valid for
  `VETO_SESSION_TTL` (14 days by default). Changing your password signs out
  your other sessions; removing a member signs them out everywhere.
- Roles are enforced on every request by the server.

## API keys

Agent keys start `veto_sk_`, admin keys `veto_ak_`. Both are random 256-bit
tokens shown once and stored as SHA-256 hashes, so a database copy cannot
be used to authenticate. Use one key per deployment of an agent, and revoke
keys you no longer use.

## The master key

`VETO_MASTER_KEY` encrypts, with AES-256-GCM, each workspace's Ed25519
signing keys, connection secrets, webhook signing secrets and Slack
credentials. Each ciphertext is bound to its row. Losing the key makes
these unreadable: you would have to recreate connections, webhooks and the
Slack integration, and new signing keys would be needed.

### Rotating the master key

1. Generate a new key: `vetod keygen`.
2. Deploy with `VETO_MASTER_KEY` set to the new key and
   `VETO_MASTER_KEY_PREVIOUS` set to the old one. Veto reads with either and
   writes with the new one.
3. Run `vetod vault rekey` once. It reseals every stored secret under the
   new key and reports how many it changed.
4. Run it again; it should report zero. Remove `VETO_MASTER_KEY_PREVIOUS`
   and redeploy.

## Network

- Terminate TLS in front of Veto. Keep `VETO_COOKIE_SECURE=true`.
- Do not expose `/metrics` publicly without `VETO_METRICS_TOKEN`.
- `VETO_TRUSTED_PROXIES` should list only your own proxies; a trusted peer
  can claim any client address.
- vetod sends `X-Content-Type-Options`, `Referrer-Policy: no-referrer`,
  `X-Frame-Options: DENY` and `Cache-Control: no-store` on every response;
  the console sends the first three on its pages.

## Data handling

Actions contain whatever your agents send, which can include personal data.
Retention (90 days by default) removes arguments, request text, untrusted
content and model requests after that time. Set it to what your policies
require. TypeSafe receives the state described in
[Question packs](../concepts/question-packs.md) for every model decision;
review its data terms for your use.

## Audit trail

Every configuration change, sign-in and decision is in the ledger with its
actor. Export it, or checkpoints at least, to storage Veto's operators do
not control if you need evidence that holds up against them.
