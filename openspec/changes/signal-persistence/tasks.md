# signal-persistence — tasks

## 1. The Signal CRD

- [ ] 1.1 Add the `Signal` type to `platform/manager/api/v1alpha1/` — spec (`sourceRef`, `adapter`, `initiationId`, optional `signalId`, `kind`, `title`, `severity`, `labels`, `reader`, `payload`), status (`phase`, `reason`, `completedAt`, `attempts`, `firstAttemptAt`, `consumedBy`, `consumedAt`, `claimants[]`) — and verify the enums match the delta spec's field table
- [ ] 1.2 Regenerate deepcopy and the CRD YAML into `chart/crds/` with controller-gen (pinned version from `build-test.md`) and verify the generated files are in the diff
- [ ] 1.3 Add `signals` (lowercase plural) with full verbs to the manager's Role and `get`/`list`/`watch` to the console's in `chart/templates/rbac.yaml`, and verify the chart render tests pass
- [ ] 1.4 Add the six chart values (`signalTtlRouted` 8h, `signalTtlDropped` 4h, `signalTtlFailed` 72h, `signalRetryDeadline` 1h, `maxSignalsPerSource` 200, `signalConsumerConcurrency` 4) to `chart/values.yaml`, render them into the manager Deployment env, and verify a render test covers an override, the default and an invalid duration

## 2. Publish

- [ ] 2.1 Reduce `/signal/inbound` to validate, apply `attributeFilters`, create one `Signal` per admitted signal with its `ownerRef` to the `SignalSource` set at creation, and acknowledge — no claim lookup, cooldown, capacity or addressing — and verify unit tests show nothing is routed at publish
- [ ] 2.2 Answer a chat command that only answers (`/pipelines`, a bare `/<pipeline>`, `/close` or `/exit` on a general surface) in place and store no `Signal`, and verify a unit test for each
- [ ] 2.3 Derive the `Signal` name from `(source, signalId)` as `sig-<first 32 hex of sha256(source NUL signalId)>` when a `signalId` is present (a `Signal` without one gets a random suffix and is never deduplicated at publication), treat an `AlreadyExists` as an acknowledged duplicate with a `duplicate` activity event, and verify tests for a re-send, a repeat inside one batch, a concurrent race, a re-send after the `Signal` went `Routed`, and the same `signalId` on two sources
- [ ] 2.4 Return `200 {"queued": n}` counting signals stored or deduplicated, `5xx {"error"}` when a store fails, and keep the `attributeFilters` `reason`, and verify the response tests and the console's adapter client still pass

## 3. The consumer

- [ ] 3.1 Implement the consumer pool on the leader only, `SIGNAL_CONSUMER_CONCURRENCY` consumers over the controller work queue, so one consumer holds a `Signal` at a time, and verify envtest shows a non-leader consumes nothing
- [ ] 3.2 Serialize messages sharing a `(source, initiationId)` in arrival order (`creationTimestamp`, then name) while other keys run in parallel, and verify tests for two messages of one conversation never appending out of order or creating two conversations, and for two conversations processing in parallel
- [ ] 3.3 Move the routing decisions into the consumer in the spec's order — addressed target, the source's Ready claimants, unclaimed, ambiguous, cooldown checked and spent atomically per source, capacity, snapshot and route — and verify unit tests walk each outcome
- [ ] 3.4 Make routing idempotent: equality reuse read directly from the API server, the `ConversationInput` written under a name derived from the `Signal` and claimant, and an addressed conversation adopting the `initiationId` as its `conversationId`, and verify tests for a crash between create and the status write and for a redo after a leader change that must append once
- [ ] 3.5 Implement the phase roll-up recomputed on every status write (`Pending`/`Routed`/`PartiallyRouted`/`Dropped`/`Failed`), with `consumedBy`, `consumedAt` and `attempts` written at the end, and verify a table test walks every claimant-state combination
- [ ] 3.6 Enqueue the chat notice for a chat signal dropped as unclaimed, ambiguous or at capacity through the existing channel-operation queue as today, and verify the surface is told and the `Signal` still records the reason
- [ ] 3.7 Implement the backlog-full change: a machine signal stays `Routing` and is requeued with a delay, chat is declined at once including addressed chat, and verify unit tests cover both branches
- [ ] 3.8 Emit the `Failed` phase as loudly as the unwired drop (activity event, logged, counted) and verify a unit test asserts the event
- [ ] 3.9 Resume on a leader change: the new leader's consumer reconciles every non-terminal `Signal`, and verify envtest covers a `PartiallyRouted` signal resumed with no duplicate conversation

## 4. Housekeeping

- [ ] 4.1 Make the retry deadline, the per-source cap with oldest-first eviction and the TTL deletes leader-only, going through the same work queue as the consumer and writing conditionally on the `resourceVersion`, and verify envtest shows a non-leader does none of them
- [ ] 4.2 Implement the per-phase TTLs from `status.completedAt` (`Routed` 8h, `Dropped` 4h, `Failed` 72h) and the `ownerRef` cascade from a deleted `SignalSource`, and verify envtest covers each TTL, their ordering and the ownerRef as a schema check

## 5. Adapters send `signalId`

- [ ] 5.1 `signals/cron`: send `cron:<source>:<tick RFC3339 UTC>`, so a re-fire of the same tick repeats the id, and verify a unit test
- [ ] 5.2 `signals/k8s-events`: send `<source>/<event uid>/<count>`, and verify a unit test
- [ ] 5.3 `signals/ha`: send `<initiationId>#<unix seconds the dwell cycle opened>`, and verify a unit test
- [ ] 5.4 `signals/telegram`: send `tg-<source>-<update id>` for a message and `tg-cb-<source>-<callback id>` for a tap, and verify a unit test
- [ ] 5.5 `signals/alertmanager` and `signals/pulse`: DECISION FIRST. A body-hash `signalId` deduplicates an identical re-notification for the `Signal`'s life. Ask the user whether that is wanted or whether a delivery time bucket is added, then implement the answer and verify a unit test
- [ ] 5.6 Extend `platform/manager/test/conformance/`: `fakemanager.go` answers `{"queued": n}` with no `conversations`, every adapter sends a non-empty `signalId` and the SAME one after a `5xx`, a different message gets a different one, and add the missing Pulse case

## 6. Console signals view

- [ ] 6.1 Add the `Signal` watch to the console's read-only informer set and verify the console builds with the kind in its cache
- [ ] 6.2 Build the signals inventory (source, `initiationId`, phase, age, filter by source and phase) and the detail view with per-claimant state, conversation links, the consumer and the delivery history, under the existing Configuration pages, and verify vitest covers both
- [ ] 6.3 Take the visual check screenshots against the dev server per `visual-check.md` and verify the rendered view matches the spec's content requirements

## 7. Unit tests

- [ ] 7.1 Run `go test ./...` (with envtest assets) in every module the change touches and verify green
- [ ] 7.2 Run the chart render tests and the console `npm run test:coverage` and verify green
- [ ] 7.3 Run `go test -tags conformance ./test/conformance/` and verify the adapter contract suite passes with the new response and `signalId` checks

## 8. System tests

- [ ] 8.1 Add or extend a lane in `platform/manager/test/system/`: a signal published, the `Signal` CR observable and claimed, a conversation opened, phase `Routed`, plus an addressed `/<pipeline> <task>` from the console path producing a `Signal`, and verify in the pack run
- [ ] 8.2 Add a leader-change case: remove the leader pod while a `Signal` is part-routed, and verify the new leader finishes it and produces one conversation, never two
- [ ] 8.3 Verify console RBAC in-cluster: the console ServiceAccount can list and watch `signals` and cannot write them (`kubectl auth can-i` both ways)
- [ ] 8.4 Dispatch `system-full.yml` on the change's branch and verify the full tier passes

## 9. Documentation

- [ ] 9.1 Reference docs: `docs/concepts.md` (the Signal kind, phases, claimant workflow, the consumer, CRD and state matrices), `docs/contracts/signal-adapter.md` (version 2.3.0, the response, `signalId`, the rewritten `API-2` and `API-3`, new rows, changelog), `docs/contracts.md` (publish then consume), `docs/console.md`, `docs/configuration.md` (the six chart values), `docs/CHANGELOG.md` (the new CRD, the contract version, the `kubectl apply -f chart/crds/` step), and `docs/crds/signal.md` via `docs-generate.py`
- [ ] 9.2 Adopter site: `docs/installation.md` (CRD upgrade step), `docs/console-guide.md` (what the signals view shows, with its screenshot), `docs/integrations/*.md` and `docs/guides/signal-adapter.md` (the claimed specification version), `README.md` kind list within budget, regenerate the diagrams via `export.py` and `readme-flow.py` if they enumerate kinds, and re-run the console `screenshots` and `demo` npm scripts
