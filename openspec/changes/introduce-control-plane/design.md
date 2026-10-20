## Context

See `proposal.md` — Why.

The manager already serves `/signal/*`, `/channel/*`, `/work`, and
`/coordinate/*` from one process
(`platform/manager/internal/httpapi/server.go`).

Each has its own authentication, built the same way: a bearer token
checked constant-time with `crypto/subtle`, read once from an environment
variable at process start.

`/control-plane/*` follows that exact shape, as a new package and route
group, not a new process.

## Goals / Non-Goals

**Goals:**
- One named, additive HTTP surface for operational configuration, proven
  end-to-end before anything real depends on it.
- A token that is isolated from every surface that already exists.
- An RBAC pattern — manager writes, everyone else reads at most — stated
  for every future resident of this surface, not just this one kind.

**Non-Goals:**
- No real first resident. The placeholder proves the wiring and is
  deleted or renamed, never grown, once one is proposed.
- No per-caller or scoped tokens, no rotation story.
- No second container, ServiceAccount, or Role.
- No change to `pause-maintenance-mode` or `introduce-rbac`.

## Decisions

### A package and route prefix inside the manager, not a new component

`structure.md` treats a directory as a component, one container each.

`platform/manager` stays the only Go module with generated CRD types.
Nothing outside it imports `platform/manager`'s `api/` or `internal/`
packages.

A second container could not create or read the placeholder kind without
either importing that module, breaking the self-contained rule, or
re-implementing a Kubernetes client against a CRD it does not own.

Reusing the module boundary that already exists costs nothing a new
component would have to rebuild on its own: a ServiceAccount, a Role, a
Deployment, and a reason for all three to track the manager's release.

**Alternative considered:** a standalone `control-plane` component with
its own Role scoped to the placeholder kind.

Rejected. It would hold the same CRUD rights the manager's Role already
states, as a second identity with no capability the manager lacks, for a
surface that lives inside the manager's own process by design.

### A new dedicated token, not `ADAPTER_TOKEN` or the coordinator token

`ADAPTER_TOKEN` scopes a caller to `/channel/*` and `/signal/*`, plus
derived per-adapter tokens. The coordinator token scopes a caller to
`/coordinate/*`, derived per-conversation.

Neither is built to carry a third scope. Widening either to cover
`/control-plane/*` would let a credential already present in a running
pod — a channel adapter's or a coordinating agent's — reach a surface it
was never issued for.

A caller scoped to one surface must not reach another. A single flat
shared secret, its own environment variable, checked the same way
`adapterAuth` checks `ADAPTER_TOKEN`, keeps that isolation with the
simplest mechanism the other three surfaces already use.

**Alternative considered:** reusing `ADAPTER_TOKEN` with a new derived
scope, in the shape of `chat.DeriveAdapterToken`.

Rejected. Derivation ties a scope to an existing CR name, a ChannelAdapter
or a SignalAdapter. The control plane names no such CR. Inventing one
only to carry a token scope would be exactly the implicit wiring this
change exists to avoid.

### A disposable placeholder, not a real first resident

Proposing a real resident and proving the surface at the same time means
neither concern can be judged on its own.

A placeholder with no real-world meaning proves token auth, routing, and
the CRUD round trip in isolation. Once that round trip is read and works,
a real resident becomes a routing-and-RBAC addition behind an
already-proven door, not a correctness question asked for the first time.

**Alternative considered:** standing up `pause-maintenance-mode`'s
`MaintenanceMode` kind as the first resident directly.

Rejected, by scope. That change is in flight separately, with its own
CRD, fields, and `/admin/*` surface. Moving it here would make this
change responsible for a contract it does not own.

### The RBAC pattern this establishes

| Role | On `agentops.dev` kinds behind `/control-plane/*` |
|---|---|
| The manager's own Role | full CRUD (`get`/`list`/`watch`/`create`/`update`/`patch`/`delete`) |
| Any other component's Role | `get`/`list`/`watch` only, and only if it needs to read the kind at all |

No other component's Role gains a write verb on anything this surface
hosts. That holds now and for any future resident, unless a later change
states otherwise explicitly.

This change itself adds no reader. Nothing besides the manager touches
the placeholder kind.

## Risks / Trade-offs

**A disposable CRD sits in a CRD list a cluster operator may mistake for
permanent.**
→ `docs/concepts.md` and the type's own api doc comment both state
"scaffolding, to be replaced" in the first sentence. The kind's name
itself reads as a placeholder, not a feature.

**A fourth bearer-token scope is a fourth secret for an operator to
provision and rotate by hand.**
→ Matches the existing `ADAPTER_TOKEN` and coordinator-token precedent
exactly. The operational burden is one more chart value, not a new
pattern to learn.

**An unconfigured token could silently leave the surface open.**
→ Mirrored from `platform/console/auth.go`'s documented rule: an unset
token authorizes nobody and is indistinguishable from a wrong one. The
middleware refuses with 503 rather than skipping the check.

## Migration Plan

Additive only. No existing route, token, or CRD changes behavior.

1. The new CRD ships under `chart/crds/` (controller-gen output).
2. A new chart value sets the control-plane token, wired the same way
   `ADAPTER_TOKEN` is today.
3. The manager's existing `Role` in `chart/templates/rbac.yaml` gains the
   placeholder kind with full CRUD verbs. No new Role, no new binding.
4. On upgrade, a cluster operator SHALL run `kubectl apply -f
   chart/crds/` before `helm upgrade`. Helm never upgrades an existing
   CRD, so skipping this step leaves the placeholder kind absent, and
   `/control-plane/*`'s CRUD handlers fail against a kind the cluster
   does not have.
5. An operator who does not set the new token value gets a correctly
   refusing surface, per the "unconfigured authorizes nobody" rule. No
   action is required to stay safe on upgrade.

No rollback step beyond the ordinary chart rollback. The placeholder kind
has no other component depending on it, so removing the CRD and the Role
line reverts cleanly.
