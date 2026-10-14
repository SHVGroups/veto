# Security policy

Veto sits between AI agents and the actions they take, so its security
matters to everyone who runs it. We appreciate reports and treat them as a
priority.

## Reporting a vulnerability

Report privately through GitHub:
[Report a vulnerability](https://github.com/shvgroups/veto/security/advisories/new).
Do not open a public issue, pull request or discussion.

Include what you can of:

- the affected version or commit;
- what an attacker can do, and what access they need first;
- steps or a proof of concept that reproduce it;
- any suggested fix.

## What to expect

- We acknowledge your report within three working days.
- We confirm or rule out the issue, and share our assessment of its
  severity, within ten working days.
- We fix confirmed issues in a patch release for the latest minor version,
  and publish a GitHub security advisory with credit to you, unless you
  prefer not to be named.
- We ask that you keep the issue private until the advisory is published,
  and we aim to publish within 90 days of your report.

## Scope

In scope: vetod, the console, the SDKs, the Helm chart and the Docker
Compose files in this repository. Examples of what we want to hear about:

- running a call that Veto did not approve, or running different
  arguments than the ones approved;
- forging, replaying or reusing grants;
- changing the ledger without verification noticing;
- bypassing authentication, roles, CSRF or origin checks;
- reading secrets sealed with the master key without it;
- reaching private network addresses through connections, MCP servers or
  webhooks when that is not allowed;
- injection into policies, packs or the console.

Out of scope:

- issues that need the master key, database access or an owner account
  already (see [what Veto does not defend against](docs/operations/security.md#what-it-does-not-defend-against));
- decisions the model got wrong, unless Veto's handling of the model's
  answer is at fault (report those as ordinary issues, with the action's
  evaluation);
- missing hardening on deployments that ignore the
  [deployment checklist](docs/operations/deployment.md#checklist);
- denial of service by volume.

## Supported versions

| Version | Security fixes |
| ------- | -------------- |
| 1.x     | Yes            |

## Hardening your deployment

See [Security](docs/operations/security.md) for how Veto protects data and
what operators should do.
