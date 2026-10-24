## Why

`coverage-across-packages` made coverage attribution honest and put an 80%
overall-coverage condition on the `agentops` quality gate every component's
project is assigned. Not one of the sixteen components clears it today —
`signal-cron` sits at 26.6%, `housekeeping` at 39.4%, and the org median is in
the high 60s/low 70s. The gate is red on every dashboard for a reason nobody
has yet acted on: the condition made the gap visible, and this change closes
it.

## What Changes

- **Every component's tests are extended until its overall coverage reaches
  at least 80%**, worked worst-first: `signal-cron`, `housekeeping`, `scripts`
  (gate-exempt per `sonar-ratings-baseline`, covered anyway since it is still
  shipped tooling), `signal-telegram`, `gateway-telegram`, `runtime-claude`,
  `egress-proxy`, `context-sync`, `runtime-ollama`, `manager`,
  `channel-telegram`, `runtime-copilot`, `signal-alertmanager`, `signal-ha`,
  `console`, `signal-k8s-events`.
- **Every new test exercises a REAL gap** — an untested error path, an
  untested branch, an untested helper — never a trivial assertion written
  only to move a number. A component whose remaining uncovered lines are
  dead code or generated code says so and removes or excludes the line
  instead of testing around it.
- **No behavior changes.** This is test-only work; no CRD field, no HTTP
  contract, no CLI flag and no chart value changes as a result. `skip_specs`
  is set on this change for that reason.
- **The before-and-after coverage is recorded as counts**, per component, the
  same way `coverage-across-packages` recorded its baseline — so "80%" reads
  as measured, not claimed.

## Capabilities

### New Capabilities

_None._

### Modified Capabilities

_None — this change adds tests against existing behavior. No spec describes a
coverage percentage, so no spec-level requirement changes._

## Impact

**Code:** test files across every module listed above; no production code
change except where a genuinely dead or unreachable line is deleted rather
than tested, or where a Sonar/coverage-tool exclusion comment is the correct
disposition for generated code.

**Reference docs made untrue:** none — `docs/concepts.md`, `docs/contracts.md`
and every bundle page describe behavior, and none of this changes behavior.

**Adopter site:** nothing — coverage is contributor-facing, exactly as
`coverage-across-packages` found. `CONTRIBUTING.md`, "Code analysis", already
states the 80% threshold and that a component under it is expected to be red;
no wording there becomes false, so it is not touched unless a component's
path to 80% needs a documented exception (e.g., an exclusion pattern), in
which case that section gains the exception and nothing else.

**`docs/CHANGELOG.md`:** not touched — nothing here ships in the chart or an
image.
