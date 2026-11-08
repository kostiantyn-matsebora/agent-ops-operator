## Why

The state store has one backend, `kube`. It keeps every conversation, run,
signal, cooldown window and receipt counter as custom resources.

- The contract's own scoring ranked it last: 19 of 70 on write cost, memory
  at scale and CPU per write.
- The `state-store` change ranked NATS JetStream KV first of the
  alternatives at 65 of 70, and named it the next change.
- The contract is now generic (`state-store-generic-contract`, merged).
  A backend is one implementation of each capability interface plus a
  conformance run.

This change writes that backend and makes it the default.

## What Changes

- **New `nats` state backend** in `platform/manager/internal/state/nats/`,
  implementing every field of `state.Stores` over JetStream KV and Object
  Store. It passes the same generic conformance suite and bench as `kube`.
- **New chart bundle `chart/charts/nats/`** running a clustered, three-server
  JetStream `nats-server` with a volume per server, a PodDisruptionBudget, a
  credential (explicit, existing Secret or generated) and a NetworkPolicy admitting only the manager.
  No single server is a point of failure.
- **`stateStore.backend` defaults to `nats`**, and `stateStoreGuard` knows the
  new bundle. `kube` stays selectable and renders no bundle.
- **One-time import from `kube`** on the first start of the `nats` backend
  over an install that already holds records, so an upgrade does not lose a
  conversation.
- **BREAKING:** a default install no longer keeps conversations and signals as
  custom resources. `kubectl get conversations` stops showing live state.
  An adopter who relies on that sets `stateStore.backend: kube`.
- **BREAKING:** a default install now runs three more pods and three more
  volumes.
- **`nats` is the default and `kube` is the alternative.** The change is done
  when the `nats` backend passes the conformance suite, the bench shows it at
  or below `kube` on bytes written per run and allocations per write, and the
  system pack passes unchanged against it.

## Capabilities

### New Capabilities

- `state-store-nats`: the `nats` backend's mapping onto the contract, its
  revision, feed, expiry and index behaviour, its import from `kube`, and the
  bundle that runs its server.

### Modified Capabilities

- `state-store`: the `kube` backend is no longer the default and renders a
  bundle for no other backend. "Exactly one backend is selected" now names
  `nats` as the default and the bundle it enables.
- `state-durability`: one sentence of the state store home changes. The
  default backend keeps state on its own clustered volumes, not in the
  Kubernetes API.
- `component-network-isolation`: adds a requirement. The manager may reach the state server on its
  client port. No other component may.
- `chart-managed-secrets`: adds a requirement. The state server credential resolves from an
  explicit value, an existing Secret, then generation, and is stable across
  upgrades.

## Impact

**Code**

- `platform/manager/internal/state/nats/` (new), `cmd/manager/store.go`
  (backend selection, import), `go.mod` (`nats.go`, test-only `nats-server`).
- `chart/charts/nats/` (new), `chart/templates/_helpers.tpl`
  (`stateStoreBackends`, `stateBackend` default, guard), `chart/values.yaml`,
  `chart/templates/deployment.yaml` (connection env), `chart/Chart.yaml`
  (dependency and version).
- `platform/manager/test/system/` (a lane running the pack on `nats`).

**Reference docs made untrue**

- `docs/concepts.md` (where state lives), `docs/contracts.md` (`/state/*`
  backing), `docs/configuration.md` (`stateStore.*`, `nats.*`),
  `docs/security.md` (new trust boundary and control), `docs/testing.md`
  (the nats tier), `docs/CHANGELOG.md` (breaking default and upgrade steps),
  `docs/guides/` page for the bundle.
- `.claude/rules/structure.md`, `invariants.md` (state store homes),
  `gotchas.md`.

**Adopter site**

- `docs/installation.md` (the new workload, volume, uninstall residue),
  `docs/getting-started.md`, `docs/introduction.md` and the landing page where
  they say state lives in custom resources, and `README.md`'s component list.

**Dependencies**

- Implementation is sequenced after `state-store-generic-contract` is
  archived. Both changes modify the same `state-durability` requirement, so
  the delta here is written against the other change's text and is re-checked
  at the gate (task 1.1).
