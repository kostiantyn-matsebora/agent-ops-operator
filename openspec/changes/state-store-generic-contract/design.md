## Context

See proposal.md — Why. The starting point is what `state-store` (PR #454)
delivers: `internal/state` with `Store` and `SignalStore`, one `kube` backend
mapping them onto the custom resources, a conformance suite, and the manager's
call sites moved onto them. The facts this change rests on:

| Fact | Value |
|---|---|
| Kinds of manager-written state | conversations, runs, inputs, signals, cooldown windows, receipt counters, adapter key-value state |
| Of those behind the contract after `state-store` | all but adapter key-value state |
| Components reading state around the contract | console (conversations, signals), housekeeping (conversations) |
| Console fields read from a conversation | the whole spec and status, `runs[]` and `runs[].inputs[]` on every list row |
| Housekeeping fields read | `name`, `runtimeContextId` with `sessionId` dual-read, phase-blind |

Constraints kept:

- `invariants.md`: the manager mounts nothing and reads no Secret. Every
  credential compiles to `valueFrom` for the kubelet.
- `structure.md`: every component is its own Go module, and nothing outside
  `platform/manager/` imports its `internal/`.
- `terminology.md`: `/channel/state` and `/signal/state` already use the word
  "state" for adapter cursors.
- Zero behaviour change under `kube`, measured by the unchanged system pack.

## Goals / Non-Goals

**Goals:**

- The contract is generic, so adding a kind of state touches no backend.
- What the store holds, and what each kind can do, is readable from one
  struct and enforced by the compiler.
- No component reads state around the seam, enforced by a check in each module.

**Non-Goals:**

- Any backend other than `kube`. NATS and Postgres are later changes and are
  now honestly selectable.
- Reducing `kube`'s own cost. The bench decides that.
- Moving configuration kinds. Pipelines, Channels, SignalSources and the
  adapter CRs stay on the Kubernetes API for everyone.
- A general query API. Every read on the new surface traces to a read the
  console or housekeeping makes today.

## Decisions

### D-1: One operation set, typed collections, a closed inventory

`state-store` shipped two interfaces over two records. Its design rejected
generics as "a rewrite for a second type that shares only six small pieces".
That judged the cost of the second type and missed the cost of the tenth.

| Option | Rejected because |
|---|---|
| One interface per kind of state | an abstraction tied to what it abstracts: every new kind is a new interface in the contract, a new implementation in every backend and a new half of the suite, and nothing says what the store holds |
| One untyped collection over bytes | loses the type: an index of one kind can be handed to another, an event reaches the wrong reconciler, and nothing says which capability a kind has |
| Generic primitives, typed collections, one closed inventory | **chosen** |

```go
Collection[T]     Get  Create  Update (CAS)  Delete  List(Index[T])  Watch
Expiring[T]       Collection[T] whose records carry an expiry
Appended[P, T]    Append  List  Grow(id)  GetWithItems → (P, Revision, []T)  ListWithItems
Blobs             Put  Get  Delete
```

```go
type Stores struct {
    Conversations Collection[ConversationRecord]
    Runs          Appended[ConversationRecord, Run]
    Signals       Expiring[SignalRecord]
    Cooldowns     Collection[CooldownWindow]
    Receipts      Collection[Received]
    AdapterState  Collection[string]
    Blobs         Blobs
}
```

`Index[T]` is a named extractor typed on the record. `Grow` applies a merge
the declaration supplies, which the suite proves idempotent and commutative,
so it takes no revision and never conflicts. A field's type says exactly what
that kind can do: `Conversations` cannot expire, `Signals` cannot append.
Nothing is optional at runtime, because every backend passes the suite for
every field.

Go has no generic methods on an interface, so a backend exposes a generic
constructor: `kube.Open[T](client, Layout[T])`. `Layout[T]` maps the record
onto the existing object (kind, fields, status subresource, the labels that
serve an index, the expiry annotation, the owner-link hint) and is the one
place in the tree a backend names a kind. A later backend takes a `Decl[T]`
and a codec and names none.

### D-2: Domain operations live above the contract

`ReleaseSource`, the cooldown replace and the receipt count read more than
one record or decide something. They become helpers over typed collections in
the package that owns the decision, using `Mutate[T]` (get, change, CAS,
retry, nothing written for no change).

`ReleaseSource` lists the signals of a source in any phase through a fourth
signal index and deletes each under the revision it listed. The index is the
contract's growth for it, a lookup like any other.

### D-3: Cooldown windows and receipt counters are collections

Both keyed by the source's name, no index, no feed consumer, no expiry. A
source with nothing recorded reads not-found, which the helper reads as an
empty window or zero. Under `kube` the layout is the `SignalSource` status
fields written by the same merge patch as before.

### D-4: Adapter key-value state is state

`state-durability` classified adapter cursors under the Kubernetes API as
configuration. A cursor is written at run time by no operator, and losing it
means polling from the beginning or registering a webhook twice. The two
handlers become the only callers of `AdapterState`, keyed by surface, name and
key. The adapter contracts do not change. Under `kube` it is the same
annotation on the same object, so an adapter upgraded across this change reads
back the cursor it wrote.

| Alternative | Rejected because |
|---|---|
| A ConfigMap per adapter | a new object, a migration, and still the Kubernetes API |
| Leave it | contradicts moving the cooldown window, and its handlers are the last state writers the static check cannot see |

### D-5: Every consumer reads state through the manager

The read surface is derived from what the two readers read, not designed.

| Reader does today | Endpoint | Store operation |
|---|---|---|
| console seeds its cache, housekeeping lists every conversation (phase-blind) | `GET /state/conversations` → `{"revision", "items": [record with runs]}` | list of the whole set, with runs |
| console resolves a UID on a cache miss | `GET /state/conversations/{name}` | combined read |
| console watches Conversations, resuming and relisting | `GET /state/conversations/stream?since=` (SSE) | `Watch` |
| console seeds and watches Signals | `GET /state/signals`, `GET /state/signals/stream?since=` | list, `Watch` |

Not served, because nothing reads it: a single signal, a source's window and
counters. The console's Queues card reads `GET /status`, and the Configuration
page's SignalSource YAML is a passthrough of a configuration object. A
sources endpoint is added when a reader exists.

**Wire record.** A kind's published field names:

```json
{"name": "…", "id": "<opaque>", "revision": "<opaque>",
 "createdAt": "…", "deletionRequestedAt": null,
 "spec": { … }, "status": { …runs[] and runs[].inputs[] included… }}
```

A signal is the same with `expiresAt`. No `metadata`, labels or annotations
cross the wire. Under `kube`, `spec` and `status` are the object's own bytes
and the four identity fields are its UID, resourceVersion and timestamps.

| Alternative | Rejected because |
|---|---|
| A flat record (`phase` at the top) | a second published shape for one kind, and the console would re-map every field |
| The `kube` object verbatim, `metadata` included | puts Kubernetes metadata into a contract that forbids it |
| Runs fetched per event, list and feed runs-less | 552 conversations is 552 reads on every relist, and the inbox computes unread, last message, run count and errored from runs on every row |

The last row is why the contract grows once: a listing returns each record
with its appended items, and a put event carries them. Free under `kube`, a
join in any backend that keeps runs apart.

**Feed.** The activity stream's shape over the store's events: `put`,
`delete`, `bookmark`, `resync`, each frame's `id:` the revision to resume
from, keep-alive comments. At-least-once, no gap.

**Naming.** `/state/…` is the store's surface. `/records/` was rejected
("record" names the message record), `/store/` because readers get state, not
a store.

### D-6: A reader states its reach

| Caller | Presents | Granted by |
|---|---|---|
| master | `ADAPTER_TOKEN` | — |
| an adapter | its derived token | `spec.stateAccess: true` on its CR, default false |
| housekeeping | `STATE_READ_TOKEN` | the chart's `stateReadAuth` Secret |

An adapter without the field is refused by name. The state-read credential is
rendered the way `controlPlaneAuth` is, refused at render if equal to either
other credential, projected by the kubelet into the manager and housekeeping,
and opens `/state/*` alone. The manager reads no Secret.

| Alternative | Rejected because |
|---|---|
| any adapter token, as `/activity` uses | every Telegram, cron and Alertmanager adapter would read every transcript |
| the state-read token for the console too | a `Channel` has one credentials Secret, already spent, and the chart cannot render one random value into two Secrets |
| TokenReview of housekeeping's ServiceAccount | a cluster-scoped grant and an authenticator the manager does not have |

Housekeeping cannot hold a derived token and must not hold the master token.
The state-read credential reads every record for a job that needs two fields.
The contract forbids a field list, so the width is accepted and stated.

### D-7: Served by every replica

`leaderProxyMiddleware` exists for in-memory leader-only state. A store read
is neither: `Get` is consistent, `List` is the backend's own cache, and a
resume point is the backend's revision, not a process's. `/state/` joins
`/healthz` in the bypass, so a leader rollout no longer drops the console's
feeds. The NetworkPolicy admits housekeeping on 8080, since a DROP there looks
like a manager outage.

### D-8: The static check lives in each module

`structure.md` forbids importing the manager's `internal/`, so the manager's
check cannot walk the console or housekeeping. Each gets a stdlib `go/parser`
test that fails, naming file and line, on the strings `conversations`,
`conversationinputs` or `signals` outside its one state client file, and on
`cooldown`, `receivedTotal` and `lastReceived` anywhere. The console's `Kinds`
is asserted disjoint from the three plurals, and the chart Role render test
asserts the same list, so the two cannot drift apart. The manager's check
gains the adapter state annotation prefix.

### D-9: Sequencing

This change modifies capabilities `state-store` publishes, so it applies and
archives only after PR #454 merges and archives. Until then its
`state-store` delta cannot validate against a published spec, and it is
written against the text #454 will publish. The first task re-validates it
after #454 archives and rebases.

## Risks / Trade-offs

- [Reshaping interfaces the call sites already moved onto] → the call names
  stay (`Get`, `Update`, `List`, `Watch`, `Mutate`). Only the type of the
  handle changes, and the compiler walks every site. The suite's scenarios
  are the ones already written, run generically.
- [The console renders from a different source] → the browser API contract
  tests keep every assertion, fed by a fake of the state client over the same
  fixtures. The mocked-browser tier and `e2e-live` run unchanged.
- [Housekeeping reclaims everything if it reads an empty list] → the listing
  is phase-blind by requirement, and a failing listing removes nothing and
  exits non-zero.
- [An Expiring record vanishes from the console before the sweep deletes the
  object] → the contract's own rule. Nothing sets an expiry on a Conversation.
- [The Configuration page loses `namespace`, `labels`, `annotations` on two
  kinds] → manager-internal lookup keys and the expiry annotation. The
  alternative, removing both kinds from the Configuration inventory, is a
  larger UI change not taken.
- [The state-read credential is wider than housekeeping needs] → accepted
  (D-6).

## Migration Plan

No upgrade step under `kube`: objects, annotations and CRs are unchanged.
Adding `stateReadAuth` generates a credential on first install and is
generated on a connected upgrade only (the call-auth shape, so a cluster-less
render mints nothing). The console and housekeeping lose RBAC in the same
release that gives them the read surface, so a partial upgrade is the only
unsafe state: the manager rolls first, and the chart renders all of it in one
release. Rollback is the previous release.
