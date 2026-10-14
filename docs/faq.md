# Questions and answers

## The idea

**What problem does Veto solve?**
AI agents now send email, move money, merge code and change data. The
usual safeguards are a confirmation prompt (which people learn to click
through), a hand-written allowlist (which breaks the first time an agent
does something useful and new), or a second language model asked "is this
safe?" (which is slow, costs tokens, and can be talked out of its answer by
the same injected text that misled the agent). Veto replaces these with one
layer that every call passes through: deterministic rules first, a typed
decision model for judgement, a person for what remains uncertain, and a
record of all of it.

**Why three options instead of allow or deny?**
Most risky calls are not wrong, they are too broad: an email to an extra
recipient, a refund larger than asked for, a merge without review. Running
a narrower version keeps the agent useful while removing the risk. Veto
builds that version in code from rules you control, and the model chooses
between running the call, running the safer version, and stopping. See
[Safer versions](concepts/safer-versions.md).

**Why not ask a language model whether the call is safe?**
Because its answer is text, and text can be steered. A typed model answers
fixed questions with probabilities over fixed choices; there is no
generated text for an attacker to shape, no output to parse, and a
confidence figure Veto can calibrate against reality. It is also much
faster. See [Decisions](concepts/decisions.md#why-a-typed-model).

**What is Jev, and is it the same as JEPA?**
Jev is TypeSafe's decision model, served by its System One API. It is not
related to JEPA, Meta's joint-embedding predictive architecture. Veto sends
Jev a state and a set of typed questions (choices, scores and yes-or-no
questions) and receives calibrated probabilities for each answer.

**Does Veto work without TypeSafe?**
Yes. Without a key, policies decide what they can and every other call
waits for a person. That is a sound way to start: you see exactly what
your agents do before you let anything run on its own.

## Using it

**Which agents and frameworks does it work with?**
Any. The SDKs for TypeScript and Python wrap a tool call in one function;
anything else uses the HTTP API. Agents that use MCP servers need no code
change at all: point them at the [MCP proxy](guides/mcp.md).

**How much latency does Veto add?**
For calls that policies decide (read-only tools, for example), a few
milliseconds plus a database round trip. For calls the model decides, the
model's latency dominates; `VETO_DECISION_TIMEOUT` bounds it at 2.5
seconds by default, and the console's **Overview** shows the actual median
and 95th percentile for your traffic.

**What happens if Veto is down?**
Agents cannot get decisions, so calls do not run; Veto fails closed by
design. Run at least two replicas and a highly available database, and see
[Scaling](operations/scaling.md). If the decision model alone is down,
Veto keeps working and applies each tool's fail mode: hold for a person,
or, for tools below the high risk class that you choose, run and record
that no model judged it.

**Can an agent skip Veto?**
An agent that holds a tool's credentials can call the tool directly. Two
things prevent that: let Veto hold the credentials, with
[connections](guides/connections.md) or the MCP proxy, so the agent never
has them; or have the tool [verify grants](concepts/grants.md), so a call
without one is refused.

**What does a reviewer see?**
What the agent was asked to do, who it acts for, the exact call, the safer
version as a diff, the reasons it was held, the model's view of each
question, and any untrusted content the agent read. They decide in the
console or from Slack. See [Reviewing held actions](guides/reviewing.md).

**How do I decide thresholds?**
Start with the defaults for each risk class, label the audit sample for a
week or two, and read **Calibration**: it shows how often the model was
right at each confidence level and suggests thresholds that meet your
target. See [Calibration and audits](guides/calibration.md).

**Can I write my own questions?**
Yes. A [question pack](concepts/question-packs.md) is a JSON document with
the decision criteria, the signals and the guards. Edit a built-in one or
write your own, then test it in the playground and with
[evaluations](guides/evaluations.md).

**Can one deployment serve several teams?**
A deployment has one workspace, created at first-run setup, and the
people in it have roles: viewers, reviewers, admins and owners. Teams that
need separate policies, agents and ledgers run separate deployments; they
can share a PostgreSQL server, each with its own database.

## Data and security

**What does Veto send to TypeSafe?**
For each model decision: the tool's name and description, the agent's
name, the request, the proposed call, the safer version, and any untrusted
content (truncated to the workspace's limit). Nothing else: no keys, no
other actions, no people's details beyond what the call itself contains.
Every request is stored on the action, so you can see exactly what was
sent.

**Where is data stored?**
In your PostgreSQL database. Veto has no hosted component and sends no
telemetry.

**How long is data kept?**
Arguments, requests, untrusted content and model exchanges are removed
after the workspace's retention period (90 days by default). Decisions,
hashes and the ledger are kept, so the record of what happened survives
without the data.

**Can the ledger be trusted if the database is compromised?**
Changes to past entries are detected, by the hash chain and by the signed
checkpoints. An attacker who holds both the database and the master key
could rewrite the chain and sign new checkpoints; to defend against that,
export checkpoints to storage they cannot reach. See
[The ledger](concepts/ledger.md).

**How do I report a vulnerability?**
Privately, as described in [SECURITY.md](../SECURITY.md). Please do not
open a public issue.

## The project

**What licence is Veto under?**
Apache 2.0. You can run it, change it and build on it, commercially or
not.

**How is Veto versioned?**
Semantic versioning. The `/v1` API, webhook payloads, grant format and
configuration are stable within a major version. See
[Upgrading](operations/upgrading.md).

**How can I contribute?**
See [CONTRIBUTING.md](../CONTRIBUTING.md). Labelled evaluation cases for
the built-in packs are one of the most useful contributions, and need no
Go or TypeScript.
