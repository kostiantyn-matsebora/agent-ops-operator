# Tasks: role-agents

## 1. Settle the CLI fact the wiring rests on

- [x] 1.1 Verify locally that a named project agent under `.claude/agents/`
      dispatches from a `claude -p` session, the way `gotchas.md` settles CLI
      facts — one minute, no cluster. Record the answer in the change if it
      is no, before any wiring lands.

## 2. The five role agents

- [x] 2.1 Write `.claude/agents/api-architect.md` — contract role rewritten
      from dd's `roles/contract.md` to this repository's shape: lane
      (`platform/manager/api/v1alpha1/`, `openspec/specs/` deltas,
      `docs/contracts.md`), bindings named (`terminology`, `wiring`,
      `invariants`, `adapters`), workflow with `controller-gen` and
      `docs-generate.py --check` as the lane's gates, hand-back without
      commit, and a `## Review criteria` section. Verify the
      `rules_compliance` hook passes on write.
- [x] 2.2 Write `.claude/agents/backend-developer.md` — lane: the Go modules
      and the Node runtimes, gates from `build-test.md`, the code-smell table
      kept as its review criteria. Verify hook passes.
- [x] 2.3 Write `.claude/agents/deployment-engineer.md` — lane: `chart/`,
      `.github/workflows/`, `.github/scripts/`, gates: the chart render
      tests and `.github/tests/run.sh`. Verify hook passes.
- [x] 2.4 Write `.claude/agents/frontend-developer.md` — lane:
      `platform/console/ui/` and the docs site's shell, the visual-fidelity
      bar kept, `visual-check.md` named as the verification path. Verify
      hook passes.
- [x] 2.5 Write `.claude/agents/testing-specialist.md` — lane: the tiers
      `docs/testing.md` owns, with the stated adaptation: the doubles under
      `test/` are deliberate and structural, extended and never replaced.
      Verify hook passes.
- [x] 2.6 Verify every file each of the five names resolves in a clean
      checkout of this repository alone (`grep` the named paths, check each
      exists) — no dangling dd reference survives the port.

## 3. Openspec wiring

- [x] 3.1 Add the tasks rule to `openspec/config.yaml`: each implementation
      section names the fulfilling role agent in its heading, the three
      trailing sections exempt. Verify by generating a scratch change's
      tasks instructions and reading the injected rule back.
- [x] 3.2 Add the dispatch step to `.claude/commands/opsx/apply.md`: one
      Agent call per named section, the session verifies, ticks and commits,
      unnamed sections worked directly. Verify the command file reads
      coherently end to end and the hook passes.
- [x] 3.3 Add the cross-review step to `.claude/commands/opsx/apply.md`,
      after the implementation sections and before the pull request: one
      review per section by the lane-picked other role, findings fixed or
      recorded. Verify the reviewer-picking rule in the file matches
      design.md.
- [x] 3.4 Add the contract-role instruction to
      `.claude/commands/opsx/propose.md` and `update.md`: contract-shaped
      deltas drafted by `api-architect`, integrated by the session. Verify
      both files pass the hook.

## 4. CI review role routing

- [x] 4.1 Add the path-to-role table beside the rules table (in
      `review-rules.py` or a sibling it loads), union across a component's
      paths, deterministic order, `--check` asserting every named role file
      exists. Verify `--check` passes and a deliberate bad entry fails it.
- [x] 4.2 Extend `review-prompt.py reader-system` to append each routed
      role's `## Review criteria` section between the role body and the rule
      text, frontmatter stripped, heading-extracted. Verify by running
      `reader-system` on a fixture queue for a console path and a chart path
      and reading the block back.
- [x] 4.3 Add the five agent files to the `read` job's base-branch restore
      list in `claude-review.yml`. Verify the restore list and the routing
      table name the same five files.
- [x] 4.4 Confirm the verdict and coordinator prompts are untouched by the
      change — `verdict-system` and `coordinator` output byte-identical on a
      fixture before and after. Verify with a diff of both outputs.

## 5. Unit tests

- [x] 5.1 Extend the review script tests in `.github/tests/` to pin the
      routing: a console path routes frontend, a chart path routes
      deployment, a path with no role routes none, and `--check` fails on a
      missing role file. Verify `.github/tests/run.sh` is green.
- [x] 5.2 Add a test pinning `reader-system`'s role block: extraction takes
      the `## Review criteria` section alone, and a component with no routed
      role produces the prefix exactly as before. Verify with the suite run.

## 6. E2E tests

- [x] 6.1 Nothing here is decided by a cluster — the change is agent
      definitions, opsx command text and review prompt assembly, all
      exercised by the script suite and by CI's own run on this change's
      pull request. Ticked on that stated ground.

## 7. Documentation

- [x] 7.1 Reference half: update `CONTRIBUTING.md` (sections name their
      agent, apply dispatches and cross-reviews) and
      `.claude/rules/worktree-delivery.md` (the reader's system prefix now
      carries the routed role's criteria). Record the task-1 CLI fact in
      `.claude/rules/gotchas.md` if its answer cost anything. Verify the
      hook passes on every touched file.
- [x] 7.2 Adopter site: no page changes — the site describes the product and
      no product behaviour changed. Ticked on that stated ground after
      re-checking the landing page, introduction, getting started and
      installation mention nothing this change touched.
