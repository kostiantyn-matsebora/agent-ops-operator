## Context

See proposal.md — Why. What shapes the approach:

- **Two Go test steps produce coverage**: `operator` (the manager, with its
  envtest suite) and `modules (<path>)` (every other Go module), both
  `go test -count=1 -coverprofile=coverage.out ./...`, and
  `.github/actions/sonar-scan` reads `coverage.out` from the module's
  directory as the job's last step. Node coverage (`node --test`, vitest) is
  per file already and is not the problem.
- **The org holds one gate**, the built-in `Sonar way` (default): new
  coverage ≥ 80%, new ratings A, new duplication < 3%, new hotspots reviewed.
  Every condition is on new code. Overall coverage is not a condition
  anywhere.
- **Projects are provisioned by `sonar-provision.sh`** through the web app's
  monorepo call, idempotently. Quality gates, unlike projects, have a public
  API: `api/qualitygates/create`, `create_condition`, `set_as_default`,
  `select`.
- **The gate's verdict is deliberately not a required check** (the published
  spec: "a failed quality gate does not fail the pull request"). Requiring it
  is `autofix-review-loop`'s step, and that change is in flight.

## Goals / Non-Goals

**Goals:**

- The coverage number per component is what the tests actually cover.
- "Passes the gate" means ≥ 80% coverage of the whole component, for every
  project, set by a repeatable step and not by hand in a dashboard.
- Nothing about what fails a pull request changes.

**Non-Goals:**

- Writing tests. That is the per-component changes that follow, against
  the corrected number.
- Requiring the gate in branch protection.
- Any change to what is excluded from analysis or how tests are classified.

## Decisions

### D1. `-coverpkg=./...` on both Go test steps

`go test -count=1 -coverpkg=./... -coverprofile=coverage.out ./...`.

- `-coverpkg` names the packages whose coverage is RECORDED; `./...` means
  every package of the module, whichever test binary exercised it. Without
  it, a test binary records its own package only — which is what made the
  envtest suite's exercise of `internal/controller` count for nothing.
- **A package no test reaches is now reported at 0% rather than absent.**
  That is the honest number: SonarCloud already counted a file absent from
  the profile as uncovered, so the dashboard does not move for those; the
  local `go tool cover` summary does, and that is the point.
- **Cost:** each test binary is compiled with instrumentation for the whole
  module, so the test step is slower — measured on merge, recorded in the
  tasks. The `-count=1` reasoning is unchanged.
- **Alternative rejected:** `-coverpkg` on the integration package only.
  The other modules' tests are single-package today, so the flag costs them
  nothing and stops being a special case the manager alone carries.

### D2. The gate is provisioned, not clicked

`sonar-provision.sh` gains a second stage after the projects:

1. `api/qualitygates/list` — find the gate named `agentops`; create it with
   `api/qualitygates/create` when absent.
2. Ensure its conditions with `api/qualitygates/create_condition` (an
   existing condition on the same metric is updated with `update_condition`
   rather than duplicated): the six of `Sonar way`, verbatim, plus
   `coverage LT 80`.
3. `api/qualitygates/set_as_default`, and `api/qualitygates/select` for
   every component project from the same `components.sh` list the projects
   came from.

- **Why a copy of `Sonar way` rather than editing it:** the built-in gate is
  read-only. A copy with one more condition keeps every new-code condition a
  contributor already sees.
- **Why overall coverage and not overall ratings too:** the ask is coverage.
  A rating condition on overall code would turn every project red for
  findings a different change owns (`sonar-ratings-baseline`), and one gate
  condition per change keeps the red readable.
- **Idempotent by lookup**, exactly as the projects stage: run twice, the
  second run creates nothing and reports the gate assigned.
- **The script is the record.** A gate set in the dashboard is state nobody
  can read from the repository; a gate the script creates is one `git show`
  away, and re-provisioning a fresh organisation reproduces it.

### D3. The verdict stays informational, and the red is the feature

Every component under 80% turns red on SonarCloud's own check the moment the
gate is assigned. Branch protection does not read it (unchanged), so no pull
request is blocked. `CONTRIBUTING.md`'s argument — "a gate that goes red for
whoever opens the next unrelated pull request is one somebody switches off"
— was about requiring it before the tree was measured; this change is the
measuring. The change that requires it comes after the coverage changes that
make it green.

## Risks / Trade-offs

- [`-coverpkg=./...` inflates a module's test build time] → measured on the
  first CI run of the branch; the manager is the only module big enough to
  notice, and its job already runs envtest for minutes.
- [Provisioning needs a user token with *Administer Quality Gates*] → the
  same token `sonar-provision.sh` already requires for projects; the script
  says so in its header and fails naming the permission on a 403.
- [The gate API's condition set drifts from `Sonar way`'s over time] → the
  script copies the conditions by NAME from the built-in gate's `show`
  response rather than hard-coding them, so a re-run tracks upstream.
- [A number that jumps from 27% to something higher reads as tests written]
  → the tasks record before and after per component as counts, and the
  commit message says it is measurement.

## Open Questions

None.
