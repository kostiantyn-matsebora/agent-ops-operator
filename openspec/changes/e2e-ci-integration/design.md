## Context

Two independent Playwright suites already exist, with no shared code to
change — only the two workflows that invoke tests, `ci.yml` and `system.yml`.

| Suite | Needs | Current invocation |
|---|---|---|
| `platform/console/ui/e2e/` (`test:e2e`) | nothing — its own `webServer` builds and serves the production bundle against a mocked API | by hand only |
| `platform/console/ui/e2e-live/` | a running cluster, a deployed chart, a reachable console Service | by hand only, per `docs/testing.md` |

The full system tier (`system.yml`, `tier: full`) already provisions a k3d
cluster, builds every image from the commit under test, and installs the
chart — exactly what `e2e-live` needs, and already stood up nightly whether
or not this change lands.

Two facts about that pack make the live suite's wiring mechanical rather than
new plumbing:

- `clusterName()` defaults to `agentops-e2e`, and `k3d cluster create` merges
  that into the default kubeconfig as context `k3d-agentops-e2e` — the exact
  default `e2e-live/support/kube.ts` already assumes (`E2E_LIVE_KUBE_CONTEXT`).
- `SYSTEM_REUSE=1` already exists, for local dev, to keep the cluster alive
  instead of the harness deleting it on exit (`.github/workflows/system.yml`,
  `harness_test.go`).

## Goals / Non-Goals

**Goals:**
- A console pull request fails on a real-browser rendering defect the jsdom
  suite cannot see.
- A regression `e2e-live` can catch is caught within a day, not whenever
  someone happens to run it by hand.
- Neither workflow changes cluster-lifecycle semantics for the tiers that
  already exist (smoke, conformance, per-module).

**Non-Goals:**
- Making `e2e-live` gate a pull request. It needs a cluster, and
  `end-to-end-testing` already reserves cluster provisioning for the smoke
  and full tiers only.
- Changing what either suite asserts. This change wires existing specs into
  schedulers, it does not add scenarios to them.
- A shared Playwright config, a shared fixture library, or any other
  consolidation between `e2e` and `e2e-live`. They test different things
  (rendering vs. behavior) against different backends and SHOULD stay two
  configs.

## Decisions

**A new `console-e2e` job in `ci.yml`, not a step on the existing `modules`
matrix's console entry.**

The `modules` job is dependency-free by design for every module except
console, which already carries npm install and build. A browser install and
a Playwright run on that same entry would make its duration and failure
surface diverge further from the other twelve.

A failure would then need `modules (platform/console)` read closely to tell
"UI unit test failed" from "real-browser smoke failed." A separate job
names the failure in its own log, the same reasoning
`continuous-integration` already states for why console UI tests get their
own step.

Alternative considered: fold it into `modules`. Rejected — the job summary
would still show one result for two different kinds of failure.

**The job gates on `contains(needs.discover.outputs.modules, 'platform/console')`**,
the exact condition the `modules` job already uses for its console-only
steps, rather than adding a new `discover` output. One path filter per
subject, not one per job that happens to care about the same subject.

**No new build step — `test:e2e`'s own `webServer` already builds.**

`platform/console/ui/playwright.config.ts`'s `webServer.command` is
`npm run build && npm run preview`. The job's steps are checkout, Node
setup, `npm ci`, `npx playwright install --with-deps chromium`, and
`npm run test:e2e`.

Nothing here duplicates `modules`' own build. No job's output is read by
another job today, and this one does not start.

**Playwright browsers are cached**, keyed on `package-lock.json`'s hash,
the same pattern `actions/cache` already uses for envtest assets and Go
modules elsewhere in `ci.yml`. Without it, every console pull request pays a
Chromium download its predecessor already paid for.

**`e2e-live` runs as additional steps inside `system.yml`'s existing `pack`
job, gated on `inputs.tier == 'full'`, after the pack's own `go test`
step — not as a second job.**

A second job would need its own cluster. That means its own
`k3d cluster create` and its own chart install, paying the pack's setup
cost twice a night for no benefit, since the first pack run already
produced a cluster with the chart Ready.

One job keeps "one cluster, one chart install, two kinds of assertion
against it," matching how the pack's own Go lanes already share one cluster
across many `*_test.go` files.

**The pack is told to keep its cluster with `SYSTEM_REUSE=1`, set only when
`inputs.tier == 'full'`.**

This reuses the exact knob local developers already use
(`.claude/rules/build-test.md`'s `SYSTEM_REUSE=1` local run) instead of
inventing a second way to say "don't tear down yet."

The workflow step that runs after `go test` becomes responsible for the
teardown `SYSTEM_REUSE` would otherwise leave to a human:
`k3d cluster delete agentops-e2e`, `if: always()`. A Playwright failure
still tears the cluster down.

**The console URL is a workflow-managed `kubectl port-forward`, not a
second use of the Go harness's own `Forward`.**

The Go harness's port-forward is a child process of the `go test` binary
and dies with it, by design — `cluster.go`'s `Forward.Stop()` is
unconditional, unrelated to `SYSTEM_REUSE` (which governs cluster deletion,
not the test binary's own forwards).

A workflow step opens `kubectl --context k3d-agentops-e2e port-forward
svc/agentops-adapter-console 18099:8080 -n agent-ops &` after `go test`
exits. This is the exact command and port `docs/testing.md` already
documents for a human running this suite by hand, so `e2e-live` itself
needs no new environment variable — only an already-reachable
`localhost:18099`.

Alternative considered: have the Go pack itself shell out to `npx
playwright test` before its own teardown, keeping its own forward open
across the call.

Rejected on two counts. It puts a Node/npm toolchain dependency inside
`platform/manager`'s Go test binary, against `docs/testing.md`'s stated
boundary. And it conflates two failure kinds — pack, browser suite — the
spec requires to stay distinguishable in one exit code.

**Playwright's HTML report and trace/video artifacts from both jobs upload
on failure only**, via `actions/upload-artifact`, `if: failure()`.

Nobody watches a nightly run complete. A report is read after a failure or
not at all, so uploading unconditionally would only keep files nobody opens
from every green night.

## Risks / Trade-offs

- **A flaky browser test blocks an unrelated console pull request** →
  `playwright.config.ts` sets `retries: 2` under `process.env.CI`, matching
  the bounded-retry posture `end-to-end-testing` already requires of the
  live suite's own agent-dependent assertions.
- **`npx playwright install --with-deps` is the slow part of a fast job** →
  cached by lockfile hash. A cache miss (a Playwright version bump) still
  completes, just slower that one run.
- **The full tier's job grows a second kind of failure (cluster/pack vs.
  browser suite) in one job's log** → each lives in its own named step, and
  the scenario "the pack's own failure skips the browser pass" is asserted
  by an explicit `if: steps.pack.outcome == 'success'` guard on the
  `e2e-live` step, not by shell `&&` chaining that would bury which part
  failed.
- **`agentops-e2e-*.kubeconfig` temp files and the merged `k3d-agentops-e2e`
  context accumulate on a self-hosted runner reused across nights** →
  out of scope here: GitHub-hosted runners are ephemeral per job, and a
  self-hosted migration is a separate change with its own cleanup story.

## Migration Plan

No data migration. Both workflow files are edited in place.

Both suites' existing specs and configs stay unchanged, except the two
additions named above: `retries` in `e2e/playwright.config.ts`, and the
documented env usage in `e2e-live`.

Rollback is reverting the workflow edits. Neither suite's assertions
depend on the new wiring to keep working by hand.
