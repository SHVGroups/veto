# Calibration and audits

A threshold such as "run alone above 0.85 confidence" is only worth
something if decisions made at 0.85 are actually right. Veto measures that
from labels your team produces as it works, and suggests thresholds from
the measurements.

## Where labels come from

- **Reviews.** When a person decides a held action the model assessed, the
  label is whether they chose what the model chose.
- **Audits.** Veto queues decisions the model made alone for audit: a
  random sample of them (2% by default, set under **Settings**), and every
  one whose outcome is reported as failed, reversed or a complaint. On the
  **Audits** page a reviewer gives a verdict: correct, should have been
  the safer version, should have waited for a person, or should have been
  stopped. A verdict of correct is agreement; anything else is not.

Decisions made by policy are not labelled: the model was not asked.

## Reading the reliability chart

The **Calibration** page groups labelled decisions by the model's
confidence, in bands of 0.1, and shows for each band how often reviewers
agreed with the model, with a 95% confidence interval (Wilson score).

- A filled green point is a band where the interval's lower end is at or
  above the workspace's calibration target (98% by default): decisions at
  that confidence are reliably right.
- A hollow point is above the target but with too few labels to be sure.
- A red point is below the target.

Bars under the chart show how many labels each band has. Filter by tool to
see one tool's curve with its current threshold and any suggestion.

## Suggested thresholds

For each tool, Veto walks down from the most confident band, adding up
labels and agreements, and finds the lowest confidence at which the
agreement's lower 95% bound still meets the target. It suggests nothing
until a tool has at least 30 labels in the range.

A suggestion lower than the current threshold means the model could decide
more of this tool's calls alone without falling below the target. A
suggestion higher than the current one means the current threshold lets
through decisions reviewers disagree with. **Apply** sets the tool's
threshold to the suggestion; the change is recorded in the ledger like any
other.

## The overview's error rate

**Wrong automatic approvals** on the overview is the share of audited
automatic decisions whose verdict was not correct, with its interval. It is
the closest thing to an error rate for decisions nobody looked at, and the
number to watch after lowering a threshold.

## Habits that keep calibration honest

- Label audits regularly; a handful a day is enough for most teams.
- Raise the sample rate for a while after changing a pack or a threshold.
- Look at the **Why actions were held** list: a reason that dominates often
  points at a threshold or a question worth revisiting.
- Change one thing at a time, and watch the curve for that tool.
