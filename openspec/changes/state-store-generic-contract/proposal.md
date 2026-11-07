## Why

The `state-store` change put conversation and signal state behind a contract,
but two things it shipped defeat the point of a seam:

- **The contract is tied to the kinds it abstracts.** It is two interfaces,
  `Store` over conversations and `SignalStore` over signals, the same six
  primitives written twice with domain operations bolted on (`GetCooldown`,
  `RecordReceived`, `ReleaseSource`). Every new kind of state is a new
  interface, a new implementation in every backend and a new half of the
  conformance suite, and nothing in the code says what the store holds.
- **Only the manager honours it.** The console reads conversations and
  signals straight from the Kubernetes API. Housekeeping lists Conversation
  custom resources to decide which workspaces are orphans: under any backend
  but `kube` it would see none and reclaim every workspace. Adapter cursors
  (`/channel/state`, `/signal/state`) are still annotations patched by the
  HTTP handlers around the store.

A NATS or Postgres backend cannot be selected honestly until both are fixed.

## What Changes

- The contract becomes **one operation set, generic over the record**,
  written once per backend: `Collection[T]`, `Expiring[T]`, `Appended[P, T]`
  and `Blobs`. What the store holds is one **closed, typed inventory**
  (`Stores`): conversations, runs, signals, cooldown windows, receipt
  counters, adapter state, blobs. A field's type names exactly the
  capabilities that kind has, so an absent capability is not callable.
- `Store` and `SignalStore` are deleted. Domain operations (the cooldown
  replace, the receipt count, the release of a deleted source) move above the
  contract as helpers in the package that owns the decision.
- The `kube` backend is one generic constructor plus a **layout** per
  collection, the only place it names a kind. Stored objects stay
  byte-identical.
- **Adapter key-value state** moves behind the contract as a collection. Its
  `kube` location, the `agentops.dev/adapter-state-<key>` annotation, does
  not change.
- The conformance suite becomes generic and is driven by the inventory, so a
  backend's test is one call and a kind of state cannot be left out of it.
- The manager serves a **state read surface** (`/state/*`): three reads and
  two resumable feeds. The console and housekeeping read conversations and
  signals only through it. Their Kubernetes grants on those kinds are removed.
- Reading it is a declared reach. The console, as an adapter, reads with its
  own token once its CR sets `spec.stateAccess` (default false). A reader that
  is not an adapter, today housekeeping alone, reads with a dedicated
  `stateReadAuth` credential. Every replica serves it, outside the leader
  proxy.
- The static check that no code reads state around the seam extends to the
  console and housekeeping modules.
- **BREAKING (additive, no upgrade step under `kube`):** housekeeping now
  needs the manager Service reachable from its pod and holds no Kubernetes
  grant. A hand-tightened NetworkPolicy would block it. The console's
  Configuration page no longer prints `namespace`, `labels` or `annotations`
  in the YAML of a Conversation or Signal. An optional field
  `spec.stateAccess` on each of the two adapter CRDs.

Depends on `state-store` (PR #454) being merged and archived first: the
`state-store` and `state-durability` deltas modify what it publishes.

## Capabilities

### New Capabilities

- `state-read-surface`: the manager's read-only surface for state, its two
  feeds, the wire record, the three credentials that open it, replica
  serving, and housekeeping deciding orphans from it.

### Modified Capabilities

- `state-store`: one generic operation set over typed collections and a
  closed inventory, the conformance suite driven by it, cooldown windows and
  receipt counters as collections, adapter state behind the contract, and
  "reached only through the contract" now covering every component.
  Published when `state-store` archives.
- `state-durability`: adapter cursors move from the configuration home to the
  state store home.
- `manager-introspection`: the "no CR proxying" rule narrows to configuration
  kinds, since state has its own surface.
- `console-topology`: conversations and signals come from the manager's state
  read surface, the remaining kinds from Kubernetes watches.
- `console-application`: the service fans in the state read surface in place
  of Kubernetes watches of state.
- `conversation-housekeeping`: the reclaiming workload reads conversations
  through the manager and holds no Kubernetes grant.
- `channel-adapter-lifecycle` and `signal-adapter-lifecycle`: the optional
  `spec.stateAccess` field that lets an adapter's token read state.

## Impact

**Code**

- `platform/manager/internal/state/` and `internal/state/kube/`: the
  capability interfaces, `Stores`, `Mutate`, the generic `kube.Open`, layouts,
  the generic conformance suite and bench.
- `platform/manager/internal/signalroute/`, `internal/controller/`,
  `internal/httpapi/`, `internal/chat/`: call sites take typed handles.
- `platform/manager/internal/httpapi/state.go` (new), `server.go`,
  `signals.go`: the read surface and the adapter state handlers.
- `platform/manager/api/v1alpha1/`: `stateAccess` on the two adapter specs.
- `platform/console/` and `platform/housekeeping/`: a state client replaces
  the Kubernetes watch and list of state.
- `chart/`: `stateReadAuth` Secret, `STATE_READ_TOKEN`, RBAC removed from the
  console and housekeeping, the NetworkPolicy admits housekeeping.

**Reference docs**

- `docs/contracts.md` (the state read surface, the introspection paragraph),
  `docs/console.md` (sources, feeds, RBAC), `docs/concepts.md` (adapter
  cursors home, `stateAccess`, restart-resilience rows), `docs/testing.md`
  (generic suite), `docs/configuration.md` (`stateReadAuth`),
  `docs/security.md` (housekeeping's trust boundary, re-run
  `threat-model.py`), `docs/crds/channeladapter.md` and
  `docs/crds/signaladapter.md` (generated), `docs/CHANGELOG.md`,
  `.claude/rules/structure.md`, `.claude/rules/invariants.md`,
  `.github/retired-vocabulary.json`.

**Adopter site**

- `docs/console.md` and `docs/console-guide.md` say where the console's data
  comes from and what it no longer needs. `docs/installation.md` if it lists
  cluster permissions. `docs/getting-started.md`, the landing page and
  `README.md` need no change: a default install renders the same pages.
