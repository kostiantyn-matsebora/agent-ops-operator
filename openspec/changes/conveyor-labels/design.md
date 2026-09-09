## Context

See proposal.md — Why. What shapes the approach, measured rather than assumed:

- **#201 IS THE WHOLE ARGUMENT.** A remote session opened its pull request
  carrying `autofix`, exactly as `.github/routines/implement-issue.md` told it
  to. `review-dispatch.yml`'s gate asked the collaborators API whether the
  labeller may push here; the labeller was `claude[bot]`; the answer was `none`.
  The gate removed the label, commented the refusal, and failed. Everything
  behaved as designed except the design.
- **The gate is right and must not move.** It exists so that a label — the thing
  that decides code gets written to a branch — counts only from someone the
  platform says may push. A bot exempted from it is the hole the whole mechanism
  was built to close.
- **The shipped spec states the impossible.** `change-delivery` says the label
  is placed by the session "under the owner's own credentials". A session has no
  credentials of its own; it acts as the app. That sentence is the defect, and
  no program change can satisfy it.
- **Consent already travels once, correctly.** `remote-implement.yml` reads who
  placed the issue label, checks their write access, and fires. That is the
  shape to generalise: a program reads a PERSON's grant and acts on it, rather
  than a machine asserting one.
- **The current bound is a count of landing comments since the label was
  placed** (`land-dispatch.py`, `count_rounds`), so removing and re-adding the
  label already resets it. `conveyor:keep-going` makes that deliberate instead
  of a side effect.
- **Convergence is real and the bound ignores it.** #198's four review rounds
  produced 14, 10, 8 then 1 finding, nearly all new each time. Three rounds
  would have stopped it mid-progress.

## Goals / Non-Goals

**Goals:**

- One label on an issue carries a change to a merged, archived state, stopping
  only where a person must decide.
- Every grant traces to a person the platform says may push, at every station.
- A stalled loop stops visibly and is restarted by one label, consumed on use.

**Non-Goals:**

- Giving the app write access, or exempting any actor from the gate.
- Merging. A person merges every pull request the line opens — twice on the
  opsx lane (the change, then its archive) and once on the plain lane, which
  archives nothing.
- Replacing the review or the fixing loop. Both are extended, not rebuilt.
- A state machine stored anywhere but the labels themselves.

## Decisions

### D1. `conveyor:run` is a STANDING INSTRUCTION on the issue, read at every transition

Not a record of a past decision, and not copied into a store. At each station
the workflow reads the ISSUE's current labels and decides whether to advance.

- **Removing it halts the line at the next station.** A stage already running
  finishes; nothing new starts. That is how a person says "stop after this"
  without cancelling work in flight, and it is the same live-read property
  `review-dispatch.yml`'s gate already has for its own label.
- **It is never consumed.** It stays on the issue as the visible reason the line
  is moving, and as the answer to "why did this archive itself".
- **A single-station label is the manual form**: `conveyor:implement`,
  `conveyor:fix` or `conveyor:archive` alone runs that station and stops. They
  are what a person uses to drive one step by hand, and what the workflow
  PLACES when carrying `conveyor:run` forward — so the stations have one
  meaning whoever placed the label.

### D1a. Two lanes, and the ISSUE selects one — no inference anywhere

An issue already bound to an openspec change runs the OPSX lane; every other
issue runs the PLAIN lane.

| Lane | Selected when | Stations |
|---|---|---|
| opsx | `openspec/changes/*/.github-issue` holds the number, OR an `opsx:` phase label is present | implement → fix → archive |
| plain | neither | implement → fix, ending at the merge |

**PROPOSE AND APPLY ARE ONE STATION, AND `conveyor:implement` DRIVES IT.** There
is no `conveyor:propose`: an issue on the opsx lane that carries no change yet
gets one proposed and implemented by the same session, in that order, because a
proposal nobody implements is not a station a person would ever want to stop
at — and one that already carries a change is continued rather than proposed
again, which the routine file's existing binding check already does. The lane's
stations are therefore three and two, not four and two.

- **BOTH TESTS ARE FACTS A PROGRAM READS**, and both are written by
  `opsx-issue.sh` rather than typed by anyone. This project's rules are explicit
  that what decides code gets written to a branch may not be a judgement call —
  so the lane is looked up, never guessed from how the issue reads.
- **THE PLAIN LANE ARCHIVES NOTHING**, because there is no change to fold into
  the published contract. `conveyor:run` therefore means "the whole of THIS
  issue's line", which is three stations on one lane and two on the other.
- **A well-described issue IS the agreement.** The proposal exists so a person
  reads the shape before the code; an issue somebody wrote and a maintainer
  labelled has already been read. Requiring a proposal anyway produced, on the
  first live run, a 165-line change directory whose delta specs said nothing,
  for a one-line README fix.
- **THE PLAIN LANE STILL OWES GREEN.** No tasks file, no delta specs, no
  trailing-sections gate — those bind openspec CHANGES, and this is not one. But
  "mergeable" means the same thing on both lanes: CI green, both guards clean,
  the docs generator satisfied, and the review answered.
- **An issue that GAINS a change mid-flight moves lanes at the next station**,
  because the lane is read at each transition exactly as the standing
  instruction is. Nothing migrates; the next station simply reads the world as
  it is.

### D2. A program may CARRY a grant or CONSUME one. It may never MINT one

The rule that fixes #201, stated as a rule rather than a patch:

| Actor | May place a label | Because |
|---|---|---|
| a person with write access | yes | the platform says they may push |
| a workflow carrying `conveyor:run` forward | yes, recording whose it was | it is relaying a person's grant, not asserting one |
| a workflow consuming `conveyor:keep-going` | it REMOVES only | removal grants nothing |
| a remote session | **never** | it is `claude[bot]` and holds no write access |

- **The carry records its origin.** When the workflow places `conveyor:fix` it
  comments once naming the person whose `conveyor:run` authorised it, so the
  pull request says whose decision it is rather than showing a bot's label.
- **The gate then passes**, because the label was placed by `github-actions[bot]`
  acting on a checked grant — and the gate is extended to accept a carried label
  ONLY when the issue it names still carries `conveyor:run` from a writer. It
  re-checks; it does not trust the carry.
- **The session opens its pull request with NO label at all.** The inverse of
  what shipped, and the one-line fix to the routine file.

### D3. The merge transition is a new trigger, because none exists

`pull_request: closed` with `merged == true`. The job reads the pull request's
body for its `Refs #<n>`, reads that issue's labels, and places
`conveyor:archive` ON THE ISSUE if `conveyor:run` is there.

- **On the issue, because the pull request is gone.** It merged; it is closed,
  and a label on a closed pull request drives nothing and is read by nobody. The
  tracking issue is what survives every station and is where the last one is
  asked for.

- **`Refs #<n>` is already required** by `pr-closes-guard.py` on every applying
  pull request, so the link is guaranteed to exist and is already validated.
- **Nothing watches merges today**, so this is genuinely new — a job in
  `remote-implement.yml` rather than a fourth workflow, since it is the same
  question asked at a different moment.
- **An archive pull request merging does NOT re-trigger.** It carries `Closes`,
  not `Refs`, which is how the job tells the last station from the others.

### D4. The bound is a named constant defaulting to 5, and `conveyor:keep-going` is consumed

`MAX_ROUNDS: 3` becomes `CONVEYOR_MAX_ROUNDS: 5` in the workflow's `env`, with
the vocabulary file naming the label that extends it.

- **Rounds are counted as they are today** — landing comments carrying the round
  marker since the grant was placed — so the counting code does not change, only
  the number and what resets it.
- **`conveyor:keep-going` is REMOVED when it takes effect**, by the model-free
  landing job that already holds `contents: write`. One placement, one grant of
  another 5; placing it again is a fresh decision made knowing what the last
  rounds produced.
- **Five rather than three** because #198 measured four productive rounds. Not a
  ceiling on progress — a ceiling on unattended spend.
- **A bounded ending says so.** The summary already names its ending; it gains
  "place `conveyor:keep-going` for another N" so the next action is in the text
  a person is already reading.

### D4a. Three outcomes, not two: fixed, disputed, and never spoken about

`land-dispatch.py` reads the model's report and classifies every work item as
fixed or disputed. There is no third state, so an item the report never mentions
becomes `disputed: not addressed by the fixing step` — the same word used when
the fixer looked at a finding and declined it, with a reason.

| The report | Today | Proposed |
|---|---|---|
| names the item `fixed`, and the patch backs it | fixed | unchanged |
| names it `disputed` with a reason | disputed | unchanged |
| does not name it | **disputed** | **UNADDRESSED**, named as such |
| does not exist at all | every item disputed | **the round ends as `no report`**, and says so |

- **MEASURED WHILE THIS CHANGE WAS ITSELF UNDER REVIEW.** Three findings on its
  proposal came back "disputed by the fixing step: not addressed by the fixing
  step". No model had spoken: the pull request touched only
  `openspec/changes/`, the fixer produced nothing, and `review-dispatch.yml`
  substituted `{"items":[]}` before the lander ever ran. A reader sees a machine
  that considered three findings and declined them.
- **The empty-report substitution stays**, because a missing file must not be a
  missing input — but the lander is told WHICH it got, so it can end the round
  honestly instead of inventing three refusals.
- **A disputed item and an unaddressed one need different endings.** A dispute
  waits for a person and blocks the archive, correctly. An unaddressed item is a
  defect in the run, and the next round should retry it rather than treat it as
  settled.
- **THIS CHANGE FIXES IT BECAUSE THIS CHANGE MAKES IT WORSE.** Under a bound of
  three it wasted three rounds; under five, driven unattended by
  `conveyor:run`, it wastes five and reports five considered refusals that never
  happened.

### D5. `autofix` is retired, not aliased

The label is renamed to `conveyor:fix` everywhere, and `autofix` is added to
`.github/retired-vocabulary.json`.

- **An alias would be a second name for one consent**, readable in two places
  and removable in one. The guard exists so a retired name cannot return
  quietly.
- **The cost is a flag day**: a pull request carrying `autofix` when this merges
  stops being driven until someone relabels it. Acceptable — the loop is
  per-pull-request and nothing is lost but a relabel.

### D6. Nothing is stored outside the labels

The line's state is the labels on the issue and the pull request. No store, no
file, no branch name convention.

- **Every station can therefore be entered by hand**, which is what makes the
  single-station labels more than a fallback.
- **A person reading the issue sees the whole line**: the standing instruction,
  which station it reached, and whether it stalled.

## Risks / Trade-offs

- **[A label starts a machine writing to a branch]** → unchanged from today: the
  gate reads write access from the platform, refusals are visible, and every
  pull request is reviewed and merged by a person.
- **[`conveyor:run` archives without a second look]** → that is what it means,
  and the archive pull request is still merged by a person. Someone who wants
  the pause uses the single-station labels.
- **[A carried label could be forged by anything with `issues: write`]** → the
  gate re-reads the originating issue rather than trusting the carry, so a label
  placed without a live `conveyor:run` behind it is refused exactly as
  `claude[bot]`'s was.
- **[Five rounds costs more than three]** → deliberately, and bounded; the
  summary names the spend and the extension is a person's decision.
- **[The flag day]** → one relabel per open pull request, on a mechanism whose
  only current user is this repository.
- **[The merge trigger fires on every merged pull request]** → it reads the body
  for `Refs #<n>` and the issue's labels before doing anything, so a pull
  request with neither is a no-op.

## Migration Plan

1. Create the five labels, with descriptions, alongside `autofix`.
2. Merge the tree changes. Both names work for nothing: the vocabulary file now
   says `conveyor:fix`, so `autofix` stops being read.
3. Relabel any open pull request that carried `autofix`.
4. Delete the `autofix` label once no open pull request carries it.

**Rollback:** revert the vocabulary file; the workflows read the label from it,
so the old name resumes working without touching either workflow.

## Open Questions

- Does the merge trigger need `pull_request_target` to read a fork's body? No —
  forks are refused by the gate already, and this repository's rules forbid that
  trigger outright. Stated so nobody reaches for it.
- Should `conveyor:run` on an issue that is ALREADY closed do anything? Proposed
  no: the line starts from an open issue, and a closed one is a decision to stop.
