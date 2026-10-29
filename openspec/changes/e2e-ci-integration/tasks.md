## 1. Gate console pull requests with the mocked e2e tier — deployment-engineer

- [x] 1.1 Add `retries: process.env.CI ? 2 : 0` to
  `platform/console/ui/playwright.config.ts` and verify `CI=1 npm run
  test:e2e` still passes against this working copy's tree.
  **Passes, 4 of 4, three consecutive runs.** Two of four
  `e2e/topology.spec.ts` specs were already failing on an unmodified
  `master`. The spec had drifted from the UI it pins, corrected as test
  content only, no `platform/console/ui/src` change:
  - Traffic is asserted through `data-stream` — `Canvas.tsx` never
    rendered the `data-flashing`/`data-animated`/SMIL `<animate>` it
    checked.
  - The spec hides `Toolsets` itself before asserting concealment —
    `VIEW_DEFAULTS.model.hidden` is `[]`.
  - `1 failing` is read from `health-summary` — three elements now carry
    the text.
  - Scoping is a double-click, and the out-of-scope notice is the
    `failing-hidden` alert.
  - The node count excludes `node-panel`.
  - The fit is measured against `pictureBounds`' own margins rather than
    the painted box, whose labels sit off-centre by design. Confirmed
    still failing on a 50px shift of `fit()`.

  The webServer also binds `--host 127.0.0.1`. On the runner `localhost`
  resolves to `::1`, so the probe at `127.0.0.1:4173` timed out after a
  clean build.
- [x] 1.2 Add a `console-e2e` job to `.github/workflows/ci.yml`, gated on
  `contains(needs.discover.outputs.modules, 'platform/console')`: checkout,
  Node 22 setup, `npm ci`, a Playwright-browser cache keyed on
  `package-lock.json`'s hash, `npx playwright install --with-deps
  chromium`, `npm run test:e2e`, and `actions/upload-artifact` of
  `playwright-report`/`test-results` `if: failure()`. Verify the job's YAML
  parses, for example with `python3 -c "import yaml
yaml.safe_load(open('.github/workflows/ci.yml'))"`.
- [x] 1.3 Add `console-e2e` to `ci-green`'s `needs:` list in `ci.yml`.
  Verify by pushing this branch and running `gh workflow run ci.yml -f
  pr=<n>`, confirming the dispatched run shows `console-e2e` and that
  `ci-green` reads its result. **Pushed** — the pull request's own CI
  shows `console-e2e` under `ci-green`'s `needs:`.

## 2. Wire the live browser suite into the nightly full tier — deployment-engineer

- [x] 2.1 In `.github/workflows/system.yml`'s `pack` job, set
  `SYSTEM_REUSE=1` on the `go test` step only `if: inputs.tier == 'full'`,
  and give that step an `id` so a later step can read its outcome. Here
  `SYSTEM_REUSE=1` only defers the Go binary's own teardown to task 2.5's
  explicit step — unlike its local-dev use, it does NOT leave the cluster
  running once this job ends.
- [x] 2.2 Add Node 22 setup, `npm ci`, and `npx playwright install
  --with-deps chromium` steps under `platform/console/ui`, gated
  `if: inputs.tier == 'full'`.
- [x] 2.3 Add a step, `if: inputs.tier == 'full' && steps.<go-test-id>.outcome
  == 'success'`, that opens `kubectl --context k3d-agentops-e2e
  port-forward svc/agentops-adapter-console 18099:8080 -n agent-ops &` and
  then runs `npx playwright test --config e2e-live/playwright.config.ts`
  from `platform/console/ui`.
- [x] 2.4 Add `actions/upload-artifact` of `e2e-live`'s report and
  `test-results`, `if: failure() && inputs.tier == 'full'`.
- [x] 2.5 Add a teardown step, `if: always() && inputs.tier == 'full'`,
  running `k3d cluster delete agentops-e2e || true` — the cleanup task
  2.1's `SYSTEM_REUSE=1` deferred to here, since no human is present in
  CI to delete it later by hand. Verify every added step's YAML with the
  same `yaml.safe_load` check as task 1.2.

## 3. Unit tests

- [x] 3.1 Run `CI=1 npm run test:e2e` in `platform/console/ui` against
  this working copy's tree and confirm it passes with the new `retries`
  setting in place. 4 of 4 pass — see 1.1.
- [x] 3.2 Validate both edited workflow files parse, for example with
  `python3 -c "import yaml, sys
[yaml.safe_load(open(f)) for f in sys.argv[1:]]" .github/workflows/ci.yml .github/workflows/system.yml`.

## 4. System tests

- [ ] 4.1 Push this branch and run `gh workflow run system-full.yml` (it
  calls `system.yml` with `tier: full`) against it. Confirm the pack job's
  log shows `go test` completing, the cluster surviving under
  `SYSTEM_REUSE=1`, the `e2e-live` step running against that same cluster's
  console, and the teardown step deleting `agentops-e2e` afterward.
  **Still open, blocked on an environment failure that is not this
  change's.** The prior round's dispatch
  ([run 40407670027](https://github.com/kostiantyn-matsebora/agent-ops-operator/actions/runs/40407670027))
  failed before `go test` reached this suite at all: `manager restart`
  timed out waiting for `agentops-manager`'s rollout (`1 out of 2 new
  replicas`). The same failure reproduces on unmodified `master` itself,
  every nightly `system-full` run from 2026-10-26 through 2026-10-29 (runs
  40295360111, 40403246734, 40414922184, 40477200812, 40665777022,
  40853551940). That is a pre-existing CI health issue. This change's diff
  (CI workflow wiring plus test content) cannot cause or fix it.
  Re-dispatched this round anyway for a current link. Left unticked, since
  it will not confirm what it asks until that failure is fixed separately.
- [x] 4.2 Confirm the scenario "a pack failure skips the browser pass" on
  a scratch run. Temporarily break a chart value to fail the pack's
  `go test` step, then confirm the `e2e-live` step is skipped while the
  teardown step still runs. **Confirmed live, not on a scratch run** — the
  prior round's `system-full` dispatch
  ([run 40407670027](https://github.com/kostiantyn-matsebora/agent-ops-operator/actions/runs/40407670027),
  job `full / system / full`) failed its `go test` step on an unrelated
  manager-rollout timeout, and the log shows exactly this gate: "Set up
  Node", "Install console UI dependencies", "Install Playwright
  browsers" and "Run e2e-live suite" all report `skipped`, while "Tear
  down the e2e cluster" still reports `success`. A real failing run beats
  a scratch one for this box.
- [x] 4.3 Run `gh workflow run system-smoke.yml` against this branch and
  confirm the smoke tier (`inputs.tier == 'smoke'`) shows no Node, npm, or
  Playwright step — the new steps stay full-tier-only. **Confirmed** — the
  prior round's dispatch
  ([run 40407665557](https://github.com/kostiantyn-matsebora/agent-ops-operator/actions/runs/40407665557))
  completed. "Set up Node", "Install console UI dependencies", "Install
  Playwright browsers" and "Run e2e-live suite" all report `skipped` on
  the smoke tier.

## 5. Documentation

- [x] 5.1 Reference docs: rewrite `docs/testing.md`'s `e2e-live` section —
  remove the "NOT CI-gated, and no workflow dispatches it" claim and state
  the actual nightly wiring into the full tier. Update `CONTRIBUTING.md`'s
  CI section to name the `console-e2e` job and the full tier's browser
  pass. **Already done**, landed in #439: `docs/testing.md` line 12 names
  `e2e-live` as nightly against the full tier's own cluster, and
  `CONTRIBUTING.md` lines 275-277 name the full tier running `e2e-live`
  against that run's own cluster after the pack's assertions pass. Both
  files were read against this branch's tree and match.
- [x] 5.2 Adopter site: none. This change is internal verification process,
  not a product capability, an install/upgrade step, or a claim the
  landing page or guides make — ticked as not applicable.
