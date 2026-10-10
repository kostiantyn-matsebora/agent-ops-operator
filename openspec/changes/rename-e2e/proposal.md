## Why

The pipeline-run cluster pack — `platform/manager/test/e2e/`, build tag `e2e`,
workflows `e2e.yml`/`e2e-smoke.yml`/`e2e-full.yml` — is misnamed.

By default (the `smoke` tier, the one that gates every release and runs
on-demand) it runs against a **stub** runtime. Nothing in the
signal-to-reply path is faked except the one thing a true end-to-end test
would never fake: the agent actually doing the work.

What it genuinely proves is that the operator's own machinery integrates
correctly with a REAL Kubernetes cluster — credential projection by the
kubelet, RBAC as the live authorizer enforces it, informer liveness,
admission FIFO on a real pod DELETE.

That is **system-level testing against a real cluster substrate**, not an
end-to-end product journey. `docs/testing.md` already says so in its own
words ("the substrate") without naming the tier that.

Calling it "e2e" overstates the tier that actually gates things. It
undersells the one lane that is genuinely end-to-end — the nightly
real-runtime lane, nested inside `full`, which fakes nothing.

It also sits oddly beside other things already named "e2e" in this
repository: `platform/console/ui/e2e/` (a mocked-API browser smoke test, no
backend at all) and `e2e-live/` (a real browser against a real running
install). Neither is run by a pipeline, so neither is this change's concern.

## What Changes

- **Rename the pipeline-run cluster pack from "e2e" to "system"**, everywhere
  it is named as such:
  - `platform/manager/test/e2e/` → `platform/manager/test/system/`
  - build tag `e2e` → `system`
  - `.github/workflows/e2e.yml` → `system.yml`, `e2e-smoke.yml` →
    `system-smoke.yml`, `e2e-full.yml` → `system-full.yml`
  - env vars `E2E_TIER`, `E2E_ARTIFACT_DIR`, `E2E_BUDGET`, `E2E_REUSE`,
    `E2E_SKIP_BUILD` → `SYSTEM_TIER`, `SYSTEM_ARTIFACT_DIR`, `SYSTEM_BUDGET`,
    `SYSTEM_REUSE`, `SYSTEM_SKIP_BUILD`. `CLAUDE_CODE_OAUTH_TOKEN` is
    unaffected — it names a credential, not this tier.
  - `.github/scripts/e2e-report.py` → `system-report.py`, and its test
  - the check-run name `smoke-evidence.py` looks up: `" / e2e / smoke"`
    becomes `" / system / smoke"`
  - every workflow, script, rule file and doc that names the pack, its tag,
    its env vars or its workflows by the old name
  - the openspec task-section convention — the second of the three trailing
    sections changes from "E2E tests" to "System tests" in
    `openspec/config.yaml`, `.claude/rules/change-tests.md`,
    `.claude/hooks/require-docs-task.sh`, `.github/scripts/docs-task-guard.py`
    (including its regex and its test fixtures), `CLAUDE.md`,
    `CONTRIBUTING.md`, and `.github/routines/implement-issue.md`
- **`docs/testing.md`'s "End to end" tier row is renamed "System"**, with its
  wording updated to match. It already describes the tier as deciding "the
  substrate", which is the accurate word this change makes the name agree
  with.
- **Explicitly NOT renamed**, because neither is run by a pipeline and
  neither is this concept's misnomer:
  - `platform/console/ui/e2e/` (`test:e2e`) — a Playwright smoke test against
    a **mocked** API, no backend. Still arguably mislabeled on its own terms,
    but out of this change's scope.
  - `platform/console/ui/e2e-live/` — the one tier that genuinely is
    end-to-end (real browser, real running install). It keeps its name. It
    earns it.
- **Openspec capability and requirement identifiers are NOT renamed.** Per
  this project's own openspec tooling, a capability's directory path is a
  stable identifier, the same rule that kept `channel-type-model` named that
  through the `type`→`adapter` field rename.
  `openspec/specs/end-to-end-testing/` keeps its path. Its requirement text is
  updated to describe "the system pack" wherever it currently describes "the
  end-to-end pack".
- **No behavior changes.** The pack still runs the same assertions against
  the same real cluster, on the same cadence, gating the same things. This is
  a naming change, not a testing-strategy change.

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

- `end-to-end-testing`: the pack's own requirements describe themselves as
  "the system pack" rather than "the end-to-end pack". The capability's
  directory path is unchanged.
- `continuous-integration`: its cross-reference to the renamed workflow
  (`e2e.yml` → `system.yml`) and its cross-reference to the renamed pack.
- `remote-change-sessions`: the clause dispatching the cluster tier to "the
  smoke end-to-end workflow" now names "the smoke system workflow".
- `role-agents`: the trailing-sections list ("unit tests, e2e tests,
  documentation") now reads "unit tests, system tests, documentation".

## Impact

- **Code**: `platform/manager/test/e2e/` (directory move, all files inside,
  build tag). Nothing in the asserted behavior changes.
- **CI**: `.github/workflows/e2e.yml`, `e2e-smoke.yml`, `e2e-full.yml`
  (renamed), `ci.yml` and `release.yml` (references updated),
  `.github/scripts/e2e-report.py` and its test, `.github/scripts/
  smoke-evidence.py` and its test (the check-run name it looks up),
  `.github/components.sh` (comments only, no behavior there depends on the
  string "e2e").
- **Process tooling**: `openspec/config.yaml` (the injected task-section
  rule), `.github/scripts/docs-task-guard.py` and its test fixtures under
  `.github/tests/docs-task/*/tasks.md`, `.claude/hooks/require-docs-task.sh`.
- **Reference docs**: `docs/testing.md` — the "End to end" tier row and its
  prose. `README.md`'s nightly-workflow badge link and label. No other
  reference doc (`docs/concepts.md`, `docs/contracts.md`,
  `docs/CHANGELOG.md`) names this tier.
- **Adopter site**: none. This tier is internal CI/dev-tooling. No landing
  page, Introduction, Getting started, Installation page or guide mentions
  it, and this change adds no new adopter-facing behavior to document.
- **Rules**: `.claude/rules/build-test.md`, `change-tests.md`,
  `documentation.md`, `gotchas.md` (historical incident narratives keep
  their original file and workflow names as history, only forward-looking
  statements change), `remote-session.md`, `structure.md`, `CLAUDE.md`.
- **In-flight openspec changes** — other sessions' own `tasks.md` files that
  already use an "E2E tests" heading — are NOT touched by this change.
  `docs-task-guard.py`'s regex keeps accepting their existing heading until
  each of them finishes independently.
