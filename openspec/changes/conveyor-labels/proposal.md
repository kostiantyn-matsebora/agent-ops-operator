## Why

**A remote session cannot approve its own work, and the shipped design asked it
to.** `change-delivery` says the approve label is "placed by the session working
the change... under the owner's own credentials" — but a session acts as
`claude[bot]`, which holds no write access. Measured live on #201: the session
opened its pull request carrying `autofix`, the fixing loop's gate asked whether
the labeller may push here, the answer was no, and the label was stripped with a
refusal comment. The pull request now sits green, reviewed and unlabelled, with
no session left to do anything about it. The gate was right; the instruction was
wrong.

**And the happy path asks for three decisions where one was meant.** Labelling
an issue means build this, make it mergeable, and archive it once I merge — but
today each stage is a separate label a person places, on a pull request and then
on a merged change, long after the decision was made.

**The bound is also the wrong shape.** `MAX_ROUNDS: 3` stops the loop whether it
is converging or stalling. #198 ran four review rounds producing 14, 10, 8 then
1 finding — strictly decreasing, nearly all of them new — and only finished
because a person drove it past the bound by hand.

## What Changes

- **ONE PREFIX, FIVE LABELS.** `conveyor:` names the line the work moves along.
  `conveyor:implement` on an issue, `conveyor:fix` on a pull request and
  `conveyor:archive` on a merged change each run ONE station and stop.
  `conveyor:keep-going` grants another set of rounds. `conveyor:run` on an issue
  runs the whole line.
- **TWO LANES, AND THE ISSUE SAYS WHICH — NOTHING INFERS IT.** An issue already
  bound to an openspec change runs the opsx lane: propose, apply, archive, phase
  labels. **An issue that is not runs a PLAIN lane** — implement what the issue
  describes, open the pull request, drive it green, stop at the merge. There is
  nothing to archive, because there is no change. A well-described issue IS the
  agreement; requiring a proposal that restates it produced a 165-line change
  directory for a one-line README fix on the first live run.
- **THE LANE IS READ, NEVER JUDGED.** An issue is on the opsx lane when
  `openspec/changes/*/.github-issue` holds its number or it carries an `opsx:`
  phase label — both written by `opsx-issue.sh`, both facts a program reads.
  What decides that code is written to a branch may not be a classifier reading
  whether an issue sounds like it needs a proposal.
- **`conveyor:run` IS A STANDING INSTRUCTION, READ AT EVERY TRANSITION** — not a
  record of a past decision. When the session opens its pull request the
  workflow consults the issue's labels and places `conveyor:fix`; when that pull
  request merges it consults them again and places `conveyor:archive`. Removing
  it halts the line at the next station, because it is read live rather than
  remembered.
- **A PROGRAM MAY CARRY A GRANT FORWARD OR CONSUME ONE. IT MAY NEVER MINT ONE.**
  Every label that grants something is placed by a person whose write access is
  read from the platform. A workflow carrying `conveyor:run` forward records
  whose it was; a session places nothing. That is the whole fix for #201.
- **THE BOUND IS A NAMED CONSTANT, DEFAULT 5**, and `conveyor:keep-going` buys
  another 5 and is CONSUMED when taken — one placement, one grant, so continuing
  is always a fresh decision made knowing what the last rounds produced.
- **A FIXER THAT SAID NOTHING IS NOT A FIXER THAT DISAGREED.** When the model
  writes no report, the workflow substitutes an empty one and every item lands
  as "disputed: not addressed by the fixing step" — which reads as considered
  refusal and is silence. Measured on #205, where three findings came back that
  way and no model had spoken. A round whose fixer returned NOTHING ends as its
  own outcome, named; an item merely missing from a real report is UNADDRESSED,
  not disputed. The distinction matters more under this change than before it:
  five unattended rounds of phantom disputes reads as a machine that considered
  the work five times.
- **`autofix` IS RETIRED INTO `conveyor:fix`**, with the old name recorded in
  `.github/retired-vocabulary.json` so it cannot quietly return.

## Capabilities

### New Capabilities
- `conveyor-lifecycle`: the labelled stations work passes through unattended,
  the two lanes and what selects one, who may place each label, what a program
  may do with one, and where the line stops for a person.

### Modified Capabilities
- `change-delivery`: "A change is approved for automatic fixing by its owner,
  once" is rewritten — the owner's word is a label THEY place, carried forward
  by a program rather than re-issued by a session, and the sentence naming the
  session as the placer is deleted as the defect it was.
- `automated-code-review`: the approve label is renamed; the round bound becomes
  a named constant with a stated default; a bounded loop ends with a summary
  that says so, and `conveyor:keep-going` restarts it.
- `change-issue-tracking`: the tracking issue carries the standing instruction,
  so its labels are read at each transition rather than at the start.
- `remote-change-sessions`: **not modified as a capability, and deliberately
  so.** Its spec says the session delivers "carrying the approve label for
  automatic fixing from creation", which this change makes false — but that spec
  is not yet published: it lives in the unarchived `remote-change-sessions`
  change. Editing a delta that has not landed would put two changes in a race
  over one file. Whichever archives second reconciles it, and the task list says
  so.

## Impact

**Code and configuration**

- `.github/review-triage.json`: `approve_label` becomes `conveyor:fix`, plus
  `run_label`, `implement_label`, `archive_label`, `keep_going_label` and the
  round bound.
- `.github/workflows/remote-implement.yml` (the trigger widens to
  `conveyor:run` and `conveyor:implement`), a new job or workflow watching
  `pull_request: closed` with `merged == true` to carry the grant to archiving,
  and `.github/workflows/review-dispatch.yml` (the renamed label, the constant,
  the consume-on-use of `conveyor:keep-going`).
- `.github/scripts/remote-implement.py`, `land-dispatch.py` (the bound and the
  summary), and a program that carries a label forward recording its origin.
- `.github/routines/implement-issue.md`: the session opens its pull request with
  NO label and says so.
- `.github/retired-vocabulary.json`: `autofix`.

**Documents the change makes untrue**

- `.claude/rules/worktree-delivery.md`: the consent table, every `autofix`
  mention, and the round bound.
- `.claude/rules/remote-session.md`: the label table and the sentence saying the
  pull request carries the approve label from creation.
- `CONTRIBUTING.md`: the `autoimplement` paragraph and the autofix paragraph.
- `.claude/skills/openspec-apply-change/SKILL.md`: where the label is placed on
  the owner's word.
- `docs/CHANGELOG.md`: nothing — no chart, CRD or image changes.

**Documents the change makes untrue — adopter site**

- None. This alters how the project is DEVELOPED and touches no shipped
  behaviour. Stated so the absence is a claim a reviewer can dispute.
