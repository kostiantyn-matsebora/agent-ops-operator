# role-agents Specification (delta)

## Purpose

Five role agents — api-architect, backend-developer, deployment-engineer,
frontend-developer, testing-specialist — give each part of a change a
specialist: dispatched per section at apply, consulted for contract artifacts
at propose, and reviewing each other's sections before the pull request opens.

## ADDED Requirements

### Requirement: A role agent is self-contained and bound to this repository's rules

Each of the five role agents SHALL be one definition file under
`.claude/agents/`, complete in itself: dispatching it MUST NOT require any
file this repository does not carry.

Each definition SHALL state the role's lane — the paths it owns — and SHALL
name the rules files that are its binding, never restating their content.
Each SHALL carry a delimited review-criteria section usable as review
context on its own.

| Agent | Lane |
|---|---|
| `api-architect` | `platform/manager/api/v1alpha1/`, `openspec/specs/` deltas, `docs/contracts.md` |
| `backend-developer` | the Go modules and the Node runtimes |
| `deployment-engineer` | `chart/`, `.github/workflows/`, `.github/scripts/` |
| `frontend-developer` | `platform/console/ui/`, the docs site's shell |
| `testing-specialist` | the test tiers `docs/testing.md` owns, and the doubles under `test/` |

#### Scenario: A role agent is dispatched in a fresh checkout

- **WHEN** a role agent is dispatched in a working copy of this repository
  alone
- **THEN** every file its definition names resolves, and the agent needs no
  team-process, protocol or binding file from another repository

#### Scenario: A role agent finishes its work

- **WHEN** a role agent completes the work it was dispatched
- **THEN** it hands back a report of what it changed and what it could not
  settle, and it has made no commit — the dispatching session is the sole
  integrator

### Requirement: A tasks file names the agent that fulfils each implementation section

The rules injected when a tasks file is generated SHALL require each
implementation section to name the role agent that fulfils it. The three
trailing sections — unit tests, e2e tests, documentation — keep their shape
and are not required to name one.

#### Scenario: A tasks file is generated

- **WHEN** a change's tasks file is generated
- **THEN** each implementation section names exactly one of the five role
  agents, and a section no role fits names none and is worked by the session
  itself

### Requirement: Apply dispatches each implementation section to its named agent

The apply workflow SHALL dispatch each implementation section to the agent
the tasks file names, one dispatch per section.

The session SHALL remain the integrator: it reads the report, verifies the
work, ticks the tasks and commits on the change branch.

#### Scenario: A section names an agent

- **WHEN** apply reaches an implementation section naming a role agent
- **THEN** that agent produces the section's edits in the change's working
  copy, and the session — not the agent — ticks the tasks and commits

#### Scenario: A section names no agent

- **WHEN** apply reaches a section naming no agent
- **THEN** the session works it directly, as before this capability existed

### Requirement: Sections are cross-reviewed by another role before the pull request opens

After the implementation sections of a change are complete and before its
pull request opens, the apply workflow SHALL have each section's diff
reviewed by a role agent other than the one that wrote it.

Every finding SHALL be fixed or recorded in the change before the pull
request opens — none is silently dropped.

#### Scenario: A cross-review finds a problem

- **WHEN** the reviewing role reports a finding on a section's diff
- **THEN** the finding is fixed on the change branch, or recorded in the
  change with the reason it stands, before the pull request opens

#### Scenario: A cross-review finds nothing

- **WHEN** the reviewing role reports no finding
- **THEN** the pull request proceeds, and the review's happening is visible
  in the session rather than recorded as an artifact

### Requirement: Contract-shaped artifacts are drafted with the contract role

The propose and update workflows SHALL name `api-architect` as the role for
contract-shaped artifacts — delta specs that change a CRD field, an adapter
contract or an HTTP endpoint, and the design's contract decisions.

#### Scenario: A proposal changes a CRD field

- **WHEN** a change's deltas touch a CRD field, an adapter contract or an
  HTTP endpoint
- **THEN** the workflow dispatches `api-architect` to draft those deltas, and
  the session integrates its artifact as it integrates any section

#### Scenario: A proposal touches no contract

- **WHEN** a change's deltas touch no CRD field, adapter contract or endpoint
- **THEN** no contract dispatch happens
