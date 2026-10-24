Every build and test below runs INSIDE the worktree (`docker exec -w "$PWD"`
from `../agent-ops-worktrees/warm-runtime-pods`), and every deploy uses
`--state-values-set chartPath=` naming this worktree's `chart/` — the defaults
resolve master and report success against it. `kubectl apply -f chart/crds/`
precedes every deploy: the API server prunes an unknown `spec.warm` silently.

## 1. API (design decision 2)

- [ ] 1.1 `Pipeline.spec.warm` (int, min 0, default 0) with a doc comment that
      says why it is wiring; `ConversationPhase` gains `Reserved` with a
      comment on what it excludes; regenerate deepcopy and `chart/crds/`;
      `go build ./... && go vet ./...` green in the container and the CRD
      diff shows both fields.
- [ ] 1.2 `runtimepod`: a warm-born pod carries a label naming it so and is
      built with the runtime idle TTL disabled — establish per runtime the
      value each reads as "never" (`runtimes/claude`, `runtimes/copilot`,
      `runtimes/ollama`), fix `itoa`'s `<=0 → 30` mapping for that path, and
      pin all three in `podspec_test.go`.

## 2. Reserved gates (design decision 2)

- [ ] 2.1 Conversation reconciler: `Reserved` ensures no topic, dispatch's
      `tryDispatch` returns nothing for it, `admit`'s waiting set and
      `MapRuntimePodToPending` skip it, and `createRuntimePod` still creates
      its pod and ConfigMap; envtest: a reservation has a pod, a ConfigMap and
      no `ensure-topic` op.
- [ ] 2.2 Ingest reuse (`findReusable` or its equivalent in `signals.go`)
      filters `Reserved`; test: a matching-signature reservation is not
      reused.

## 3. Adoption (design decision 3)

- [ ] 3.1 `signals.go`: before `GenerateName`, list `Reserved` conversations
      for the Pipeline, oldest first, and `Update` the first one with inputs,
      signature label, title, origin and phase in ONE write; on
      Conflict/NotFound take the next; none → the existing create path.
      Envtest: a chat signal adopts, the pod name is the reservation's, the
      first `/work` poll dispatches, and R2 stays reserved when R1 exists.
- [ ] 3.2 Fan-out: a source served by two Pipelines adopts on the one holding
      a reservation and creates cold on the other; envtest asserts both.
- [ ] 3.3 Race test: an adoption and a preconditioned delete of the same
      reservation, one succeeds and one gets Conflict, in both orders; the
      losing side's fallback is exercised (next reservation / new object /
      next eviction tier).

## 4. Eviction order and accounting (design decision 4)

- [ ] 4.1 `createRuntimePod`: tier 1 deletes the NEWEST reservation's
      Conversation with `Preconditions{ResourceVersion}` and retries on
      conflict; tier 2 is today's idle eviction; `Pending` last. A
      conversation whose own Pipeline holds a reservation never reaches this
      branch (it adopted). Envtest: workspace conversation + full cap with one
      reservation → reservation deleted, conversation never `Pending`;
      reservation + idle conversation → reservation goes first.
- [ ] 4.2 `evictableCount` counts reservations as free; envtest: a cap full of
      reservations admits a new conversation without `Pending`.
- [ ] 4.3 Deleting the Conversation cascades pod + ConfigMap through the
      existing ownerRefs and fires the pod-DELETE watch; assert the waiting
      conversation is promoted on that event.

## 5. Refill (design decision 5)

- [ ] 5.1 Pipeline reconciler: count reservations, create up to `warm − held`
      while `live < cap` and nothing is `Pending`, delete surplus when `warm`
      shrinks or the Pipeline is not Ready; requeue on `WARM_REFILL_DELAY`
      (default 30 s, env + chart value under `manager.env`). Envtest per
      branch, including "two Pipelines, one free slot → one reservation".
- [ ] 5.2 Snapshot drift: compare each reservation's materialised spec with
      `SnapshotFor` now; mismatch → delete and let refill replace. Envtest:
      editing `serviceAccountName` replaces the reservation.
- [ ] 5.3 Wire the pod-DELETE watch into the Pipeline reconciler's enqueue
      (map pod → its conversation's `pipelineRef`), acting only after the
      delay; test: a freed slot is not refilled before the delay and is after.

## 6. Manager-side idle release (design decision 6)

- [ ] 6.1 Conversation reconciler: an adopted warm-born pod with
      `!NeedsWorker` for the runtime's effective `idleTTLMinutes` since
      `lastActivity` is deleted (pod only), phase → `Idle`; requeue at the
      remaining interval. Envtest with a short TTL: reservation outlives it,
      adopted conversation is released after it. `/exit` unchanged, asserted.

## 7. Chart, console and verification

- [ ] 7.1 `chart/`: `pipelines[].warm` rendered on the Pipeline template and
      on each bundle route's values (default 0), `WARM_REFILL_DELAY` on the
      manager; NOTES.txt states that a reservation holds a capacity slot;
      `helm template` with `warm: 1`, `helm-unittest`/render tests pass,
      `serviceaccount-guard.py` and the chart tests green in the container.
- [ ] 7.2 Console: `Reserved` is rendered as a phase badge and excluded from
      the count of open conversations; `npm test` in `platform/console/ui`.
- [ ] 7.3 Deploy the WORKTREE chart to the local install with `warm: 1` on
      one route: observe a `Reserved` conversation and its pod; send a bare
      chat message; observe adoption (`status.runtimePod` equals the
      reservation's pod) and the time from signal to `run.dispatched` in the
      activity stream against a cold route. Record the numbers in design.md.
- [ ] 7.4 Fill the cap with reservations and post a task on a
      workspace-persistent route; observe one reservation deleted and the
      task admitted without `Pending`. Then wait past the runtime TTL on the
      adopted conversation and observe its pod released.
- [ ] 7.5 `python3 .github/scripts/publication-guard.py` and
      `retired-vocabulary-guard.py` pass; record the verdict only.
- [ ] 7.6 `.claude/rules/`: `invariants.md` (a reservation is the first thing
      evicted and never a reason for Pending; adopt/evict serialised on the
      object), `terminology.md` (a reservation is a `Reserved` Conversation,
      never "a warm pod" alone), `wiring.md` (`spec.warm` is wiring).

## 8. Documentation

### 8.1 Reference docs

- [ ] 8.1.1 `docs/concepts.md`: the `Reserved` phase in the phase table, the
      pool, the eviction order, the state-durability matrix row for a
      reservation (a Kubernetes object, derivable, nothing in memory).
- [ ] 8.1.2 `docs/guides/pipeline.md` marker gains `warm`; re-run
      `python3 .github/scripts/docs-generate.py` so `docs/cr-reference.md`
      and every generated Pipeline block carry the field; `--check` passes.
- [ ] 8.1.3 `docs/CHANGELOG.md`: the new field and phase, the CRD apply step,
      the slot a reservation costs.

### 8.2 Adopter site

- [ ] 8.2.1 `docs/guides/pipeline.md` prose: when to set `warm`, that it is
      per route and costs a slot, that resumed conversations are not served.
- [ ] 8.2.2 `docs/installation.md`: `pipelines[].warm` and
      `WARM_REFILL_DELAY` under the capacity decision, beside
      `maxActiveConversations`.
- [ ] 8.2.3 `docs/console-guide.md`: what a `Reserved` row is; re-run
      `npm run screenshots` and `npm run demo` in `platform/console/ui` if the
      fixture shows one.
- [ ] 8.2.4 `docs/introduction.md` / `getting-started.md` / the landing page:
      check whether any states "a signal creates the pod" as the whole story;
      amend only where it does.
