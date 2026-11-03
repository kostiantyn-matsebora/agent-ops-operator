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
- The console reading signals through the manager. Under `kube` its direct
  watch keeps working. That change lands with the first non-`kube` backend.
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
| finds by `initiationId`, `pipelineRef`+phase, phase | declared index set, closed |
| resumes as a new leader from state alone | at-least-once resumable feed |
| ends a terminal signal after a per-phase TTL measured from completion | `expiresAt` on a record, written in the update that makes the phase terminal (D-14) |
| appends runs, reads them rarely | append-only sub-records |
| stores a payload once, reads it once | blobs by key |
| one install per namespace | scope fixed at construction |
| writes two records with no atomicity today | no transactions |
| holds a conversation with a finalizer while it archives threads, and reads when it was asked to go | finalizers set, deletion-requested marker, delete that completes on release (D-11) |
| derives a topic op id and a workload owner link from the object's identity | opaque record ID, new on recreate (D-11) |
| orders admission and the idle clock by creation time | backend-minted `CreatedAt` (D-11) |
| marks a delivered channel, a tracked run and a routed member result on an existing run | one grow-only run update (D-12) |
| lists a parent's members, a word-chain's names, and every conversation | three more declared indexes: by parent, by word-chain, whole set |
| creates a signal under a name derived from `(source, signalId)` and reads AlreadyExists as the duplicate | create refused as already existing, in every phase (D-14) |
| writes a signal's status under the version it read and repeats the pass on a loss | CAS `Update` that writes status and expiry only (D-14) |
| counts a source's live signals, and finds the earlier live messages of one conversation | two non-terminal signal indexes beside the whole set (D-14) |
| removes a deleted source's signals through an owner reference | explicit `ReleaseSource` (D-14) |
| keeps a source's cooldown window across a restart | `GetCooldown` / `PutCooldown` (D-15) |
| reads a deprecated per-input agent override for one release | `InputItem.Agent` carried through, removed with the dual-read |

Provenance of the additions, counted by type-checking every Kubernetes
client call in the manager's non-test packages that takes a Conversation,
a ConversationInput or their lists (before section 4 moves any):

| Kind | Call sites |
|---|---|
| `Get` of a conversation | 30 |
| status write (`Status().Patch`) | 34, of which 4 only grow a run (D-12) |
| spec or metadata `Patch` | 6 (finalizer add and remove, queue prune, input append) |
| `Create` of a conversation | 1, behind `AllocateName`, reached from 3 callers |
| `Delete` of a conversation | 3 |
| `List` of conversations | 17: 4 by parent, 2 by `initiationId`, 1 by word-chain, 10 whole set (one across namespaces) |
| `Get`, `Create`, `Delete` of a ConversationInput | 3, 1, 1 |

Reads of object metadata the first contract could not express: the deletion
timestamp 5, the creation timestamp 4, the UID 1, finalizers 3. The
deprecated input agent override is read once. Nothing reads annotations,
generation or owner references of a conversation. The two labels the manager
writes are lookup keys and map from fields the record has.

Two declared indexes, by pipeline and phase and by phase, have no call site
in the manager today: its lookups filter the whole set. They stay declared,
and the section 4 migration decides whether a call site adopts them.

Provenance of the Signal additions, counted the same way over the non-test
packages (reading every call that takes a Signal, a SignalList or the
cooldown list, since this tree was not yet migrated):

| Kind | Call sites |
|---|---|
| `Create` of a signal | 1, `publishOne`, with the controller owner reference set beside it |
| `Get` of a signal | 1, `Reconcile`, from the API reader |
| status write (optimistic-lock merge patch of the status) | 1 helper, reached from 6: the terminal-signal backfill, three in `consume`, `exhaust`, `evict` |
| `List` of signals | 2: `isHead` (earlier live messages of one source and initiation id) and `enforceSourceCap` (live signals of one source). Both list the whole namespace and filter |
| `Delete` of a signal | 1, the TTL delete, conditional on the UID and revision read |
| watch | 1 `For(&Signal{})`, plus a channel source (`kickSignal`) that hands another signal to the queue |
| type referenced as an in-memory value | 37: the signal reconciler 19, `signalroute` 17 (`Admit`, `Route` and their helpers mutate status in memory), `publish.go` 1 |
| `SignalSource.status.cooldown` | 1 read (`recoverCooldown`), 1 write (`recordCooldown`), reached from 2 (`spendCooldown` and its rollback) |

Reads of object metadata the first sketch of the contract could not express:
the creation timestamp 2 (`signalBefore`, ordering by arrival, and the
backfill), the UID and revision 1 (the conditional delete). Nothing reads a
Signal's labels, annotations, generation or finalizers, and nothing writes a
finalizer to one. Nothing reads the owner reference: the cascade from a deleted
source is the only use of it.

Not derived: the signal's `spec` is never written after publication (the
consumer writes status only), no signal write commutes (every one is a
compare-and-swap whose loser repeats its pass), and nothing appends to or
grows a signal, so none of the run sub-record machinery applies.

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

What is uncached under `kube`, and so costs an apiserver round trip where an
informer answered from memory: every `Get`, `List` and `ListRuns` (a read of
the whole Conversation, runs included, since the CRD keeps them in status).
`Reconcile` used to open with `Get` and then `ListRuns`, two reads of one
object. `GetWithRuns` is the combined read, one round trip under `kube`. Reconcile uses it. Every write re-reads before its compare-and-swap. Request-path
reads already went to the apiserver through `APIReader`. The new cost is the
reconciler's own reads. The expiry sweep lists the namespace only when armed. A write or an
observed record carrying `ExpiresAt` arms it, and it also runs once at start. An
install where nothing sets it therefore pays one list per process, not one per 30s.

The bench (`go test -run xxx -bench . -benchmem ./internal/state/...`) has two
tiers. The in-memory conformance store costs about 300 ns and 3 writes per run
append. It says nothing about apiserver latency. `BenchSignals` shows about
37 µs for a create plus two status writes, and 68 µs to list 200 live of 300
signals. The `kube` benchmarks run against envtest's real API server on one workstation (a
local apiserver and etcd, no kubelet, no network hop, no other client), 200
iterations each:

| Operation under `kube` | Measured |
|---|---|
| `Get` + `ListRuns` (the old Reconcile read), 5 runs | 1.69 ms |
| `GetWithRuns` (the same two facts) | 0.88 ms |
| Signal create + two status writes | 6.0 ms |
| `List(SignalNonTerminalBySource)`, 300 signals stored | 11.6 ms |

These are costs of the backend's own work against a local server, not cluster
latencies, and no cluster measurement was made. The last row is the finding: the
signal indexes filter in Go over one namespace list, so every cap or head-of-queue
check decodes every stored signal, live or terminal. It is bounded by rate times
TTL and is the first thing a selector or an index in a later backend would remove.
This change does not claim the signal path is cheap on a loaded cluster.

### D-5: The `kube` backend keeps today's CRD layout

Runs stay embedded in `status.runs[]`, inputs stay `ConversationInput`
objects.

The contract's append takes no revision and never surfaces a conflict. The
`kube` backend meets that with an internal read-append-CAS loop on the
status array, retried on conflict, since appends commute.

The cost: an append moves the control record's revision. The bench measures
the write amplification this causes.

Splitting runs into a `ConversationRun` CRD would remove that cost and would
be a CRD migration in a change whose acceptance criterion is zero behaviour
change. It stays open as a later `kube`-only optimisation, decided by the
bench.

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
volumes. Naming the backend after the apiserver's storage engine would name
something the adopter neither chooses nor sees. What they get is custom
resources — so `kube`.

### D-10: The conformance suite is a Go package every backend imports

`internal/state/conformance` exposes `Run(t, func() Store)`. The `kube`
backend runs it under envtest in `go test ./...`. A later backend runs it
in-process against its own embedded server. `Bench(b, ...)` reports bytes
written per run, allocations per write and resident records after a
listing, so the NATS decision is made from measurements.

A static check in the manager's test suite fails on any package outside
`internal/state/` that references the Conversation or ConversationInput
types through a Kubernetes client call. It also guards the Signal, its list,
the cooldown entry, the receipt counters (`receivedTotal`, `lastReceived`) and the `SignalSource` status field. Until the Signal call
sites move, `httpapi`, `controller` and `signalroute` sit in an explicit,
marked, temporary allowlist (`pendingSignalMigration`) that the migration
empties and deletes. It never excuses a Conversation. `RunSignals` is the
Signal half of the suite, and a backend passes both.

### D-11: Identity and deletion are part of the record

Migrating the reconciler found what the first contract could not say.

A record carries `ID` (opaque, backend-minted, stable for life, new on
recreate), `CreatedAt`, `Finalizers` and `DeletionRequestedAt`. The first
two and the marker are read-only: a write's copies are ignored, so a caller
can neither forge an identity nor un-mark a deletion.

`Delete` is a request. A record holding no finalizer goes at once. A held
record stays readable, marked, and the feed carries it as a change. The
`Update` that empties its finalizers removes it, and the feed carries a
delete. The alternative, a delete that always removes and a separate
"closing" phase, was rejected: the reconciler archives threads before the
record may go, and that hold must survive a manager restart and a leader
change, which only state on the record does.

On a marked record a finalizer cannot be added: `ErrDeleting`. Create under a
held key is `ErrAlreadyExists`. Under `kube` the marker and the finalizers are
the apiserver's own, so the mapping adds nothing. The last-release `Update`
writes status first, since the main write that drops the last finalizer
deletes the object.

`Update` now reads the stored object, checks its revision against the
caller's, and writes onto it, so a label or annotation the record does not
model survives, where before it was dropped.

Every instant the contract returns is UTC. The `kube` mapping copied a
`metav1.Time` through, and the client decodes those into the process's local
zone, so three round-trip tests failed under any zone but UTC.

### D-12: A run mutates only by growing

The reconciler and two handlers edit an existing run: mark it tracked, add a
delivered channel, mark it routed to a parent. `UpdateRun(key, runID,
RunUpdate)` takes exactly those three effects. Each only grows the run, so
the operation takes no revision, never conflicts, is idempotent and commutes
with `AppendRun` and with itself, the same story as D-5. Under `kube` it is a
read-apply-CAS loop on `status.runs[]`, and writes nothing when the run
already holds the effect.

A caller that clears the undelivered-reply marker on a thread in the same
write as marking the channel delivered now makes two writes: the run update,
then the record update. A crash between them leaves the marker set on a run already recorded
delivered. Section 4 owns showing the delivery pass tolerates that, per the
atomicity requirement.

An arbitrary run edit was rejected: a run is history, and an operation able
to rewrite it would need a revision and a merge story.

### D-13: How the call sites moved

Recorded after section 4, where the decisions were made against real code.

- **Writes.** Every merge patch became `state.Mutate` (get, change, `Update`
  with the revision read, read again on `ErrConflict`, ten attempts, a change
  that alters nothing writes nothing). The closure re-derives its change from
  the record it is handed, since a patch never conflicted and a closure does.
  The four run-growth sites use `AppendRun` / `UpdateRun`.
- **Reads.** The reconciler reads one record and its runs once per pass. The
  `kube` store reads through an uncached client in production (D-4), so a pass
  costs one more apiserver read than the informer cache did. The cost is the
  price of a revision the CAS can trust.
- **Event source.** `SetupWithManager` registers the store's feed as a
  `source.Func`, which controller-runtime starts under the leader lease like
  every other source. `Owns(&corev1.Pod{})` cannot survive the removal of
  `For(&Conversation{})` (the builder refuses `Owns` without `For`), so it is a
  `Watches` on the pod mapping the controller reference to the conversation:
  the same routing, spelled out. `MapRuntimePodToPending` is unchanged.
- **Scope.** A store is bound to one namespace (D-2), so the pod watch that
  once listed conversations across every namespace now ignores a pod outside
  the store's namespace. The manager's cache is already limited to its own
  namespace, so no event it delivered before is lost.
- **Owner link.** The pod and ConfigMap owner reference is built from the
  record's `ID`, name and kind (a `PartialObjectMetadata` as the owner).
  Nothing reads a Kubernetes object to produce it.
- **A delivery is two writes.** `markDelivered` is `UpdateRun(MarkDelivered)`
  then a record `Update` clearing the thread's undelivered-reply note. A crash
  between them leaves a run recorded delivered with the note still naming it.
  The note is informational (nothing else reads it), and the next delivery pass
  settles it without sending again. A crash before the first write loses
  nothing, since the reply is re-derived. Both are pinned by
  `TestDeliveryPassToleratesACrashBetweenTheRunMarkerAndTheNote`.
- **Run history bound.** The ten-run cap the work report applied itself is now
  the `kube` backend's, applied on `AppendRun`, because it is a limit of the
  stored object and every writer must stay inside it.
- **Unused index.** `ByPhase` is read by the backlog bound and the pod-freed
  wake-up. `ByAll`, `ByCausedBy`, `ByNameBase` and `ByConversationID` are read
  at their call sites. `ByPipelineAndPhase` has no caller yet. It stays: it is
  in the spec and the conformance suite, and a later backend serves it for a
  Pipeline-rooted sweep without a contract change.
- **The static check** (`internal/state/static_test.go`) flags any reference to
  a Conversation or ConversationInput type, which is stricter than a client
  call taking one, so holding the object in a variable cannot get around it.

### D-14: Signals get a second interface, not a wider one

`Store`'s `Get`, `Create`, `Update`, `List` and `Watch` are typed on
`ConversationRecord`. Three ways to add Signal were weighed:

| Option | Rejected because |
|---|---|
| One `Store` with signal methods beside the conversation ones (`GetSignal`, `CreateSignal`, ...) | doubles the surface of an interface every test double and the kube backend already implement, and ties every future signal backend to the conversation sub-records (runs, blobs) it has no use for |
| Generics: `Store[R Record]`, `Event[R]` | rewrites a contract the manager already moved onto, for a second type that shares only six small pieces |
| A second interface, `SignalStore`, over the shared pieces | **chosen** |

`SignalStore` shares `Key`, `Revision`, `Ref`, `EventType`, the four errors
and the UTC rule, the `expiresAt` semantics and the identity fields
(`ID`, `CreatedAt`), and adds only what the call sites above need:

- `Get`, `Create`, `Update`, `Delete`, `List`, `Watch` with the conversation
  store's meaning. `Update` writes **status and expiry only**: the publication
  fields, the identity and the creation instant are ignored, as the identity
  already is on a conversation. That makes the spec's write-once rule a
  property of the contract, not a convention.
- **Create is the duplicate arbiter.** `ErrAlreadyExists` in every phase, with
  no lookup, as `publishOne` does today.
- **No `Mutate` helper, no revision-free write.** A signal pass reads once,
  mutates in memory across `Admit` and `Route`, and writes with the revision
  it read, carrying the returned one into its second write. A loss repeats the
  pass through the work queue, since a get-modify-update loop would re-run a routing
  decision that already created a conversation. Nothing about a signal grows,
  so there is no `UpdateRun` analogue.
- **No finalizers, no marked state.** A signal holds none and `Delete`
  removes at once.
- **Three indexes.** `SignalAll` (the new leader's pass, the namespace-wide
  relist the feed starts with), `SignalNonTerminalBySource` (the cap) and
  `SignalNonTerminalByInitiation` (the head-of-queue check). The two live
  indexes exclude terminal records at the contract, so rate × TTL finished
  records of a flapping source do not cost the cap a scan. They filter in Go
  under `kube` as `Store.List` does.
- **Expiry replaces the TTL delete.** `reconcileTerminal` read the signal,
  waited until `completedAt` + TTL and deleted it conditionally on the revision.
  The consumer now writes `ExpiresAt = CompletedAt + ttl(phase)` in the
  update that makes the phase terminal, in the same compare-and-swap, so no
  second writer exists and the conditional delete has nothing to guard.
  Under `kube` the status subresource is written first and the expiry
  annotation second, only when it changed: a crash between them leaves a
  terminal signal with no expiry, which the existing backfill pass stamps.
  The reverse order could leave a live signal expiring. The sweep deletes with
  a UID and revision precondition. The TTL knobs stay on the reconciler.
- **The owner link is a hint, not a mechanism.** `Create` takes `SourceID`,
  the opaque identity of the source as the source's own object reports it,
  and `kube` turns it into the controller owner reference, so the stored object
  is the one publication always wrote and no read of the source is added.
  The cascade stops being load-bearing: `ReleaseSource` deletes every
  signal naming it in any phase and clears its cooldown window, idempotently,
  and the manager calls it when the source reconciler finds its source gone
  and when a pass finds the orphan. This is D-7's rule applied to signals.
  A name outlives its source (a prune and re-apply, a reinstall), so
  `ReleaseSource(source, sourceID)` takes the departed identity when the caller
  knows it: a signal recorded under another identity, and under `kube` the
  successor's window, stay, and each delete carries the UID it listed. The pass
  knows it (`SignalRecord.SourceID`). The source reconciler, which has only the
  name from a deletion, passes `""` and releases the whole name. What remains
  is the reconciler's own sub-millisecond gap between seeing the source absent
  and releasing, which a successor created inside it can lose.
- **Create replaces an expired record.** An expired record is invisible to
  `Get` and `List`, so refusing a create over it would acknowledge a re-sent
  `signalId` as a duplicate of something nobody can read. `kube` deletes it
  under the UID and revision it read and creates again, once. Losing either
  race is a duplicate.
- **The feed is the informer's replacement for the same reason as D-6.**
  `SignalReconciler` registers `Watch` as a raw source with the relist
  `pumpFeed` already does, and the `kick` channel stays as a second source.

The payload stays inline in the record. A status write re-encodes it, which is
today's cost. A backend may keep it elsewhere without the contract changing,
and moving it out is a later optimisation for the bench to justify, not a
change to make blind in a change whose criterion is zero behaviour change.

### D-15: The cooldown window is state and moves behind the contract

`SignalSource.status.cooldown[]` is written by the manager's router
(`recordCooldown`) and read back once per process per source (`recoverCooldown`).
It is not configuration and not derivable: it is the record that stops a
restart re-opening conversations for ids inside their window, and the signal
spec says cooldown keeps its persistence on the source.

**Decision: in scope.** Leaving it makes "the state store abstracts all manager
state" false at exactly the place that matters, because under any non-`kube`
backend the window would be the one decision input still held in the
Kubernetes API, written to an object the source reconciler and the console
watch re-read on every fresh admission. Moving it costs two methods and
changes nothing under `kube`: `PutCooldown` is the same status merge patch,
`GetCooldown` the same read, the stored list identical.

- `GetCooldown(source)` returns the window. An absent one reads empty, not an error.
- `PutCooldown(source, entries)` replaces the whole window, last write wins,
  no revision. The leader is the only writer and holds a per-source lock. A
  lost write costs a duplicate investigation after a restart, never a stuck
  signal, and the caller already logs and carries on. This matches a merge
  patch, so the contract does not invent a stronger guarantee than the
  behaviour it replaces. An unknown source reads empty, never `ErrNotFound`.
- It lives on `SignalStore`, not on a third interface: a window is read only
  where a signal is routed, and `ReleaseSource` clears both.

Also moved: `receivedTotal` and `lastReceived`. They decide no route, but they
are the last state the manager wrote onto a source's status from outside the
store, and the aim is that no manager state is left behind the seam. The
contract takes two operations, not a record: `RecordReceived(source, at)`
counts one routed signal (Total + 1, Last = at) and the backend serializes
concurrent consumers without losing an increment. `GetReceived(source)` reads
them, zero for a source that is gone. Under `kube` it is the revisioned merge
patch of the same two status fields the router always made, retried on
conflict, so the stored object is unchanged. The console keeps reading the
object directly under `kube`, as for the rest of the source. The `Served` and
`Wired` conditions stay where they are: they are configuration status the
source reconciler derives, not state a decision reads back.

## Risks / Trade-offs

- [Zero behaviour change across 29 write sites is a claim, not a property] →
  the system pack is the acceptance test, unchanged, plus the byte-identical
  object scenario in the conformance suite.
- [D-4 turns some cache reads into apiserver reads under `kube`] → only
  paths that used `APIReader` already pay it. The reconciler's own `Get` and
  `ListRuns` now do too (D-4 lists what stays uncached). The bench covers the
  in-memory store only. A cluster measurement is outstanding.
- [D-6 changes the reconciler's event source] → under `kube` the informer
  still feeds it, with a test asserting the same event sequence for a
  lifecycle as before.
- [A feed consumer keeps its resume point in memory] → on restart it resumes
  from "now" and relists, which is the same relist an informer does. The
  contract's gap scenario covers the connected-then-disconnected case.
- [A Signal's expiry is set once, at the terminal write, so a later change to a
  TTL chart value does not re-age signals already terminal] → their window was
  decided when they finished, which is the signal spec's own "from
  completedAt". The backfill pass stamps only a terminal signal with none.
- [Orphan release by the manager adds a call where an owner reference did the
  work for free] → one `ReleaseSource` on the source reconciler's not-found
  path, plus the missing-source branch the signal pass already has.
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
