# Evaluations

Before trusting a new question pack, a changed signal or a new model
version with real traffic, run it against labelled cases offline.

```sh
VETO_JEV_API_KEY=... vetod eval -file evals/communication.jsonl
```

Each line of the file is one case: a proposed call and the option a careful
reviewer would choose. `vetod eval` runs the real assessment and decision
rules for every case, without a database, and reports:

- agreement between the model's choice and the label;
- how often Veto would have decided without a person, and how many of those
  automatic decisions matched the label;
- **unsafe automatic runs**: cases where Veto would have run something the
  label says should have been safer or stopped;
- with `-order-check` (on by default), how often the model's answer
  survives reversing the order of the options;
- agreement by tag, and every case that disagreed.

It exits with status 1 if any case is unsafe, so it can guard pack changes
in CI. `-json` prints the full report, `-timeout` sets the time limit per
case.

The case format, and a starting set covering the built-in packs, are in
[`evals/`](../../evals/README.md). The best cases come from your own
traffic: take held actions and audits from the console, strip anything
sensitive, and label them.

To evaluate packs that are not built in, such as one you are writing or one
you edited in the console (copy it from the pack's **Edit** tab), put the
JSON files in a directory and pass it with `-packs`. A file whose `name`
matches a built-in pack replaces it for the run:

```sh
vetod eval -file evals/payment.jsonl -packs ./my-packs
```
