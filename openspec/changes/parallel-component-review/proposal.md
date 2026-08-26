## Why

**Every review is slow, and the time is one step.** Three real runs of
`claude-review.yml` broke down the same way: checkout 2s, hygiene guards 5s,
upload 1s — and the model step 126s, 351s, 415s. Nothing else is worth
optimising.

The model step is slow because ONE context reads EVERYTHING, serially, on every
push: nine rule files whether or not the diff touches their subject, the whole
pull request's diff, and every standing thread. The cost is the size of the
pull request, not the size of the push.

**And a single serial reading misses the failure this repository is built to
have.** Every module is stdlib-only and imports nothing from the manager, so a
change to a contract — an HTTP field, an env var name, a CR field — compiles in
every module, passes every module's tests, and breaks at runtime in a component
the diff never names. The contract file reads as correct because it IS correct;
it is just no longer what its consumers speak. A reviewer reading the diff sees
the contract change and not the reach.

## What Changes

- **ONE REVIEWER PER CHANGED COMPONENT, IN A CLEAN CONTEXT, IN PARALLEL.** The
  main context builds a queue from the diff grouped by component, and spawns a
  read-only subagent per entry. Each reviewer sees its component's diff, the
  rules for its path and its standing threads — and nothing from any other
  component. Wall clock becomes the slowest component, not the sum.
- **REVIEWERS RETURN DATA AND POST NOTHING.** `findings[]`, `changedNames[]`
  (the identifiers, fields, paths and env vars the diff added, removed or
  renamed) and `threads[{id, verdict}]`. Only the main context writes to the
  pull request, so the one-summary and no-repeat rules keep one enforcement
  point.
- **CONSOLIDATION IS A SECOND READING, ACROSS COMPONENTS.** The main context
  dedups findings, then resolves the CONSUMERS of every changed name
  mechanically (`git grep`) and checks whether each still holds. That is the
  contract case: the reviewer of `docs/contracts.md` reports `threadId` changed;
  the consolidator finds the seven files that speak it.
- **THE REVIEWER DEFINITION LIVES INSIDE `claude-review.yml`**, passed inline
  through `--agents`, not as a checked-in `.claude/agents/*.md`. The action
  refuses to run a workflow file that differs from the default branch's — so a
  reviewer defined there is guarded by the same rule that guards the review,
  and a pull request cannot rewrite the thing that judges it.
- **NO REVIEW STATE IS ADDED.** No last-reviewed marker, no incremental diff.
  The speed comes from parallelism and routed rules; the published rule that
  the review keeps no record survives intact. Incremental review is a later
  change if repeat runs still hurt after this one.
- **A SPIKE COMES FIRST**, on a throwaway branch: that `Agent(component-reviewer)`
  spawns under the action's allowlist, that reviewers run in parallel there,
  and what a reviewer's context actually contains — in particular whether
  `.claude/rules/` is auto-loaded into a subagent, which decides whether rule
  routing is free or has to be done by naming files in the delegation message.

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

- `automated-code-review`: gains three requirements — a change is reviewed per
  component in isolation; the review is consolidated across components, with
  the consumers of every changed name checked; and the reviewer's definition is
  part of the guarded review file. *A finding already made is not made again*
  and *What may close a thread holds no power to change the code* are
  unchanged: consolidation is where both are enforced, in one context.

## Impact

**Code**

- `.github/workflows/claude-review.yml` — the prompt becomes the consolidator's;
  `claude_args` gains `Agent(component-reviewer)` and an inline `--agents`
  definition. The two-job split, the always-ran guard and the artifact are
  untouched.
- `.github/scripts/` — possibly one small program that groups a diff's paths by
  component (`components.sh` already knows the list) and one that resolves
  consumers of changed names; both model-free, both tested in `.github/tests/`.
- `.github/tests/review-dispatch.test.sh` gains nothing; a sibling suite pins
  the review workflow's shape if a program is added.

**Reference docs**

- `.claude/rules/` — nothing states how the review is structured today;
  `worktree-delivery.md`'s "the review found something" section is unaffected.
  If the spike settles a fact about subagents under the action, it goes in
  `gotchas.md`.
- `CONTRIBUTING.md` — the *Pull requests* paragraph describing the review
  gains one clause: findings are per component, and the summary carries the
  reach the review checked.

**The adopter site**

- Nothing. No page under `docs/` describes contribution or review; the reader
  unaffected is the adopter, and nothing here reaches a cluster, a chart value
  or a CRD.

**Not affected**

- `docs/CHANGELOG.md`, every Go module, the chart, the published images,
  `review-dispatch.yml`.
