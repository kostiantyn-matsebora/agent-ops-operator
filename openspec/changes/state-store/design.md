## Context

See proposal.md — Why. The facts the approach rests on, measured on the live
install on 2026-10-29:

| Fact | Value |
|---|---|
| Conversations | 161, of which 108 Closed |
| Total JSON | 1.75 MB, `status.runs[]` is 1.32 MB of it |
| Largest object | 40 KB, growing without bound |
| Conversation write sites | 29 — controller 15, httpapi 7, chat 7 |
| Writes per run | 8–12, each a full-object status write |
| Manager RSS | 49 Mi leader, 31 Mi non-leader |

The constraint is not throughput. At most five active conversations, a run
every few seconds, signals in the tens per hour. What costs is per-write
overhead × object size × resident copies.

The constraints this design keeps:

- `invariants.md`: the manager mounts nothing, reads no secrets, every
  reconciler is strictly serial per conversation, one inflight unit.
- `state-durability`: a new leader resumes from stored state alone.
- `structure.md`: `platform/manager/` is self-contained, nothing outside it
  imports `internal/`.
- `worktree-delivery.md`: one change, one pull request, zero behaviour change
  measured by the unchanged system pack.

## Goals / Non-Goals

**Goals:**

- One seam for "where state lives", decided per install by one chart key.
- A contract that no backend shaped, enforced by one conformance suite that
  also benches.
- The `kube` backend byte-for-byte compatible with today, the default.
- Every later backend a bundle, never a parent-chart change.

**Non-Goals:**

- Any backend other than `kube`. NATS is the next change.
- Reducing the `kube` backend's own memory. Cache transforms and selectors
  are a separate, optional optimisation.
- The console reading state through the manager. Under `kube` its direct
  watch keeps working. That change lands with the first non-`kube` backend.
- Moving `SignalSource.status.cooldown[]`. It is state on a configuration
  object and moves with `signal-persistence`.
- A CRD layout change for runs. See D-5.

## Decisions

### D-1: Option B — state leaves the CRD model, CRs remain one backend

The interface holds domain records, not `v1alpha1` structs. The `kube`
backend maps records to the existing CRDs. Alternatives:

| Alternative | Rejected because |
|---|---|
| Stub CR + body in store | state in two places, the stub still churns on every claim and phase write, and the abstraction never becomes complete |
| Keep `v1alpha1` structs as the record type | every backend inherits `ObjectMeta`, labels, `resourceVersion` strings — the k8s shape leaks into the contract and every backend fakes it |
| Reduce the CRD footprint only (transforms, selectors, retention) | cuts our copies, never the apiserver's, and leaves no seam for an install to choose |

### D-2: The contract is derived from the manager, never from a backend

Every operation traces to a manager need in the code. The table is the
contract's provenance and lives in the spec's requirements:

| Manager does | Contract guarantees |
|---|---|
| get-modify-put raced by leader and HTTP handler | CAS on an opaque revision |
| finds by `initiationId`, `pipelineRef`+phase, phase, source | declared index set, closed |
| resumes as a new leader from state alone | at-least-once resumable feed |
| treats signals as expiring | `expiresAt` on a record |
| appends runs, reads them rarely | append-only sub-records |
| stores a payload once, reads it once | blobs by key |
| one install per namespace | scope fixed at construction |
| writes two records with no atomicity today | no transactions |

Credentials, TLS, DSNs, bucket names, schema migrations, retention and
compaction are a backend's own and live in its bundle. They never appear in
the contract.

### D-3: Feed events carry the record

An event is `(key, revision, record)`.

Alternative, keys only: rejected. A consumer fetches exactly the records that
changed, the same decode cost, plus a round trip per event.

Carrying the record lets the reconciler act on a snapshot and let CAS catch
staleness — how controller-runtime already behaves under `kube`.

### D-4: Reads are consistent, caches are the backend's business

`Get` returns the current record. The `kube` backend serves it from the
apiserver, the way `APIReader` does for the HTTP handlers today.

The reconciler acts on the fed snapshot and reads again only when it needs
the latest. The reconcile hot path gains no round trip.

### D-5: The `kube` backend keeps today's CRD layout

Runs stay embedded in `status.runs[]`, inputs stay `ConversationInput`
objects. The contract's append-only runs are a CAS update of the status
array.

Splitting runs into a `ConversationRun` CRD would be a CRD migration in a
change whose acceptance criterion is zero behaviour change. It stays open as
a later `kube`-only optimisation, decided by the bench.

### D-6: The reconciler is driven by the store's feed

`For(&Conversation{})` becomes `WatchesRawSource(source.Channel(...))` fed
by `Store.Watch`. Under `kube`, `Watch` is implemented over the informer so
the events and their ordering are the ones the reconciler sees today.
`Owns(&corev1.Pod{})` and `MapRuntimePodToPending` stay: a pod is a
Kubernetes object under every backend.

### D-7: Explicit workload release under every backend

The manager deletes a conversation's pod and ConfigMap on close and delete.
The `kube` backend still sets the owner reference as a second net.

Reason: an owner reference only works when the owner is a Kubernetes object,
and behaviour must not differ by backend. envtest runs no garbage collector,
which makes it the proof.

### D-8: Tiered scoring for the backend roadmap

Linear weights let a store buy back a bad memory score with a good CLI. The
model used, and recorded here for the next change:

| Tier | Attributes | Role |
|---|---|---|
| 1 — rank | write cost, memory at scale, CPU per write | the ordering |
| 2 — gate | durability ≥ 4, unattended recovery after a restart | fail = out |
| 3 — gate | OSI-permissive licence, maintained, runs in CI without a cloud | fail = out |

Gates removed Valkey (durability), MongoDB, SurrealDB and Consul (licence),
RethinkDB (dormant) and DynamoDB (cloud-only in CI, kept as an AWS-only
option). Survivors on tier 1, max 70:

| Backend | Tier 1 | Role |
|---|---|---|
| bbolt service | 70 | optional third, single-node, SPOF accepted |
| NATS JetStream KV | 65 | **first alternative** |
| FoundationDB | 51 | operability 1, no floor in the model, not pursued |
| etcd standalone | 46 | same semantics as NATS, heavier, not pursued |
| Postgres | 37 | **second**, bring-your-own, for installs that already run one |
| CRs | 19 | the baseline, this change |

### D-9: One chart selector, one bundle per backend

`stateStore.backend`, default `kube`. Each other backend is a bundle under
`chart/charts/<name>/` with `condition: <name>.enabled`, its own values
under `<name>.*`, and the parent's `stateStoreGuard` fails the render unless
exactly the selected backend's bundle is enabled. The manager gets
`STATE_BACKEND` and the selected bundle's connection env from the parent's
`deployment.yaml`.

Names rejected: `persistence.*` already means the context and workspace
volumes. `etcd` names the engine behind the apiserver, which on k3s is
kine, and what the adopter gets is custom resources — so `kube`.

### D-10: The conformance suite is a Go package every backend imports

`internal/state/conformance` exposes `Run(t, func() Store)`. The `kube`
backend runs it under envtest in `go test ./...`. A later backend runs it
in-process against its own embedded server. `Bench(b, ...)` reports bytes
written per run, allocations per write and resident records after a
listing, so the NATS decision is made from measurements.

A static check in the manager's test suite fails on any package outside
`internal/state/` that references the Conversation, ConversationInput or
Signal types through a Kubernetes client call.

## Risks / Trade-offs

- [Zero behaviour change across 29 write sites is a claim, not a property] →
  the system pack is the acceptance test, unchanged, plus the byte-identical
  object scenario in the conformance suite.
- [D-4 turns some cache reads into apiserver reads under `kube`] → only
  paths that used `APIReader` already pay it. The reconciler acts on the fed
  snapshot. Measured by the bench before and after.
- [D-6 changes the reconciler's event source] → under `kube` the informer
  still feeds it, with a test asserting the same event sequence for a
  lifecycle as before.
- [A feed consumer keeps its resume point in memory] → on restart it resumes
  from "now" and relists, which is the same relist an informer does. The
  contract's gap scenario covers the connected-then-disconnected case.
- [The static import check is a blunt instrument] → it names the package and
  the call, and allows `internal/state/kube` alone.
- [D-8's numbers are estimates] → that is why the bench exists. The next
  change ships only if the bench agrees.

## Migration Plan

Nothing to migrate. The `kube` backend reads and writes the objects that
exist. Rollback is the previous manager image. `stateStore.backend` absent
means `kube`.

## Open Questions

- Whether the `kube` backend should later split runs into a `ConversationRun`
  CRD (D-5). Deferred to the bench. Changes no spec of this change.
