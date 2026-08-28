## Context

See proposal.md — Why. What shapes the approach is what a runtime pod bakes
in at creation and cannot change afterwards (`runtimepod.Build`):

| Baked in | Whose fact | Mutable later? |
|---|---|---|
| `CONVO_ID`, `POD_NAME`, pod name `agentops-conv-<c>` | the conversation | no — env and name are immutable |
| workspace claim `subPath: <c>` | the conversation | no — and the agent may never mount the claim root |
| `context-store` `subPath: <c>` on the sidecar | the conversation | no |
| MCP ConfigMap `agentops-mcp-conv-<c>`, `valueFrom` secret env | the Pipeline | no — kubelet-resolved, the manager reads no Secrets |
| repo URL/ref, SSH key volume or HTTPS token | the profile | no — same reason for the credential |
| ServiceAccount, `MCP_ENDPOINTS` on the egress proxy, resources | Pipeline / profile | no |
| image, command, sidecar, mediation | the runtime | no |

Everything in that table is the Pipeline's snapshot or derived from it. So the
unit a pod can be warmed FOR is a Pipeline — and the conversation name must
exist before the pod does.

Two existing facts make the chosen design cheap:

- The sidecar already restores context lazily, on the first `/work` it proxies
  (`context-sync/proxy.go` `ensureRestored`), and the runtime already
  long-polls `/work?convo=` until it is answered.
- Conversation names are `GenerateName`; nothing derives a name from a
  signature, so a conversation can exist before the signal that fills it.

## Goals / Non-Goals

**Goals:**
- A new conversation on a warm Pipeline is answered from a pod already past
  boot and clone, with NO change to any runtime, sidecar or proxy image.
- A warm pod costs a real conversation nothing: first evicted, never a cause
  of `Pending`.
- The adopt/evict race is settled by the API server, not by in-process
  ordering, because ingest and the reconciler are separate goroutines today and
  could be separate processes tomorrow (the HTTP API is not leader-gated).

**Non-Goals:**
- Resuming an IDLE conversation on a warm pod. Its name is fixed and its pod
  is gone; the checkout is already on its claim (fetch, not clone) and the
  cold path there is scheduling + boot. A generic late-bound pod that could
  serve it (design A below) is deferred.
- Warming the model, the MCP servers or the claude CLI: none live in the
  runtime pod.
- A pool per runtime — see decision 1.

## Decisions

### 1. The pool is per Pipeline, not per AgentRuntime

A per-runtime pod would have to receive identity, credentials and MCP secrets
at bind time, which is exactly what the "manager reads no Secrets" invariant
and kubelet-resolved `valueFrom` make impossible. The bindable set under a
per-runtime pool was "no SA override, public repo, secret-free MCP, no
resources override" — the routes with the SMALLEST cold start. Per Pipeline
has no such condition. Cost: the pool is bounded per route, and `warm` is a
per-route decision an adopter makes.

### 2. A reservation is a `Conversation` in phase `Reserved`

Alternatives: a bare pod with a reserved name owned by the Pipeline, or a
pod-plus-ConfigMap pair with a bookkeeping label. The Conversation object wins
on four counts, each of which the alternatives had to reinvent:

| Need | With a `Reserved` Conversation |
|---|---|
| ownership of pod + ConfigMap | ownerRef on the Conversation, as today |
| the snapshot | `SnapshotFor` at creation, as today |
| housekeeping's orphan rule ("the Conversation predates its directory") | holds unchanged — the alternatives BREAK it and get their checkout reclaimed mid-warm |
| a serialisation point for adopt vs evict | the object's `resourceVersion` |

The pod is created by `createRuntimePod` through the existing path, so every
guard it carries (start deadline, storage breaker, mediation condition) covers
a reservation for free. Phase `Reserved` is gated exactly where `Pending` is:
no topic, no dispatch (`tryDispatch` returns nothing for it), excluded from the
admission waiting set, excluded from signature reuse (`findReusable` filters
the phase).

### 3. Adoption is an ingest step ahead of `GenerateName`

In `signals.go`, where `conv = &Conversation{GenerateName: ...}` is built: list
`Reserved` conversations with `spec.pipelineRef == pipeline`, sort by
`creationTimestamp`, and for the oldest do an `Update` (not a merge patch —
the `resourceVersion` must be asserted) that writes inputs, signature label,
title, origin and phase. On `Conflict` or `NotFound`, take the next; when none
remain, fall through to the create path unchanged. Inputs are written in the
same update, so a reservation is never observed adopted-but-empty.

### 4. Eviction of a reservation is a conditional DELETE of the Conversation

`createRuntimePod`'s eviction branch gains a first tier: among live pods whose
conversation is `Reserved`, choose the NEWEST (least invested) and delete the
Conversation with `Preconditions{ResourceVersion}`. Deleting the object, not
the pod, is what makes the pod-DELETE watch, the ConfigMap GC and housekeeping
all fall out of existing machinery — and it is the write that conflicts with
an adoption. A conflict means the reservation was just adopted: re-read and
take the next tier. The existing idle-eviction tier follows; `Pending` stays
last. `evictableCount` counts reservations as free, so `admit` never parks a
conversation behind them.

Alternative rejected: an in-memory pool lock shared by ingest and the
reconciler. It works in one process and is invisible state, which
`state-durability` forbids as the only copy of a decision.

### 5. Refill belongs to the Pipeline reconciler, with a delay

The Pipeline reconciler already owns everything per-route (bound claims,
Ready). It gains: count `Reserved` conversations for this Pipeline; create up
to `warm − held` while `len(liveRuntimePods) < cap` and no `Pending`
conversation waits; delete surplus when `warm` shrinks, the Pipeline is not
Ready, or the snapshot changed (compare the reservation's materialised spec
against `SnapshotFor` now — replace, never patch, per `pipeline-model`). It is
enqueued from the same pod-DELETE watch the conversation reconciler uses, with
`RequeueAfter` of the refill delay (`WARM_REFILL_DELAY`, default 30 s) rather
than acting on the event itself — the delay is what stops a TTL-freed slot
being filled and evicted in the same minute.

Ordering between admission and refill is the Pending check: a refill pass that
sees any `Pending` conversation does nothing, and the conversation reconciler
admits on the same event. A race that lets one reservation in ahead of a
Pending conversation is corrected on the next pass by tier 1 of decision 4 —
the reservation is the first thing evicted.

### 6. Idle release for a reservation is manager-side

The runtime exits after `RUNTIME_IDLE_TTL_M` without work; a waiting
reservation would exit after one minute by default. The pod is therefore built
with the TTL disabled (a value the runtimes read as "never" — verify each of
the three, `itoa` currently maps `<=0` to `30`), and the manager releases the
pod itself once the conversation is adopted and `!NeedsWorker` for the
runtime's effective `idleTTLMinutes` since `status.lastActivity`. It is the
same predicate as `/exit` and eviction, applied on a timer, and it applies
only to warm-born pods (a label on the pod), so every other pod keeps the
runtime-side exit it has today.

Alternative rejected: a "hold" answer on `/work` that resets the runtime's
clock — three runtime images to change, for a behaviour the manager can carry.

### 7. Design A, deferred: a generic pod bound at `/work`

Would also serve resurrection: the sidecar mounts the store root and chooses
`<store>/<conv>` when the unit names the conversation; the agent learns
`CONVO_ID` from the unit; the workspace becomes a synced volume like context.
That last item changes what workspace persistence MEANS — durable at
checkpoint instead of at write — and touches every runtime. Kept on record as
a later phase, not folded in.

## Risks / Trade-offs

- [A reservation holds real resources for a route that stays quiet] → it is
  per-route and off by default; `installation.md` states the slot it costs.
- [Cap fully warm, burst arrives] → tier 1 eviction and adoption both serve a
  burst at zero cost; a burst of one Pipeline adopts, of another evicts.
- [Adoption writes inputs on an object the runtime is already polling] →
  `tryDispatch` reads the conversation fresh on every poll; the adopted
  object's inputs are pending and its phase is no longer `Reserved`, so the
  next poll dispatches.
- [A snapshot change replaces reservations, briefly dropping the pool] →
  accepted; the alternative is a reservation running a stale identity.
- [The console lists reservations as conversations] → they carry the phase
  and `pipelineRef`; the list filters or badges them (`console-guide.md`).
- [Housekeeping's transcript reclamation uses a grace period keyed on the
  context handle] → a reservation has no handle and no transcript; nothing to
  reclaim, nothing to protect.

## Migration Plan

Additive: a new optional field, a new phase value. `kubectl apply -f
chart/crds/` before `helm upgrade`, as every CRD change here (the API server
prunes an unknown `spec.warm` silently — `gotchas.md`). Rollback: set `warm`
to 0 everywhere, reservations are deleted, then downgrade.

## Open Questions

- The exact TTL value each runtime reads as "never" (claude and copilot
  `runtime.js`, ollama) — settled in the task that disables it, per runtime.
- Whether the console hides reservations by default or shows them badged —
  a presentation choice, decided when the view is touched.
