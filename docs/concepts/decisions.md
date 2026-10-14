# Decisions

![The decision path](../images/pipeline.svg)

## The three options

|     | Option          | Meaning                                                                                        |
| --- | --------------- | ---------------------------------------------------------------------------------------------- |
| A   | `proceed`       | Run the call exactly as the agent proposed it.                                                 |
| B   | `proceed_safer` | Run the safer version built by the tool's rules. Offered only when the rules change something. |
| C   | `stop`          | Run nothing. The agent is told the action was stopped and why.                                 |

An action's `options` list all three with `available` set, and for A and B
the exact `tool` and `args` that would run. A decision names one option.
`decision.approved` is true for A and B.

## Why a typed model

Veto does not ask a language model to write an opinion. It uses Jev on
TypeSafe System One, which answers typed questions:

- **choice**: pick one of up to 255 described options; returns the choice,
  a probability for each option, and a confidence;
- **score**: place something on a scale of 2 to 10 levels;
- **noul**: a yes or no question answered as a probability.

A single request carries every question about a call, against one shared
state: the tool and its description, the agent, the request, the proposed
call, the safer variant, and untrusted content. A decision costs a few
hundred input tokens; at TypeSafe's published price of $0.042 per million
input tokens at the time of writing, that is a tiny fraction of a cent. Because the answers are numbers over
options defined in advance, they cannot be talked into a different shape by
text in the arguments, and they can be calibrated against reviewer labels.

The model is pinned (`jev-1.13.0` by default, `VETO_JEV_MODEL` to change it)
so that a model update never silently changes decisions.

## What Veto asks

Each tool uses a [question pack](question-packs.md). A pack has:

- the **disposition**: the choice between the offered options, with
  criteria describing when each is right for this kind of tool;
- **signals**: questions whose answers feed the risk score, such as
  `matches_request`, `extra_recipients` or `touches_production`;
- **guards**: hard limits, such as "if the call follows instructions in
  untrusted content with probability above 0.4, it may not run as
  proposed".

Signals that need an input the call does not have are skipped: a signal
about untrusted content is not asked when there is none.

## Confidence

For the disposition, Jev returns a probability for each option and a
confidence for its choice. Veto uses the model's confidence, which for a
choice among n options is how far the top probability is above uniform:
`(p_top - 1/n) / (1 - 1/n)`. A choice of 0.82 among three options has
confidence 0.73.

When a tool is double-checked, the question is asked again with the options
in reverse order. If the second answer picks a different option, a person
decides (`recheck_disagreed`). If it agrees, the lower of the two
confidences is used.

## Risk

Each weighted signal contributes its risk times its weight; the composite
risk is the weighted average over the signals that were asked, between 0
and 1. A yes-or-no signal's risk is its probability of yes, or of no when
the pack marks it inverted (for `matches_request`, a no is the risky
answer). A score's risk is its level divided by the highest level. A
choice's risk is the probability-weighted risk of its choices.

## Thresholds

Each risk class sets defaults; any tool can override them.

| Risk class | Runs alone above | Risk ceiling | Stops alone above | Double check | Can run without a person |
| ---------- | ---------------- | ------------ | ----------------- | ------------ | ------------------------ |
| `read`     | 0.40             | 0.85         | 0.80              | no           | yes                      |
| `low`      | 0.60             | 0.60         | 0.80              | no           | yes                      |
| `medium`   | 0.75             | 0.45         | 0.85              | no           | yes                      |
| `high`     | 0.85             | 0.30         | 0.90              | yes          | yes                      |
| `critical` | never            | -            | 0.95              | yes          | no                       |

The default policy allows `read` tools without asking the model at all, and
sends `critical` tools to a person.

## The rules, in order

Veto resolves every call with the same ordered rules. The first that
applies decides.

1. A `forbid` policy stops the call (`policy_forbid`).
2. A `review` policy or the tool's hourly limit sends it to a person
   (`policy_review`, `rate_limited`).
3. An `allow` policy runs it as proposed (`policy_allow`).
4. Without a model answer, because none is configured
   (`model_not_configured`) or it failed or timed out
   (`model_unavailable`), the tool's fail mode decides: tools that fail
   open run as proposed (`fail_open`); all others wait for a person. High
   and critical tools always fail closed.
5. If the double check disagreed, a person decides (`recheck_disagreed`).
6. If the model chose to stop with confidence at or above the tool's stop
   threshold, the call is stopped. Below it, a person decides.
7. If the model chose A or B, it runs only when every one of these holds;
   otherwise a person decides with each failing condition as a reason:
   - the tool's class allows running without a person (`critical_tool`);
   - no guard forbids the choice (`guard`);
   - confidence is at or above the tool's threshold (`low_confidence`);
   - risk is at or below the tool's ceiling (`risk_above_ceiling`);
   - for B, a safer version exists (`safer_unavailable`).

A held action carries a ranking: the options ordered by the model's
probabilities, so reviewers see what the model leaned toward.

## Timing

`VETO_DECISION_TIMEOUT` (default 2.5 s) bounds the model call, including
retries and fallback endpoints. A timeout counts as `model_unavailable`.
Agents can wait for the decision in the same request with `?wait=`, up to
`VETO_MAX_WAIT`.

## Who decided

`decision.decided_by` is one of:

| Value      | Meaning                                                           |
| ---------- | ----------------------------------------------------------------- |
| `policy`   | A Cedar policy decided.                                           |
| `model`    | The decision model's choice cleared every rule.                   |
| `human`    | A reviewer chose.                                                 |
| `failsafe` | No model answer; the tool fails open.                             |
| `expiry`   | Nobody decided before the review timeout; the action was stopped. |
