## 1. The roles

- [ ] 1.1 `.claude/agents/component-reviewer.md`: the workflow's inline
  `--agents` definition moved verbatim — description, tools (`Read`, `Grep`,
  `Glob`, `Bash(git diff:*)`, `Bash(git log:*)`, `Bash(git show:*)`), the
  reading order and the RETURN JSON. Verify: `claude -p` from a checkout can
  name it (`Agent(component-reviewer)` in the allowlist) and it returns the
  JSON for one component of an open pull request
- [ ] 1.2 `.claude/agents/review-coordinator.md`: the current prompt's STEP 3
  and STEP 4 — consolidate, reach, first-finding rule, HOW TO REPORT, the two
  summary shapes — with the queue and spawn steps removed; tools
  `Bash(gh pr comment:*)`, `Bash(gh api:*)`, `Bash(git grep:*)`, `Read`,
  `Bash(.github/scripts/mark-thread-resolved.sh:*)`, the inline-comment MCP
  tool. Verify: given three reviewers' JSON by hand it posts one summary in
  the stated shape on a scratch pull request

## 2. The script

- [ ] 2.1 `.claude/workflows/review-pr.js`: `meta` with name `review-pr` and
  three phases (Queue, Read, Consolidate); phase 1 an agent returning
  `{queue, threads}` by schema from `gh pr diff --name-only`,
  `review-queue.py` and the thread query; phase 2 `pipeline(queue, …)` with
  `agentType: 'component-reviewer'`, `label: entry.group`, and the FINDINGS
  schema; phase 3 the coordinator agent handed every result and the threads;
  `log()` of queue length, reviewed and unreviewed counts; `return` the
  outcome. Verify: `node --check` parses it and `/review-pr <n>` from a
  checkout runs end to end on an open pull request
- [ ] 2.2 The FINDINGS schema refuses a prose return: verify by reviewing a
  component with a deliberately broken reviewer prompt locally and reading the
  `null` → `unreviewed: <group>` path in the summary

## 3. The workflow file

- [ ] 3.1 `claude-review.yml`: before the action step, `git checkout
  "origin/${{ github.base_ref }}" -- .claude/workflows/review-pr.js
  .claude/agents/component-reviewer.md .claude/agents/review-coordinator.md`,
  with a `::notice` naming any of them the pull request changed; the
  `prompt:` becomes the one-line `/review-pr` invocation with number and base;
  `--agents`, the tracker checklist and the waiting instructions are removed;
  `--allowedTools` gains `Workflow` and the two agent names. Verify:
  `python3 -c "import yaml; yaml.safe_load(open(...))"` and a diff that removes
  more lines than it adds
- [ ] 3.2 The "review actually ran" gate: keep the execution-file and
  summary-count checks (#78's marker), and fail when the workflow's return is
  absent. Verify by reading the gate against the three outcomes (returned with
  summary, returned without, no execution file)
- [ ] 3.3 `.github/scripts/serviceaccount-guard.py`-style dry read is not
  applicable; instead run the whole file through `actionlint` if available,
  else `yamllint`. Verify: no error

## 4. The first run, measured

- [ ] 4.1 Merge, then on the next pull request read the run: the `/workflows`
  record's per-agent timing from the execution artifact, the effective
  concurrent pool on `ubuntu-latest`, the summary on the pull request, the
  gate green. Record the pool size and the wall time here, beside #77's ten
  minutes
- [ ] 4.2 If the pool measured 2: open a follow-up naming the larger runner
  and its cost, rather than changing `runs-on` in this change

## 5. Documentation

Both halves, ticked separately, and this is the last section.

**Reference docs:**

- [ ] 5.1 `.claude/rules/worktree-delivery.md` "THE REVIEW FOUND SOMETHING":
  the review is a saved workflow with two roles, runnable as `/review-pr <n>`
  from a checkout; the guard is the base-branch restore
- [ ] 5.2 `.claude/rules/gotchas.md`: the two subagent-under-`-p` entries
  become the record of why the plan is a script (keep the measurements)
- [ ] 5.3 `CONTRIBUTING.md`: how a change is reviewed, and that the review
  runs locally; `openspec/specs` is updated by the archive
- [ ] 5.4 `docs/CHANGELOG.md`: no entry — nothing an adopter installs changes;
  state that here as the check having been made

**The adopter site:**

- [ ] 5.5 Verified again at the end: no page under `docs/` describes the
  review workflow (`grep -rn "claude-review\|component-reviewer" docs/*.md
  docs/guides/*.md` returns nothing), so the site is unchanged
