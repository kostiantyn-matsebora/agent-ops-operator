## Context

See `proposal.md` — Why, for the naming argument. This document covers how
the rename is carried out without a window where CI, the release gate or the
docs-task guard disagree about what the pack is called.

The pack's identity is load-bearing in several places that must change
together, not independently:

- the Go package path and its build tag, read by `go test -tags`
- three workflow files, one of them `workflow_call`-reused by the other two
  and by `release.yml`
- five `E2E_*` environment variables read by the pack's own test code
- a check-run name (`smoke-evidence.py`'s `SUFFIX`) computed from the
  workflow's displayed `name:` field, not from the file name
- the openspec task-section regex (`docs-task-guard.py`) that every future
  change's `tasks.md` is validated against

## Goals / Non-Goals

**Goals:**
- One consistent name, "system", for the pipeline-run real-cluster pack,
  across code, CI and process tooling.
- No behavior change: the same assertions, the same tiers, the same gating.
- No window in which `smoke-evidence.py`'s check-run lookup or the release
  gate silently stops matching because one half renamed and the other has
  not yet.

**Non-Goals:**
- Renaming `platform/console/ui/e2e/` or `e2e-live/`. Neither runs in a
  pipeline, so neither is this misnomer.
- Splitting the real-runtime lane out of `full` into its own named tier.
  That lane is arguably the one that earns "end-to-end," but naming it is a
  separate decision with its own tradeoffs, not a side effect of this rename.
- Touching other in-flight openspec changes' own `tasks.md` files. They are
  other sessions' active work.
- Renaming any openspec capability's directory path.

## Decisions

### "system" over "cluster" or "substrate"

All three are accurate. "system" was the user's own call after reviewing the
pyramid classification: unit → integration → **system** → acceptance/e2e.

It reads naturally beside the existing tier names ("unit", "contract
conformance", "system") without introducing a new compound word into every
file that already says "the pack".

### Capability and requirement identifiers are NOT renamed

`openspec instructions specs` is explicit: "Do not move or rename the
capability." `openspec/specs/end-to-end-testing/` keeps its path, the same
way `channel-type-model` kept its name through the `type`→`adapter` field
rename.

Requirement HEADER text, unlike the capability path, validates cleanly when
changed inside a MODIFIED block. `openspec validate` accepted "The end-to-end
pack runs against a real single-node cluster" becoming "The system pack
runs against a real single-node cluster".

Scenario TITLES do not: the validator diffs scenario titles against the
existing requirement and refuses a MODIFIED block that drops one.

One scenario in `continuous-integration` therefore has its title fixed by a
direct edit to `openspec/specs/continuous-integration/spec.md` in THIS change
(task 4.3), before the delta is validated. The delta then carries the new
title, "A per-module job is not duplicated in the system workflow", with its
body saying `system.yml`.

The direct edit follows the way the `vm-alertmanager-signal-adapter` →
`alertmanager-signal-adapter` capability rename was done, back in chart
5.24.0.

### Order of operations

Everything that names the pack by its old identity changes in ONE commit, not
a sequence of commits that each leave CI referencing a mix of old and new
names.

The specific risk: `smoke-evidence.py`'s check-run suffix is computed from
the WORKFLOW's `name:` field, read at a DIFFERENT time (release) than the
workflow file is edited.

A rename that updates the workflow's `name:` without updating the suffix
the lookup script matches breaks the release smoke-reuse logic silently —
exactly the kind of gap `gotchas.md` already warns about for
`workflow_run.name` matching.

Sequencing within that one change:

1. `git mv platform/manager/test/e2e platform/manager/test/system`, then
   `-tags e2e` → `-tags system` inside every moved file.
2. Rename the three workflow files, their `name:` fields, and every
   `E2E_*` env var they set or read.
3. Update every caller: `ci.yml`'s path filter and job name, `release.yml`'s
   `uses:` and the commit it looks up, `e2e-report.py` → `system-report.py`
   and its CI step, `smoke-evidence.py`'s `SUFFIX` and its test.
4. Update the openspec task-section convention (`openspec/config.yaml`,
   `docs-task-guard.py`'s regex and label strings, its test fixtures,
   `.claude/hooks/require-docs-task.sh`) so the NEXT change's generated
   `tasks.md` asks for "System tests" rather than "E2E tests".
5. Update `docs/testing.md`, `README.md`'s badge, and the rule files that
   describe the pack by name (`build-test.md`, `change-tests.md`,
   `documentation.md`, `remote-session.md`, `structure.md`, `CLAUDE.md`,
   `CONTRIBUTING.md`, `.github/routines/implement-issue.md`).
6. Apply the four capability deltas from `specs/`.

### `gotchas.md` keeps its historical names

`gotchas.md` narrates incidents by the file and workflow names that existed
when they happened (`e2e.yml`, `e2e-smoke.yml`, the `smoke / e2e / smoke`
check-run name).

Editing those to say "system" would describe an incident that did not
happen under that name — the same principle that keeps
`retired-vocabulary.md`'s own withdrawn-rule history intact. Only
forward-looking statements change.

### Retired-vocabulary entries

`.github/retired-vocabulary.json` gets entries for the specific identifiers
being retired: `test/e2e/` as this pack's path, `-tags e2e`, the three old
workflow file names, and the five `E2E_*` env vars.

NOT the bare word "e2e", which stays legitimate for `e2e-live` and ordinary
English use. This is a tasks.md line item, not a design risk — a pattern
too broad would fail the guard on every legitimate remaining "e2e" mention.

## Risks / Trade-offs

**[Risk] A partial rename leaves `ci.yml` or `release.yml` pointing at a
workflow file that no longer exists**, silently skipping the job it names.
→ Mitigation: one commit covers every caller, and
`continuous-integration`'s render/lint tier catches a malformed workflow
file before it reaches a pull request.

**[Risk] `smoke-evidence.py`'s check-run suffix lookup goes stale** for one
release cycle if the workflow `name:` change and the script's `SUFFIX`
change land in different commits.
→ Mitigation: both land in the same commit (step 3 above), and
`.github/tests/smoke-evidence.test.sh` pins the new suffix in the same
change.

**[Risk] A leftover scenario title in `continuous-integration`'s spec**
("... e2e workflow") would read as an incomplete rename.
→ Mitigation: task 4.3 retitles it to "system workflow" directly in this
change, so nothing is deferred until after archive.

**[Risk] This change's own `tasks.md` could fail the renamed guard**, since
`docs-task-guard.py` stops accepting "E2E tests".
→ Mitigation: its second trailing section is titled "System tests", so the
guard accepts it.
