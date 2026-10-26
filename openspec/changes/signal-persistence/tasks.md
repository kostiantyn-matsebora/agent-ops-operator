# signal-persistence — tasks

## 1. The Signal CRD

- [x] 1.1 Add the `Signal` type to `platform/manager/api/v1alpha1/` — spec (source-derived fields, `initiationId`, payload record, occurrence count), `status.claimants[]` (ref, state, conversation, reason), derived phase — and verify `go build ./...` passes
- [x] 1.2 Regenerate deepcopy and the CRD YAML into `chart/crds/` with controller-gen (pinned version from `build-test.md`) and verify the generated files are in the diff
- [x] 1.3 Add `signals` (lowercase plural) with full verbs to the manager's Role and `get`/`list`/`watch` to the console's in `chart/templates/rbac.yaml`, and verify the chart render tests pass — landed as `chart/templates/rbac.yaml` (manager) and `chart/templates/console.yaml` (console's own Role lives there, not `rbac.yaml`)

## 2. Persist-then-ack ingest and the shared routing path

- [x] 2.1 Extract today's routing (cooldown, claim fan-out, equality routing) from `/signal/inbound`'s serving code into one routing function operating on a `Signal`, and verify existing `internal/httpapi` tests still pass unchanged — the function lives in the new `internal/signalroute` package (`RouteClaimants`), shared with the reconciler
- [x] 2.2 Make `/signal/inbound` persist the `Signal` CR first, with its `ownerRef` to the owning SignalSource set at creation, run the inline first routing attempt, then respond — preserving every documented response semantic (unwired drop reason included) — and verify the httpapi tests assert CR-before-response ordering
- [x] 2.3 Implement absorb: a matching `initiationId` on a still-open Signal of the same source appends payload and bumps the count instead of creating a CR, and verify a unit test covers absorb against `Pending` and `PartiallyRouted` — known narrow gap: an absorbed repeat's appended payload is not redelivered to a claimant already `Routed` on a still-`PartiallyRouted`, multi-claimant signal (the common single-claimant case is unaffected — a terminal Signal never absorbs, so the repeat's own fresh CR finds the same open conversation by equality reuse)
- [x] 2.4 Implement the backlog-full change: machine kinds stay `Pending` as Signals, chat keeps the immediate decline, and verify unit tests cover both branches

## 3. The Signal reconciler

- [x] 3.1 Implement the reconciler routing non-terminal Signals through the shared routing path — claimant snapshot at first routing attempt, retries touching only non-terminal claimants, retry budget flipping exhausted ones to `Failed` — and verify fake-client unit tests in `internal/controller` cover each transition — this package's own convention for reconciler logic
- [x] 3.2 Implement the phase roll-up recomputed on every status write (`Pending`/`Routed`/`PartiallyRouted`/`Dropped`/`Failed`) and verify a table test walks every claimant-state combination
- [x] 3.3 Implement bounds: per-source live cap with oldest-eviction (pre-fan-out as `Dropped` reason overflow, post-fan-out by flipping non-terminal claimants to `Failed`), TTL GC after terminal phases via requeue with `Dropped` shorter than `Routed` and `Failed` on the `Routed` TTL (`SIGNAL_TTL_DROPPED` 4h, `SIGNAL_TTL_ROUTED` 72h, `Failed` on the `Routed` TTL) — and verify envtest covers both eviction shapes, GC ordering, and ownerRef-driven GC on source deletion — TTL is measured from `creationTimestamp` (`SignalStatus` carries no separate "became terminal" stamp). ownerRef GC is asserted as a schema check, not a cascading delete (this repo's envtest runs no garbage-collector controller — consistent with every other ownerRef-GC test here)
- [x] 3.4 Emit the `Failed` phase as loudly as the unwired drop (activity event, logged, counted) and verify a unit test asserts the event

## 4. Console signals view

- [x] 4.1 Add the `Signal` watch to the console's read-only informer set and verify the console builds with the kind in its cache
- [x] 4.2 Build the signals inventory (source, `initiationId`, phase, count, age, filter by source/phase) and the detail view with per-claimant state and conversation links, and verify vitest covers both — rides the console's existing generic Configuration inventory/detail pages at `/config/signals`
- [x] 4.3 Take the visual check screenshots against the dev server per `visual-check.md` and verify the rendered view matches the spec's content requirements — fixture-backed (mocked `/api/*`), not against a live manager — no live backend was available in this environment

## 5. Unit tests

- [x] 5.1 Run `go test ./...` (with envtest assets) in `platform/manager` and verify green
- [x] 5.2 Run the chart render tests and the console `npm run test:coverage` and verify green
- [x] 5.3 Run `go test -tags conformance ./test/conformance/` and verify the adapter contract suite passes untouched — the proof the response semantics did not move

## 6. System tests

- [ ] 6.1 Add or extend a lane in `platform/manager/test/system/`: signal ingested → Signal CR observable → conversation opened → phase `Routed`, plus a manager-restart case resuming a `PartiallyRouted` signal without a duplicate conversation, and verify in the pack run — the dispatched smoke run passed `TestSignalPersistenceReachesRoutedPhase`. The restart case is `fullTier`, skipped by smoke, so this stays open until a dispatched `system-full.yml` run passes it
- [x] 6.2 Verify console RBAC in-cluster: the console ServiceAccount can list/watch `signals` and cannot write them (`kubectl auth can-i` both ways) — asserted by `substrate_test.go`'s `TestRBACAsEnforced`, which passed in the dispatched smoke run

## 7. Documentation

- [x] 7.1 Reference docs: `docs/concepts.md` (the Signal kind, phases, claimant workflow, CRD and state matrices), `docs/contracts.md` (persist-then-ack note, response semantics unchanged), `docs/console.md` (the view and the watched kind), the state-durability matrix row, `docs/CHANGELOG.md` (new CRD + `kubectl apply -f chart/crds/` step) — `docs/cr-reference.md` no longer exists, deleted when the per-kind `docs/crds/` pages replaced it, before this change. The equivalent is the new, generated `docs/crds/signal.md`
- [x] 7.2 Adopter site: `docs/installation.md` (CRD upgrade step — already generic, confirmed needing no edit), `docs/console-guide.md` (what the signals view shows), `README.md` kind list within budget, regenerate the diagrams via `export.py`/`readme-flow.py` if they enumerate kinds (confirmed neither does — Signal carries no wiring edge), and re-run the console `screenshots` and `demo` npm scripts — `screenshots` ran clean with no asset changes, since no captured view renders Signal data yet. `demo`'s video encode needs a Docker daemon this environment doesn't have, so the poster/video are not re-published here and need a workstation session
