# Proposal: role-agents

## Why

Every opsx session implements, reviews and documents a change as one generalist
context. The deployment-dashboard repository proved a role model — backend,
frontend, infrastructure, testing, contract — worth porting.

Its agents cannot be copied as they are. Each is an anchor into a ~90 KB
team-process apparatus with its own protocol, outbox and orchestrator, and none
of that belongs here.

This repository already has the binding corpus (`.claude/rules/`) and the
coordination medium (`tasks.md`). Porting the five roles as self-contained
agents gives each section of a change a specialist without importing a second
process.

## What Changes

- **Five role agents land in `.claude/agents/`** — `api-architect`,
  `backend-developer`, `deployment-engineer`, `frontend-developer`,
  `testing-specialist`. Each is self-contained: the role's substance rewritten
  to `writing.md`'s shape, its lane (the paths it owns), the rules files that
  are its binding, and a delimited review-criteria section. No team-process
  import — the hand-back is the ordinary Agent report, the opsx session is the
  sole integrator, agents never commit.
- **The tasks file names its roles.** `openspec/config.yaml` `rules.tasks`
  gains a rule: each implementation section names the fulfilling agent.
- **`/opsx:apply` dispatches per section** to the named agent and stays the
  integrator — it reads the report, ticks the tasks, commits on the change
  branch.
- **`/opsx:apply` gains a cross-role review step** before the pull request
  opens: the diff of each section is reviewed by a role other than the one
  that wrote it, findings resolved or recorded.
- **`/opsx:propose` and `/opsx:update` name `api-architect`** for
  contract-shaped artifacts — delta specs and the design's contract decisions.
- **The CI review's readers become role-aware.** The `reader-system` prefix
  gains the routed role's review criteria per component, selected the same way
  rule files are routed and restored from the base branch like every review
  asset. The `file-reviewer` reading contract, the per-job byte-identical
  prefix and the coordinator are unchanged.

## Capabilities

### New Capabilities

- `role-agents`: the five role agent definitions — what each owns, what binds
  it, how it hands back — and their dispatch: per-section at apply, contract
  artifacts at propose/update, cross-role review before the pull request.

### Modified Capabilities

- `automated-code-review`: the fixed system prefix of a component's file
  readers additionally carries the review criteria of the role routed to that
  component's paths, taken from the base branch.

## Impact

- **Code**: `.claude/agents/` (five new files), `openspec/config.yaml`,
  `.claude/commands/opsx/apply.md`, `.claude/commands/opsx/propose.md`,
  `.claude/commands/opsx/update.md`, `.github/scripts/review-rules.py` (or a
  sibling routing table), `.github/scripts/review-prompt.py`,
  `.github/workflows/claude-review.yml` (base-branch restore list),
  `.github/tests/` (the script suite rows that pin the routing and the
  prefix).
- **Documents made untrue, reference half**:

  | Document | Made untrue how |
  |---|---|
  | `CONTRIBUTING.md` | how a change is proposed and implemented here — sections now name their agent |
  | `.claude/rules/worktree-delivery.md` | its description of what a reader's system prefix holds |

- **Documents made untrue, adopter half**: none. The landing page,
  introduction, getting started, installation, the integration pages and the
  guides describe the product, and no product behaviour changes.
- **Dependencies**: none added. The dispatch uses the Agent tool the session
  already has, and the CI review keeps its `claude -p` shape.
