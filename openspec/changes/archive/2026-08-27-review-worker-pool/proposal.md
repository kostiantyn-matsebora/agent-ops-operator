## Why

The review's fan-out is decided turn by turn by the consolidating model, and
its execution artifacts show what that costs: on #74 it spawned three
background subagents, wrote "I'll wait for them", ended its turn — and the run
reported `success` with nothing posted; on #77 it spawned nine, one per
message, and spent seven of ten minutes in `sleep 60` until they returned.
Agent teams cannot help: `claude -p` does not spawn teammates. A dynamic
workflow can: it runs under `-p`, and it moves the plan out of the model's
turn and into a script the runtime executes.

## What Changes

- **The fan-out becomes a saved dynamic workflow**, `.claude/workflows/review-pr.js`,
  invoked by the review job as `/review-pr <number>`. The script builds the
  component queue, runs one `component-reviewer` agent per component through
  `pipeline()` — concurrent, bounded by the runtime's agent cap, each returning
  findings as schema-validated data — and hands the whole set to one
  `review-coordinator` agent that resolves reach, dedups against the standing
  threads, and posts the inline findings and the one summary. The script holds
  the loop; no model decides what to spawn next, waits on anything, or can end
  a turn with work in flight.
- **Two roles, two agent definitions, two files:** `.claude/agents/component-reviewer.md`
  and `.claude/agents/review-coordinator.md`, each the whole of its role's
  instructions and tool allowlist. The workflow's own `--agents` inline
  definition and its consolidator prompt are removed.
- **The guard moves with them:** before the run, the review job restores the
  workflow script and both agent files **from the base branch**, so a pull
  request cannot rewrite the review that judges it. A pull request that edits
  any of the three is reviewed by the base's copy and told so.
- **The "review actually ran" gate** asserts the run's outcome mechanically:
  the workflow returned, and the coordinator's summary is on the pull request.
  A review that stops short is red.
- **Removed:** the consolidator's tracker checklist, the `sleep` waiting
  instructions, the inline reviewer definition.

Not **BREAKING**: the triage vocabulary, the accepted-findings dispatch, the
thread reconciliation and the summary shapes are unchanged.

## Capabilities

### New Capabilities

_none_

### Modified Capabilities

- `automated-code-review`:
  - *A change is reviewed per component, each in isolation* — the concurrent
    readings are agents a WORKFLOW SCRIPT runs, bounded by the runtime's
    concurrency, each in its own context returning data; isolation per
    component is kept.
  - *The review is consolidated across components, on what changed* — the
    consolidation is an agent the script hands every reading's data to; it
    never waits on a running reader and cannot lose one.
  - *The reviewer's definition is part of the guarded review* — the roles'
    definitions and the workflow script are files restored from the BASE branch
    before the run; the guard moves from "inside the workflow file" to "the
    base branch's copy", and a pull request editing any of them is reviewed by
    the base's copy.

## Impact

- **New `.claude/workflows/review-pr.js`** — the orchestration: queue, fan-out
  with a findings schema, consolidation, `log()` of counts per phase.
- **New `.claude/agents/component-reviewer.md`, `.claude/agents/review-coordinator.md`**
  — the role definitions, moved out of the workflow's `--agents` and `prompt:`.
  Usable locally too: `/review-pr <n>` from a checkout.
- **`.github/workflows/claude-review.yml`** — restores the three files from the
  base ref, invokes `/review-pr`, and gates on the summary; loses the inline
  definition, the tracker, and the waiting instructions.
- **`.github/scripts/review-queue.py`** — unchanged; the script's first agent
  runs it.
- **Reference docs:** `.claude/rules/worktree-delivery.md` ("THE REVIEW FOUND
  SOMETHING"), `.claude/rules/gotchas.md` (the subagent-under-`-p` entries
  become the record of why the plan is a script), `CONTRIBUTING.md` (how a
  change is reviewed, and that the review is runnable locally). No CHANGELOG
  entry: nothing an adopter installs changes.
- **Adopter site:** no site page describes the review — checked: the landing
  page, Introduction, Getting started, Installation, the guides and
  `docs/security.md` name no review workflow — so nothing on the site changes.
- **Measured before relied on:** the runtime caps concurrency by the runner's
  CPUs; the first run on `ubuntu-latest` records the effective pool, and a
  larger runner is a one-line change if it is 2.
