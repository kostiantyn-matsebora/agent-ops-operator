# state-store

## Why

Conversation, ConversationInput and the coming Signal are **state**, not
configuration. Today they share the configuration kinds' storage — custom
resources — and pay its full price on every write and every copy:

- A one-field status write re-encodes the whole object through the manager,
  the apiserver's schema validation, kine's SQL translation and every
  informer. Roughly five encode/decode passes of a document that grows
  without bound, 8–12 times per run.
- Every object is resident four to five times: the apiserver's watch cache,
  kine's page cache, two manager informers and the console's cache. Closed
  conversations included, forever.
- `signal-persistence` will add a kind whose terminal records are bounded by
  rate × TTL. Under a flapping source that is thousands of objects, each
  with the same write and copy cost.

The storage engine is not the point. The absence of a seam is. There is no
place where "where state lives" is one decision, so it cannot be made
differently per install.

## What Changes

- A **state store contract** inside the manager: the only way the manager
  reaches a conversation, an input or a signal. Domain records, an opaque
  revision, compare-and-swap writes, a declared index set, a resumable
  at-least-once change feed, expiry, append-only sub-records, blobs. No
  Kubernetes types, no selector strings, no transactions.
- The contract is written from what the manager does with state, never from
  a backend's features. Every guarantee is one scenario of a **conformance
  suite**, and the suite is what a backend passes to exist. It also measures
  the tier-1 numbers — bytes per write, allocations, resident records — so
  the next backend decision is made from a table, not an estimate.
- The **`kube` backend**: today's custom resources, behind the contract,
  byte-identical objects, `kubectl get conv` untouched. The default. The one
  backend with no bundle.
- The manager consumes state through the contract everywhere — reconciler,
  HTTP API, chat, dispatch. The Conversation reconciler is driven by the
  store's change feed, not by a Kubernetes watch it owns.
- Pod and ConfigMap release becomes the **manager's explicit act** under
  every backend. The `kube` backend keeps the ownerRef as well, but no
  behaviour depends on garbage collection any more.
- Chart: one selector, `stateStore.backend`, exactly one value, the render
  refuses anything else. `kube` is the only value this change ships.
- Out of scope, each its own later change: the NATS JetStream KV backend
  (first), a bring-your-own Postgres backend (second), an optional bbolt
  single-node backend, the console reading state through the manager,
  `signal-persistence` rebased on the contract.
- **BREAKING**: none. Zero behaviour change is the acceptance criterion.
  The system pack passes unchanged against the `kube` backend.

## Capabilities

### New Capabilities

- `state-store`: the contract every state backend meets — records, CAS,
  indexes, feed, expiry, sub-records, blobs, scope — the conformance suite
  that defines it, the `kube` backend's behaviour-preservation, the chart's
  single-backend selection, and explicit workload release.

### Modified Capabilities

- `state-durability`: conversation state's declared home becomes the state
  store, which the `kube` backend resolves to the Kubernetes API. The
  restart-resilience matrix names the backend a row depends on.

## Impact

**Code**

- `platform/manager/internal/state/` — the contract, domain records, the
  `kube` backend, the conformance suite with its bench.
- `platform/manager/internal/controller/conversation_controller.go`,
  `index.go`, `drain.go`, `mediation.go` — reconcile from the feed, read and
  write through the store, explicit pod and ConfigMap release.
- `platform/manager/internal/httpapi/` (server, signals, workreport,
  channelread, contextreport, contextreset, coordinate),
  `internal/chat/` (router, ops, delivery, coordinate, pipelines),
  `internal/dispatch/`, `internal/namewords/`, `internal/control-plane/` —
  every Conversation and ConversationInput access moves to the store.
- `platform/manager/cmd/manager/main.go` — backend selection from
  `STATE_BACKEND`, store construction, reconciler wiring.
- `chart/values.yaml`, `chart/templates/deployment.yaml`,
  `chart/templates/_helpers.tpl` — `stateStore.backend`, the guard, the
  manager's env.

**Reference docs**

- `docs/concepts.md` — the state store as a declared home, the
  restart-resilience matrix gains its backend column.
- `docs/configuration.md` — `stateStore.backend`.
- `docs/testing.md` — the store conformance tier and its bench.
- `docs/CHANGELOG.md` — the new values key, no upgrade step.
- `.claude/rules/invariants.md` — "Filesystem state on a PVC, else
  Kubernetes API" becomes "else the state store", and the runtime-pod row
  stops naming ownerRef GC as the mechanism.
- `.claude/rules/structure.md` — `internal/state/` in the manager's map.

**Adopter site**

- `docs/configuration.md` is adopter-facing and carries the new key.
- `docs/installation.md`, `docs/getting-started.md`, the landing page and
  `README.md` do not change: a default install is identical. They change
  when the first alternative backend lands.
