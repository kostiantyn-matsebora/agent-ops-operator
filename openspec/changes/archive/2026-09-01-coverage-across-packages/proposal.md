## Why

The coverage SonarCloud shows for the operator is 27%, and the number is
mostly wrong: `go test -coverprofile` attributes coverage only to the package
under test, so the forty-four envtest files in `internal/integration/` that
drive the controller, the HTTP API, the chat core and the pod builder count
for none of them. And nothing measures the tree against a coverage target at
all — the org's only quality gate is the built-in one, which judges NEW code
only, so a component at 27% and one at 79% pass it identically. Before any
test is written to reach 80%, the number has to be true and the target has
to exist.

## What Changes

- **Coverage counts every package a test exercises.** The Go test steps in
  CI run with `-coverpkg=./...`, so a package exercised by another package's
  tests reports the coverage it actually has. The local recipe in the rules
  file says the same flags.
- **The quality gate requires 80% coverage of the WHOLE component.** The
  provisioning step creates the organisation's gate — the built-in new-code
  conditions plus overall coverage at or above 80% — sets it as the default
  and assigns it to every component's project, idempotently. The verdict
  stays SonarCloud's own check, NOT one branch protection requires; that
  requirement is unchanged and stays with the change that gates it. What
  changes is what "pass" means: a component under 80% is red on its own
  dashboard and on every pull request that touches it, which is the backlog
  made visible rather than a switch anybody has to remember.
- **The before-and-after is recorded as counts** in this change's tasks, per
  component, so the next changes — the tests that bring each component to
  80% — start from a measured gap.

## Capabilities

### New Capabilities

_None._

### Modified Capabilities

- `code-quality-analysis`: coverage is attributed to every package a test
  exercises, not only the package under test; the gate every project is
  assigned requires 80% coverage of the component as a whole, provisioned by
  the same deliberate step that creates the projects.

## Impact

**Code:** `.github/workflows/ci.yml` (the `operator` and `modules` test
steps gain `-coverpkg=./...`); `.github/scripts/sonar-provision.sh` (creates
the gate and its conditions, sets it default, assigns it per project — the
public quality-gate API, unlike the monorepo call above it); a test for the
provisioning script under `.github/tests/` with `curl` stubbed.

**Reference docs made untrue:** `openspec/specs/code-quality-analysis/spec.md`
(the delta folds in at archive); `.claude/rules/build-test.md`, "Coverage,
with the flags CI uses" (the Go row's command). `docs/CHANGELOG.md` is not
touched: nothing here ships in the chart or an image.

**Adopter site:** nothing — coverage measurement is contributor-facing and
no page under `docs/` describes it. `CONTRIBUTING.md`, "Code analysis", is
the contributor's page and is updated: "what does not fail your pull
request" keeps its answer, and gains what the gate now requires and why a
component's check is red until it reaches 80%.
