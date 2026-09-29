# Design: role-agents

## Context

See proposal.md — Why. The constraints that shape the approach:

- The source material is five 8-line anchors in deployment-dashboard, each
  pointing into that repository's team-process (roles, a 22 KB protocol, an
  outbox script) and a per-project bindings layer.
- A subagent dispatched interactively here arrives with `CLAUDE.md` and every
  unscoped rule already in context — measured 2026-08-26, recorded in
  `gotchas.md`.
- The CI review deliberately inverts that: no reading inherits a rule file,
  and `review-rules.py` routes rule text per path into a fixed, byte-identical
  per-job prefix (`review-prompt.py reader-system`).
- The review's assets are restored from the base branch before any model runs.
  A pull request may not rewrite the review that judges it.
- `writing.md` binds every markdown file, and the `rules_compliance` hook
  enforces it on write. The dd files as written do not pass it.

## Goals / Non-Goals

**Goals:**

- Five self-contained role agents whose substance survives the port and whose
  references all resolve inside this repository.
- One coordination medium. `tasks.md` and the opsx session carry everything
  the dd protocol carried.
- Role expertise reaches both review surfaces — the apply-time cross-review
  and the CI readers — from the same five files.

**Non-Goals:**

- No orchestrator role, typed forms, outbox or execution modes. The session
  is the orchestrator.
- No port of `docs-keeper`, `drawio-diagrammer` or the dd schemas.
- No change to the reading contract, `thread-verdict`, the coordinator, the
  queue, the coverage record or the fixing loop.
- No per-task dispatch. The unit is the section.

## Decisions

### The agent file is the role rewritten, not the anchor copied

Each agent is one file in `.claude/agents/`, in this shape:

| Part | Holds |
|---|---|
| frontmatter | `name`, `description`, `tools`, `model: inherit` |
| mission | the role's substance from dd's `team-process/roles/`, rewritten to `writing.md`'s shape |
| lane | the paths the role owns, from the spec's table |
| bindings | the rules files that are its criteria, NAMED — never restated |
| workflow | discover, implement in-lane, validate with the lane's own gates |
| hand-back | report changed files and open points, never commit — the session integrates |
| `## Review criteria` | the role's own bar for judging a diff, delimited for extraction |

`model: inherit` for the same reason the review roles carry it: the workflow
sets `--model` in CI, and an interactive dispatch inherits the session's.
Alternative — dd's `model: sonnet` — rejected: a hardcoded model is the
runner-has-no-settings lesson in reverse.

### Bindings are names, because the two dispatch contexts differ

Interactively, the unscoped rules arrive with the subagent for free, so
restating them in the agent file would be paid twice. In CI, rules are routed
separately by `review-rules.py`.

So the agent file NAMES its binding rules and states only what no rule
states — the role's own competencies and bar.

The `## Review criteria` section therefore must not restate rule content.
What it adds is what the rules corpus does not carry: the backend role's
code-smell table, the frontend role's accessibility and visual-fidelity bar,
the testing role's assertion-weakening watch, the contract role's
consistency-over-cleverness principles.

One adaptation is stated rather than inherited: dd's NO-MOCKS creed becomes
"the doubles under `test/` are deliberate and structural (`structure.md`) —
extend them, never replace them with mocks, and never 'fix' them into real
dependencies".

### CI: the role rides the prefix, the reading contract stays

`review-prompt.py reader-system` currently assembles: file-reviewer body,
routed rule text, delta specs. It gains one block between the role body and
the rules: the `## Review criteria` section of the role routed to the
component's paths.

- **Routing is a second table beside the rules table** — path patterns to at
  most one role each, union across the component's paths, deterministic
  order. It lives in `review-rules.py` (or a sibling read the same way) and
  gets the same `--check`: every role file it names exists.
- **Extraction is by heading.** The same frontmatter-strip the prompt
  assembler already does, then the `## Review criteria` section alone. The
  implementer's workflow and hand-back never reach a reader.
- **The prefix stays byte-identical per job**, so the cache property the
  no-inherited-rules requirement measured is untouched. The cost is one
  role's criteria (~2–4 KB) per component job, paid once.
- **The restore list grows.** The `read` job restores the five agent files
  from the base branch beside `file-reviewer.md` and `thread-verdict.md`.

Alternative — swap the reader's role file per component kind — rejected: the
file-reviewer body IS the reading contract (blindness, JSON shape, tool
bounds), and five variants of it would be five copies of that contract to
keep aligned.

### Apply: dispatch per section, integrate in the session

- The tasks rule (injected by `openspec/config.yaml`) makes each
  implementation section name its agent in the section heading.
- `apply.md` gains the dispatch step: one Agent call per section, the report
  read back, the session verifies, ticks and commits. Sections naming no
  agent are worked directly.
- Per-section, not per-task: each dispatch pays the full inherited-rules
  context, so the unit that keeps the cost at a handful of dispatches is the
  section.
- Remote sessions inherit this with no second wiring — the routine reads the
  same committed command files.

### Cross-review: one round, another role, before the pull request

After the implementation sections complete, `apply.md` dispatches one review
per section, one round, bounded. Findings are fixed on the branch or recorded
in the change with the reason they stand.

The reviewer is picked by lane: `api-architect` for any diff touching its
lane, otherwise `testing-specialist` for sections it did not write, otherwise
`backend-developer`.

Alternative — reuse the CI file-reviewer shape locally — rejected: the CI
review runs on the pull request anyway, and the apply-time pass exists to
catch what a role's bar sees before CI spends a matrix on it.

### Propose and update name the contract role

`propose.md` and `update.md` gain one instruction: deltas that change a CRD
field, an adapter contract or an HTTP endpoint are drafted by dispatching
`api-architect`, and the session integrates the artifact.

The dd insight this keeps: the agreed interface is a committed artifact,
never left as chat — which is already this repository's prototype rule
generalised.

## Risks / Trade-offs

- **Agent dispatch under `claude -p` is assumed, not measured** → settle it
  the way `gotchas.md` settles CLI facts: one local `claude -p` invoking a
  project agent, before the apply wiring lands. Fork agents are refused under
  `-p` — these are named project agents, a different mechanism.
- **Role criteria drift into rule restatement** → the bindings decision above
  is the line, and the cross-review of this change's own sections checks it.
  The extraction test pins the mechanism, not the prose.
- **The reader prefix grows for every component** → bounded by the criteria
  section's size, paid once per job by cache. If a role's section grows past
  a few KB, the fix is editing that section, not the mechanism.
- **In-flight changes have tasks files with unnamed sections** → the naming
  rule is injected at generation, so old tasks files stay valid and their
  sections are worked directly — the same grace `change-tests.md` records
  for its own shape rule.
- **A cross-review could stall a change on taste** → one round, and "recorded
  with the reason it stands" is a valid ending. The CI review and its human
  triage remain the authority.

## Migration Plan

None. The change is process and CI wiring, delivered as one ordinary pull
request on `change/role-agents`. Rollback is reverting it — no state, no
deploy, no data.
