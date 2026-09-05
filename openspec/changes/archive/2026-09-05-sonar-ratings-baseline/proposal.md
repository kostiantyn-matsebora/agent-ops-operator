## Why

`coverage-across-packages` gave every component's project a gate condition on
OVERALL coverage, on purpose deferring ratings: "a rating condition on overall
code would turn every project red for findings a different change owns" — this
one. Today the `agentops` gate carries only the built-in `Sonar way`'s NEW-code
conditions (new reliability rating A, new security rating A, new
maintainability rating A) plus overall coverage. Nothing gates the EXISTING
backlog of Blocker and High findings sitting in code nobody's pull request is
touching, so a component can sit at an E reliability rating forever — no
condition ever asks. Before any finding is fixed, the target has to exist and
the backlog has to be visible, the same argument that shipped the coverage
condition.

## What Changes

- **Every Blocker and High severity finding open across every component is
  fixed**, in code, so every project's OVERALL reliability, security and
  maintainability ratings reach at least B. The Clean Code severity scale
  (Blocker, High, Medium, Low, Info), not the retired one — a finding's
  `impacts[]` name which rating it counts against.
- **The quality gate gains three overall-rating conditions**: reliability
  rating, security rating and maintainability rating on the whole component,
  each failing worse than B — provisioned the same way the coverage condition
  was, copied onto the same `agentops` gate `sonar-provision.sh` already
  maintains, idempotently.
- **The before-and-after is recorded as counts**, per component per rating,
  exactly as coverage-across-packages recorded coverage — so a number that
  drops from many findings to zero reads as measurement, not as an unverifiable
  claim.

## Capabilities

### New Capabilities

_None._

### Modified Capabilities

- `code-quality-analysis`: the gate every project is assigned also requires at
  least a B reliability, security and maintainability rating on the component
  as a whole, provisioned by the same deliberate step that creates the
  coverage condition.

## Impact

**Code:** `.github/scripts/sonar-provision.sh` (three more conditions in the
gate stage's `wanted` list); every component whose current rating is below B —
which component that is is not known until the organisation's findings are
read, so this proposal cannot name files yet; a script to enumerate open
Blocker/High findings per component (extending or run alongside
`.github/scripts/sonar-issues.py`, which today reads only a pull request's own
issues, never the branch backlog); a test for the extended provisioning
conditions under `.github/tests/`.

**Reference docs made untrue:** `openspec/specs/code-quality-analysis/spec.md`
(the delta folds in at archive); `.claude/rules/gotchas.md` or
`.claude/rules/worktree-delivery.md` if the finding sweep surfaces a technique
worth recording. `docs/CHANGELOG.md` is not touched: nothing here ships in the
chart or an image.

**Adopter site:** nothing — code-quality analysis is contributor-facing, same
as coverage-across-packages found. `CONTRIBUTING.md`, "Code analysis", gains
what the gate now additionally requires.
