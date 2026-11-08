## Context

- The contract is `internal/state` (`Collection`, `Expiring`, `Appended`,
  `Blobs`, one closed `Stores`). `kube` is the only backend. See
  `state-store-generic-contract`.
- `cmd/manager/store.go` selects by `STATE_BACKEND` and builds `Stores` once.
- The console and housekeeping read through `/state/*`, so they are untouched.
- The manager mounts no volume and reads no secret (`invariants.md`).
- Pods are released by the manager, never by ownership, so a backend without
  owner references loses nothing.

## Goals / Non-Goals

**Goals:**

- A `nats` backend passing the unchanged conformance suite and bench.
- `nats` as the chart default, with a safe upgrade from `kube`.

**Non-Goals:**

- Multi-record atomicity. The contract offers none.
- Postgres, bbolt, an out-of-process store app.
- Removing `kube`, or deleting the CRDs it uses.
- Cross-cluster or leaf-node topologies.

## Decisions

### D-1: One KV bucket per kind, a client in `internal/state/nats`

Buckets: `conversations`, `runs`, `signals`, `cooldowns`, `receipts`,
`adapterstate`. Blobs use an Object Store. One bucket per kind keeps
per-kind history, limits and replica count independent.

| Alternative | Rejected because |
|---|---|
| One bucket, prefixed keys | One history limit and one watch for every kind |
| Raw JetStream streams | Reimplements CAS and per-key latest that KV gives |

The package is the only importer of `nats.go`. A static test extends
`TestNoManagerPackage…` so nothing else imports it.

### D-2: Revision is the KV entry revision

A KV revision is the stream sequence of the entry's last write. It is a
`uint64`, unique, and monotonic per bucket. `Update` is
`kv.Update(key, value, rev)`, `Create` is `kv.Create`. Both map their
wrong-last-sequence error to `state.ErrConflict` or `ErrAlreadyExists`.

`Watch` resumes with an ordered consumer starting at sequence `rev + 1`.
The feed stays at-least-once, as the contract says. A reconnect may redeliver
the last event, and consumers already tolerate it. A bookmark is a periodic
heartbeat event carrying the consumer's last delivered sequence.

### D-3: Envelope in the value, JSON

The value is a JSON envelope: `id`, `createdAt`, `finalizers`,
`deletionRequestedAt`, `expiresAt`, then the record. Identity, finalizers and
the deletion marker are backend-minted and ignored on write, as in the
contract. JSON keeps `nats kv get` readable. Size is judged by the bench, and
the envelope is the one place to swap the codec if it loses.

### D-4: Lists come from a watch-fed mirror, reads do not

A record is small, and the manager is the only client. Each collection keeps
an in-memory map fed by one watch and evaluates the declared `Index`
extractors over it. This is the same posture `kube` took with the informer
cache (`WithCachedLists`). `Get`, `Update`, `DeleteAt` read the server so a
precondition is never a guess. `List` blocks until the first catch-up
completes.

| Alternative | Rejected because |
|---|---|
| Index keys in the bucket (`idx/<name>/<value>/<key>`) | Two writes with no multi-key atomicity, so an index can lie after a crash |
| Subject-per-index filtering | KV watch filters on key only, and values change |

Cost: memory proportional to resident records. That is the bench's
`resident_records`, and the reason a terminal signal expires.

### D-5: Runs are entries, `Grow` is a CAS retry

A run is one entry `runs/<conversation>/<runId>`. `Append` is `Create`.
`Grow` reads the entry, applies the declared `Merge`, and writes under CAS,
retrying on conflict. The merge is idempotent and commutative, so a retry
converges and no conflict reaches the caller.

`GetWithItems` reads the parent and lists the prefix. The result is one
instant only to the parent's revision, which is the contract's promise.
`WatchWithItems` joins on the mirror.

### D-6: Expiry is a field plus a sweep

The envelope's `expiresAt` makes a record invisible at once on read and in
the mirror. A sweep, started with the backend like `kube`'s, deletes
expired entries and the feed carries the delete. NATS per-message TTL is not
used. It would need server 2.11+, and an expired entry would vanish without a
feed event the contract requires.

### D-7: Blobs in an Object Store

Queued input payloads can exceed the server's maximum message size, which an
Object Store chunks. `Put` replaces, `Delete` of an absent key is not an
error.

### D-8: The server is a bundle, three replicas by default

`chart/charts/nats/` renders a StatefulSet running `nats-server -js` with a
PVC per pod for the store directory. Values: `nats.replicas` (1 or 3),
`nats.persistence.size`, `nats.persistence.storageClass`, `nats.image`.

- Three replicas is the default and form a cluster. Every bucket and the
  object store are created with `R=3`, so one server can be lost with no
  outage and no loss.
- Replication is set at bucket creation by the manager, from
  `NATS_REPLICAS`. Changing it later is a documented rebuild, not a runtime
  action.
- A PodDisruptionBudget allows one server down. Anti-affinity is a
  preference (`preferredDuringScheduling`), so a one-node local cluster
  still schedules all three. On such a cluster the replicas share a failure
  domain, and the notes say so.
- One replica remains selectable. The render prints a chart note that the
  store is then a single point of failure.
- Quorum needs two of three. Losing two servers makes the store unavailable,
  not lossy. The manager's storage breaker holds work and reports an outage.
- The image comes from the official `nats` image, pinned by digest in
  `values.yaml`, multi-arch.

The credential is a user and password resolved in order: an explicit value,
then an existing Secret, then generation on install only. It follows the same
rule as `call-auth.yaml`. The manager receives
`NATS_URL` and `NATS_USER`/`NATS_PASSWORD` by `valueFrom`, so it reads no
Secret.

### D-9: The selector defaults to `nats`

`stateStore.backend` defaults to `nats`, and `nats` joins the list of backends
the guard knows. The guard accepts `nats` selected with the bundle enabled, and
`kube` selected with the bundle disabled.

The bundle's `enabled` is a derived default, not an adopter step. A
`stateStore.backend: kube` render SHALL NOT also need `nats.enabled: false`.
The parent sets the bundle's condition from the selector, so one value
changes backend. The guard still fails a hand-set contradiction.

### D-10: Import from `kube` runs once, in the manager, before serving

On start the `nats` backend checks the `_meta/imported` key in `adapterstate`.
If absent, it reads these kinds through the `kube` backend, opened read-only
for this, and copies each through the `nats` collections with `Create`:

| Source | Target |
|---|---|
| `Conversation` and its runs | `conversations`, `runs` |
| `ConversationInput` payloads | the blob Object Store |
| `Signal` | `signals` |
| `SignalSource` status cooldown window | `cooldowns` |
| `SignalSource` status receipt counters | `receipts` |
| `Channel` and `SignalSource` adapter-state annotations | `adapterstate` |

It treats already-exists as done and writes the marker last. The manager
serves nothing until it returns.

- Needs `get/list` on the kinds `kube` already holds. No new verb.
- Creates by existence. The one later write is the re-copy rule below, and it
  touches only a record the new version has not written. It never patches a
  Kubernetes object, in line with the storage invariant.
- Leaves the CRs. Deleting them is the adopter's act after verifying, and
  the docs say how.
- Revisions are not preserved. A resume token from before the import is
  invalid, and consumers start from now, which the contract already allows.

**The import races an old replica, and this is how it is closed.** During a
rolling upgrade an old-version replica can still write custom resources. It
creates Signals on `/signal/inbound` and appends inputs to Conversations on
`/channel/inbound`, on any replica, leader or not.

1. The import starts only after this replica holds the leader Lease, so the
   old leader's reconcilers have stopped.
2. Each copied record carries the source's `resourceVersion` in its envelope
   as `importedFrom`.
3. The import repeats a pass every 30 seconds. A pass creates what is missing.
   It also re-copies a record whose source `resourceVersion` differs from
   `importedFrom`, but only if the `nats` record has had no write of its own.
   A record the new version has already written is never overwritten.
4. The passes continue until the manager's pod list shows no manager pod of
   another version. One final pass then runs and the marker is written last.
5. The manager reports not ready until the marker exists.

| Alternative | Rejected because |
|---|---|
| No import, document the loss | A default flip that silently drops live conversations |
| A separate migration Job | A second binary holding state-store credentials and `kube` RBAC |
| Dual-write period | The contract has no multi-record atomicity to make it safe |

### D-11: `nats` is the default, and three gates define done

1. The conformance suite passes against the in-process server.
2. The bench shows `nats` at or below `kube` on bytes written per run and
   allocations per write. Numbers are recorded in the task, not asserted
   from the earlier scoring.
3. The system pack passes against a default install without a test edited.

`nats` is the default and `kube` is the alternative. The gates are this
change's acceptance criteria, not a condition on the default. A failing gate is
a defect in the backend to fix before the change is finished, never a reason to
ship the old default.

## Risks / Trade-offs

- **A new stateful workload on every default install** → three replicas with
  a PVC each, a PodDisruptionBudget, a readiness-gated manager, documented
  backup (`nats stream snapshot`) and the `kube` pin.
- **Three pods and three volumes cost more on a small cluster** → the
  resources are small, the notes document `nats.replicas: 1` as a conscious
  choice, and the anti-affinity is soft.
- **A rolling server upgrade can drop quorum** → the PodDisruptionBudget and
  an ordered StatefulSet update keep two servers up, proven by a system lane.
- **Breaking: `kubectl get conversations` shows nothing live** → the console
  is the supported viewer, the CHANGELOG states the pin, and the import
  leaves old CRs.
- **Mirror memory grows with resident records** → expiry bounds signals, the
  bench reports it, and the housekeeping GC still applies.
- **Server restart stalls the manager** → the storage breaker holds work and
  reports an outage before a loss (`terminology.md`, context continuity).
- **Import copies a stale view while an old replica still writes** → the
  repeated passes and the `importedFrom` re-copy rule of D-10 close it. A
  record both versions wrote during the roll keeps the new version's value.
- **One chart value flips two things (backend and bundle)** → the guard fails
  every contradiction and names the value.

## Migration Plan

1. Upgrade the chart. The new manager starts, finds the empty buckets and
   the `kube` records, imports, writes the marker, then serves.
2. Verify through the console and `/state/conversations`.
3. Optionally delete the old CRs, after a snapshot.

Rollback: set `stateStore.backend: kube` and upgrade. State written to `nats`
after the import is not copied back. The docs say so plainly and name a
rollback as a loss of conversations opened since the upgrade.

## Open Questions

- The default storage size per server. Settled from the bench's resident
  record figures, no spec depends on it.
