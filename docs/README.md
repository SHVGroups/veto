# Veto documentation

Veto sits between AI agents and the actions they take. An agent proposes a
tool call; Veto checks it against your policies, builds a safer version,
asks a typed decision model to choose between running it, running the safer
version or stopping it, and holds anything uncertain for a person. Every
step is recorded in a tamper-evident ledger.

## Start here

- [Quickstart](getting-started/quickstart.md): run Veto and send your first action in ten minutes.
- [Local development](getting-started/local-development.md): work on Veto itself, with named local URLs.
- [How Veto works](concepts/how-veto-works.md): the path of one call, end to end.

## Concepts

- [Decisions](concepts/decisions.md): the three options, the decision model, confidence, risk and thresholds.
- [Policies](concepts/policies.md): Cedar rules that run before the model.
- [Question packs](concepts/question-packs.md): the typed questions the model answers.
- [Safer versions](concepts/safer-versions.md): rules that build option B in code.
- [Grants](concepts/grants.md): signed, single-use proof that a call was approved.
- [The ledger](concepts/ledger.md): the hash chain and signed checkpoints.

## Guides

- [Connecting agents](guides/agents.md): the TypeScript and Python SDKs, and the raw HTTP API.
- [The MCP proxy](guides/mcp.md): put Veto in front of an MCP server without changing the agent.
- [Connections](guides/connections.md): let Veto hold API credentials and run approved calls.
- [Reviewing held actions](guides/reviewing.md): the review queue, keyboard use and step-up.
- [Slack](guides/slack.md): approve from a channel.
- [Webhooks](guides/webhooks.md): events, payloads and signature verification.
- [Calibration and audits](guides/calibration.md): measure the model and tune thresholds.
- [Evaluations](guides/evaluations.md): test packs offline against labelled cases.
- [The console](guides/console.md): a tour of every page.

## Reference

- [HTTP API](reference/api.md) and the [OpenAPI document](../api/openapi.yaml)
- [Configuration](reference/configuration.md)
- [Command line](reference/cli.md)
- [Events](reference/events.md)
- [Metrics](reference/metrics.md)
- [Database](reference/database.md)

## Operations

- [Deployment](operations/deployment.md)
- [Security](operations/security.md)
- [Scaling](operations/scaling.md)
- [Backup and restore](operations/backup-and-restore.md)
- [Upgrading](operations/upgrading.md)
- [Monitoring](operations/monitoring.md)

## Help

- [Troubleshooting](troubleshooting.md)
- [Questions and answers](faq.md)
- [Architecture](architecture.md): how the code is organised, for contributors.
