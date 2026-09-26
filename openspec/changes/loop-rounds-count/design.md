## Context

The line was decided in the workflows' shell and in eleven scripts.

A rule such as "what stands for the archive station" existed as a grep in the
gate, a condition in the carry and a check in the fire.

Each fix of one copy left the others, and the ten contradictions in the proposal are the copies drifting.

## Goals / Non-Goals

**Goals:**

- Every decision of the line is made in one place, testable without a network.
- Every state and event of the stations and the loops is a table row, and a
  test walks the whole table.
- An adapter can only get I/O wrong, and each adapter has a suite for that.
- The loop cannot cycle on its own refusal, and the bound holds.

**Non-Goals:**

- Making a round finish faster. A round against 107 analysis issues still times
  out. It now counts.
- Any new label, marker or vocabulary entry.
- Replacing the platform's own gates (branch protection, conversation
  resolution).

## Decisions

- **`conveyor.py` is pure.** Facts arrive as frozen dataclasses (`Placer`,
  `Line`, `PullRequest`, `Trigger`) and a `Decision` leaves. No `gh`, no
  clock, no environment. Alternative rejected: keeping the decisions in each
  script and testing them there. That is the state the ten contradictions came
  from.
- **The tables are data.** `STATION_TABLE` and `LOOP_TABLE` map (state, event)
  to the next state, with `None` meaning the event does not move it. An event
  outside the table raises. `done` is terminal. `recover:*` is legal only from
  `running`. A test iterates every state against every event, so a new state
  cannot ship half wired.
- **The writer takes events.** `conveyor-state.py` reads the live label, asks
  the table and writes one edit or none. A caller cannot name a value, so it
  cannot put the line in a state the machine has no path to.
- **One grant rule.** `conveyor:run` stands for every station.
  `conveyor:archive` stands for the archive station and for the fix station of a
  pull request that says `Closes #<n>`. The carry, the gate and the fire all
  call `standing_grant`.
- **A completion is re-checked like a start.** A review or CI completion on a
  pull request whose fix label the workflow placed re-reads the grant and its
  placer. A refusal removes the label and ends the loop as stalled.
- **Rounds count by kind, in one function.** `ending` counts every round that
  ran a model and does not count `clean` or `failed`. A counted round at the
  cap ends the loop. `count_marked` is the one counter that the gate, the
  landing and the recovery all use.
- **The cap is a gate rule.** `gate` refuses to start a round at the ceiling
  unless `conveyor:keep-going` stands, and sends `end:capped`.
- **The guard splits by purpose.** `--purpose ci` asks the dispute question.
  `--purpose archive` also asks whether a round runs. The split follows the
  review-thread rule: a state the loop moves is never a check's question.
- **A failed check is work or it is not.** `check_is_work` reads the failed
  steps. A `docs-task` failed only by the guard step is `waiting`, and the
  round's ending names what is waited on.
- **Fail closed on unreadable facts.** An unreadable fire record or pull
  request list reads as "a session is at work", as one fact, so a rate limit
  never starts a second session.
- **The archive carry needs a finished change.** `is_finished` reads the bound
  change's tasks. A merge of a proposal or an applying pull request carries
  nothing.
- **A fire follows the stage.** `fire` picks the station from the change and
  the issue's labels, records once per station, and starts nothing while a pull
  request from the change is open.

## Risks / Trade-offs

- **A large rewrite of running automation.** Mitigated by the table walk, the
  combination tests for each decision and a suite per adapter, and by driving
  this pull request by hand, since it edits the loop that would drive it.
- **A timed-out round now spends the budget**, so a loop against a large work
  list stops after five with nothing landed. That is the bound working, and
  `conveyor:keep-going` grants more.
- **With the running-round question gone from CI**, `ci-green` can be green
  while a round is mid-push. Branch protection still requires the head to be up
  to date and every thread resolved, and the merge is a person's click.
