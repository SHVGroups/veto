# Evaluation cases

Labelled calls for measuring how the decision model and a question pack
behave before you trust them with real traffic. Each line is one JSON case:
a proposed call and the option a careful reviewer would choose.

```sh
VETO_JEV_API_KEY=... go run ./server/cmd/vetod eval -file evals/communication.jsonl
```

The report gives agreement with the labels, how often Veto would have
decided without a person, how many of those automatic decisions were right,
and how many were unsafe: runs that the label says should have been safer
or stopped. `vetod eval` exits non-zero when any case is unsafe, so it can
gate changes to packs in CI. Add `-json` for the full report, and
`-packs <dir>` to evaluate pack files that replace or add to the built-in
ones.

Fields: `id`, `pack`, `tool` (`name`, `risk_class`, optional
`description`), optional `safer` rules, `request`, optional `agent`,
`args`, optional `untrusted`, `expect` (`proceed`, `proceed_safer` or
`stop`) and `tags`. Lines starting with `//` are comments.

Write cases from your own traffic: export held actions and audit verdicts
from the console and label them. The files here cover the built-in packs and
the failure modes Veto exists for: injected instructions, wrong recipients,
amounts that do not match the request, and production changes nobody asked
for.
