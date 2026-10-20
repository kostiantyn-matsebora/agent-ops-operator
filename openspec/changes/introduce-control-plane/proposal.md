## Why

Every CR the manager writes today, it writes from its own reconcile loop.
Nothing else can ask the manager to change state on purpose.

"Configure system behavior" has no door of its own, only `kubectl apply`
or a Helm value read once at startup.

`pause-maintenance-mode` already needs that door. It solves this narrowly,
bolting `/admin/*` onto the manager's existing channel-adapter-authenticated
surface.

Left unaddressed, every future operational-configuration need invents its
own route and reuses whichever credential is lying around. This change
gives the manager one deliberate, named surface instead, proven end-to-end
with a disposable placeholder before anything real depends on it.

## What Changes

- Add a new package, `platform/manager/internal/control-plane/`, and a new
  HTTP route group, `/control-plane/*`, registered on the manager's existing
  HTTP server alongside `/signal/*`, `/channel/*`, `/work`, and
  `/coordinate/*`. This is a package and a route prefix, not a new
  component — same process, same container, same Deployment, same
  ServiceAccount as the rest of the manager.
- Add a new, dedicated shared bearer token for this surface, its own
  environment variable, checked constant-time, following the same shape as
  the existing `adapterAuth`/`coordinate` middleware. It is deliberately a
  single flat shared secret — not per-caller, not scoped, not derived —
  matching the `ADAPTER_TOKEN` precedent. It is a distinct credential from
  `ADAPTER_TOKEN` and the coordinator token: a caller holding one must not
  be able to reach the other's surface.
- Add one new CRD kind, explicitly a **disposable placeholder** (a trivial
  key/value shape with no real-world meaning), to exercise the full round
  trip this surface exists to prove: token presented → handler invoked →
  object created/updated → object read back. It is documented as scaffolding
  to be replaced, not extended, once a real first resident is proposed.
- RBAC: add the placeholder kind to the manager's existing `Role` in
  `chart/templates/rbac.yaml` with the same full verb set
  (`get`/`list`/`watch`/`create`/`update`/`patch`/`delete`) every other
  manager-owned kind already has there. No new ServiceAccount, no new
  Role, no new binding — the manager keeps being the one identity with
  read+write on kinds it owns. State this explicitly as the pattern for
  whatever lands in `internal/control-plane/` later: the manager's Role
  holds full CRUD on it, every other component's own Role stays read-only
  (`get`/`list`/`watch`, never a write verb) if it needs to read it at all.
  Nothing else reads this placeholder, so this change grants no other
  component anything new.
- Explicitly out of scope:
  - Does not touch the in-flight `pause-maintenance-mode` change
    (`MaintenanceMode` CRD, `spec.paused` fields, its own `/admin/*`
    surface). That stays exactly as separately designed. It may become a
    resident of `/control-plane/*` later, but this change does not move it.
  - Does not touch the in-flight `introduce-rbac` change (`RoleGrant` CRD,
    console write RBAC). Unrelated surface, unrelated caller.
  - No per-caller or scoped tokens, no token rotation story, no second
    container or ServiceAccount — credible follow-ups, not this change.
  - No real capability behind the placeholder CRD. It carries no business
    meaning and is deleted or renamed, never grown in place, when the first
    real resident arrives.

## Capabilities

### New Capabilities
- `control-plane-api`: the `/control-plane/*` HTTP surface, its dedicated
  token authentication, the manager-exclusive read+write RBAC pattern for
  whatever CRD kinds live behind it, and the disposable placeholder kind
  used to prove the round trip.

### Modified Capabilities
None. This is a new, additive surface. No existing capability's behavior
changes.

## Impact

**Code**:
- `platform/manager/api/v1alpha1/`: new placeholder CRD type, generated
  deepcopy, and its CRD YAML under `chart/crds/`.
- `platform/manager/internal/control-plane/`: new package — the token-auth
  middleware and the placeholder's CRUD handlers.
- `platform/manager/cmd/manager/main.go` (or wherever the HTTP server wires
  route groups today): registers `/control-plane/*` and the new middleware.
- `chart/templates/rbac.yaml`: the placeholder kind added to the manager's
  existing `Role` block.
- A new chart value for the new shared token, wired the same way
  `ADAPTER_TOKEN` is today.

**Reference docs**:
- `docs/concepts.md` — the placeholder CRD, explicitly marked as disposable
  scaffolding, plus a short statement of the manager-exclusive-write RBAC
  pattern this surface follows.
- `docs/contracts.md` — the new `/control-plane/*` HTTP surface and its
  token authentication.
- `docs/CHANGELOG.md` — the new CRD, the new chart value, the
  `kubectl apply -f chart/crds/` upgrade step.

**Adopter site**: none of the adopter-facing pages (landing page, Getting
started, Installation, guides) describe manager-internal HTTP surfaces or
scaffolding CRDs, so none need a change for this proposal. This will change
once a real resident lands behind `/control-plane/*`.
