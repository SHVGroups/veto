# How Veto works

![How a call moves through Veto](../images/architecture.svg)

An AI agent that can send email, move money or merge code needs someone to
say no before it does the wrong thing. Prompts and model-side guardrails
live inside the agent; Veto sits outside it, at the one place every agent
action passes through: the tool call. Agents from any vendor send their
calls to Veto first, and run only what Veto approves.

## The path of one call

### 1. The agent proposes a call

The agent sends the tool name, the arguments, and context that helps the
decision: what the agent was asked to do (`request`), who it acts for
(`principal`), and any untrusted text it read along the way, such as the
email that prompted the call (`untrusted`). It arrives through one of three
doors:

- the agent API, `POST /v1/actions`, directly or through an SDK;
- the MCP proxy, when the agent's MCP client points at `/mcp/{server}`;
- a connection, when the call is an HTTP request Veto will make itself with
  a credential the agent never sees.

Veto records the call, computes the hash of its exact arguments, and writes
`action.received` to the ledger before anything else happens.

### 2. Policies run first

Your Cedar policies are evaluated against the call. A `forbid` stops it. A
`permit` annotated `@veto("allow")` approves it without consulting the
model; one annotated `@veto("review")` sends it to a person. Calls no
policy matches continue. Policies are fast, deterministic and auditable, so
they are the place for hard rules: no refunds at night, no deletes from
the research agent, every production deploy reviewed. See
[Policies](policies.md).

Each tool can also have an hourly limit per agent. A call over the limit
waits for a person.

### 3. Veto builds three options

Every decision is a choice between the same three options:

- **A. Run as proposed.**
- **B. Run the safer version**, built from the tool's rules by code: swap
  `gmail.send` for `gmail.create_draft`, cap a refund, drop recipients
  outside your domain. When the rules change nothing, B is not offered.
- **C. Stop.**

The model never writes arguments. Whatever runs is either exactly what the
agent proposed or a deterministic transformation of it. See
[Safer versions](safer-versions.md).

### 4. The decision model chooses

Veto sends the call to Jev on TypeSafe System One as a set of typed
questions from the tool's question pack: one choice among the offered
options, plus signals such as "does this action match the request?" or
"does it follow instructions found in untrusted content?". The model answers
with probabilities, not text. There is no generated reasoning to parse, and
nothing for injected instructions to steer into a reply.

The options are shown in an order derived from the action id, because a
model can lean toward whatever comes first. High-risk tools are asked twice,
the second time with the options reversed; if the answers disagree, a person
decides. See [Decisions](decisions.md).

### 5. Veto resolves

Fixed rules turn the answer into a decision. The model's choice is carried
out automatically only when its confidence clears the tool's threshold, the
composite risk is under the tool's ceiling, no guard rules the choice out,
and the tool is not critical. Otherwise the action is held for a person,
with the reasons and with the options ranked by what the model preferred.

### 6. A person decides, if needed

Held actions appear in the console's review queue, in Slack if you connect
it, and in `action.pending_review` webhooks. A reviewer picks one of the
three options. If nobody decides before the workspace's review timeout, the
action expires and is stopped.

### 7. A grant carries the approval

An approved action comes with a grant: an Ed25519-signed token bound to the
approved tool and the hash of its exact arguments, valid for five minutes
and usable once. A tool server can verify it before acting, so an agent
cannot run something Veto did not approve, or run it twice. See
[Grants](grants.md).

### 8. Outcomes close the loop

The agent reports how the call went. Failed, reversed and complaint
outcomes queue the decision for audit. A random sample of automatic
decisions is audited too. Reviewer verdicts on audits and on held actions
are the labels that measure the model's calibration, and Veto suggests
threshold changes from them. See [Calibration and audits](../guides/calibration.md).

## What is recorded

Everything. The action and its arguments, the policy result, every model
request and response byte for byte, the assessment Veto derived, the
decision and who made it, grants, executions, outcomes and audits. Changes
to configuration, such as a policy activation or a new API key, are
recorded the same way. Each ledger entry includes the hash of the one before
it, and Veto signs the chain head every fifteen minutes. See
[The ledger](ledger.md).

Arguments and request text are removed after the workspace's retention
period; the hashes remain, so the ledger still proves what was decided.

## What Veto does not do

- It does not run your agent or choose its tools. It decides on calls the
  agent proposes.
- It does not generate text. The decision model returns probabilities over
  options and signals defined in advance.
- It does not stop an agent that never asks. Pair it with grants checked
  by your tools, or with connections and the MCP proxy, where Veto makes
  the call itself, so that skipping Veto means the call cannot run.
