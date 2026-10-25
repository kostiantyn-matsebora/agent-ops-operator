## 1. Gate console pull requests with the mocked e2e tier — deployment-engineer

- [ ] 1.1 Add `retries: process.env.CI ? 2 : 0` to
  `platform/console/ui/playwright.config.ts` and verify `CI=1 npm run
  test:e2e` still passes against this working copy's tree.
  **The `retries` config landed, but the suite does not pass**: two of four
  `e2e/topology.spec.ts` specs were already failing against an unmodified
  `master`, for reasons unrelated to this change. Fixed in place: a stale
  `data-flashing`/`data-animated`/SMIL `<animate>` assertion (`Canvas.tsx`
  animates traffic through `TrafficLayer`'s own marks now, not those
  attributes) and a missing "open the collapsible Display panel" click
  before the `Toolsets` checkbox inside it. NOT fixed: both specs also
  assume the `mcptoolsets` class starts hidden (`display.ts`'s
  `VIEW_DEFAULTS.model.hidden` is `[]` — nothing is hidden by default), so
  both still fail at that assertion. Proposed patch: have the spec hide the
  class itself (open the panel, uncheck `Toolsets`) before asserting
  concealment, rather than assuming the app starts that way — a
  self-contained test fix, not a `platform/console/ui/src` change. Left
  for review rather than guessed at further: redesigning what this spec
  asserts is this change's own stated non-goal (`design.md`).
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
  `ci-green` reads its result. **Pushed.** It will show red until 1.1's
  remaining finding is resolved. No `gh` auth here to dispatch a
  stand-alone run ahead of the pull request's own CI.

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

- [ ] 3.1 Run `CI=1 npm run test:e2e` in `platform/console/ui` against
  this working copy's tree and confirm it passes with the new `retries`
  setting in place. Same finding as 1.1 — ran, two specs still fail on the
  pre-existing, out-of-scope mismatch documented there.
- [x] 3.2 Validate both edited workflow files parse, for example with
  `python3 -c "import yaml, sys
[yaml.safe_load(open(f)) for f in sys.argv[1:]]" .github/workflows/ci.yml .github/workflows/system.yml`.

## 4. System tests

- [ ] 4.1 Push this branch and run `gh workflow run system-full.yml` (it
  calls `system.yml` with `tier: full`) against it. Confirm the pack job's
  log shows `go test` completing, the cluster surviving under
  `SYSTEM_REUSE=1`, the `e2e-live` step running against that same cluster's
  console, and the teardown step deleting `agentops-e2e` afterward.
- [ ] 4.2 Confirm the scenario "a pack failure skips the browser pass" on
  a scratch run. Temporarily break a chart value to fail the pack's
  `go test` step, then confirm the `e2e-live` step is skipped while the
  teardown step still runs.
- [ ] 4.3 Run `gh workflow run system-smoke.yml` against this branch and
  confirm the smoke tier (`inputs.tier == 'smoke'`) shows no Node, npm, or
  Playwright step — the new steps stay full-tier-only.

## 5. Documentation

- [ ] 5.1 Reference docs: rewrite `docs/testing.md`'s `e2e-live` section —
  remove the "NOT CI-gated, and no workflow dispatches it" claim and state
  the actual nightly wiring into the full tier. Update `CONTRIBUTING.md`'s
  CI section to name the `console-e2e` job and the full tier's browser
  pass.
- [ ] 5.2 Adopter site: none. This change is internal verification process,
  not a product capability, an install/upgrade step, or a claim the
  landing page or guides make — ticked as not applicable.
