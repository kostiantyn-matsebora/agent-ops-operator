## Why

An operator has no way to stop one noisy signal source or one misbehaving
pipeline without deleting its CR.

There is also no way to pause the whole install for a maintenance window or
an incident, short of scaling the manager to zero. That drops in-flight work
ungracefully and leaves every CR reporting a health it no longer has.

Every other operational lever here is a CR field a reconciler reads live.
Pausing is not, and that gap grows as pipelines and coordinators multiply.

## What Changes

- Add `spec.paused` (bool, default `false`) to `SignalSource`, `Pipeline` and
  `Coordinator`. A paused `SignalSource` drops inbound signals before grouping
  or cooldown. A paused `Pipeline`/`Coordinator` is excluded from fan-out and
  from `/<name>` chat addressing — both claiming AND addressing, since
  addressing today bypasses the `Ready` check entirely.
- Each of the three gains a `Paused` status condition, independent of
  `Ready`/`Served`/`Wired`, so "misconfigured" and "deliberately stopped" are
  never the same signal to an operator reading `kubectl get`.
- Add a new CRD, `MaintenanceMode` — a namespaced singleton (`metadata.name:
  default`) the chart renders once per release. While `spec.paused: true`:
  the manager drops every inbound signal at ingest (before any per-source
  check) and holds every new conversation in `Pending` with a reason naming
  maintenance mode, without creating a runtime pod, an MCP ConfigMap, or a
  chat topic — mirroring the existing storage-breaker's conservative
  admission hold. Already-running conversations are left to finish.
  Nothing is evicted.
- Add a new manager HTTP surface, `/admin/*`, that performs these toggles as a
  CR patch on the operator's own behalf — the one write a console or an
  mcp-aops caller cannot make directly, since neither holds Kubernetes write
  RBAC. Two independent callers, two identity classes already used elsewhere
  in this surface: the console's channel-adapter credential, and a
  coordinator-rooted conversation's own token (the latter additionally
  cross-checked against that conversation's OWN bound tool allowlist before
  the manager acts, so advertising a tool is never itself the permission).
- Add six `aops-mcp-server` tools forwarding to `/admin/*`:
  `pause_signal_source`, `resume_signal_source`, `pause_pipeline`,
  `resume_pipeline` (pipeline or coordinator, by kind), `set_maintenance_mode`,
  `clear_maintenance_mode`. Reachable only when explicitly bound into a
  Pipeline's/Coordinator's `toolsets`, exactly like every other tool.
- Add console controls: a pause/resume toggle on a `SignalSource`,
  `Pipeline` and `Coordinator`'s detail view, and an install-wide maintenance
  switch, each calling `/admin/*` — gated behind the console's own
  authentication, never the general per-conversation chat composer (the
  blast radius of "anyone who can message a bound channel" is wrong for an
  action that affects objects outside that conversation). This is a
  deliberate, narrow exception to the Configuration page's existing
  read-only position (`Config.tsx`: "a console that edits them competes
  with helmfile") — pausing is an operational toggle, not a wiring edit.
- **BREAKING**: none. Every new field defaults to the current behavior
  (`paused: false`, no `MaintenanceMode` object required — its absence reads
  as not-paused).

## Capabilities

### New Capabilities
- `maintenance-mode`: the `MaintenanceMode` CRD, the singleton convention, and
  the manager's install-wide ingest/admission gate while it is active.
- `operational-pause-controls`: the `spec.paused` field and `Paused` condition
  shared in shape across `SignalSource`, `Pipeline` and `Coordinator`, plus the
  `/admin/*` HTTP surface both the console and `aops-mcp-server` call.

### Modified Capabilities
- `signal-source-model`: adds `spec.paused` and the ingest-time drop.
- `pipeline-model`: adds `spec.paused`, excludes a paused Pipeline from
  fan-out AND from `/<pipeline>` chat addressing (today's addressing
  bypasses `Ready` entirely — it must not bypass `Paused`).
- `coordinator-model`: the same two requirements, for `Coordinator`.
- `aops-mcp-server`: the six new tools and the bound-toolset cross-check that
  authorizes them server-side.
- `console-topology`: the per-object pause/resume controls and the
  maintenance-mode switch in the console UI.

## Impact

- **Code**: `platform/manager/api/v1alpha1/` (new field × 3, new
  `MaintenanceMode` kind + deepcopy + CRD YAML), `internal/controller/`
  (three reconcilers' `Paused` condition, a new `MaintenanceMode`
  reconciler, the conversation reconciler's admission hold),
  `internal/httpapi/` (`/signal/inbound`'s early drop, the new `/admin/*`
  surface), `internal/chat/` (`readyPipelines`/`readyCoordinators` honor
  `Paused`), `internal/chat/router.go` (the paused check in
  `resolveClaimant`/`HandleCommand`, the actual `/<pipeline>` addressing
  path), `platform/mcp-aops/` (six new tools + client
  calls), `platform/console/` (new UI controls, a new `Manager` client
  method, new `/api/*` routes behind console auth).
- **Chart**: a new `chart/crds/maintenancemode.yaml`, a rendered singleton
  instance, and RBAC adding `MaintenanceMode` to the manager's existing
  resource list in `chart/templates/rbac.yaml`. The manager already holds
  full CRUD on `SignalSource`, `Pipeline` and `Coordinator` specs, so
  `spec.paused` needs no RBAC change on those three.
- **Reference docs**: `docs/concepts.md` (three new fields, two new
  conditions, the `MaintenanceMode` kind in the CRD matrix and the state
  matrix), `docs/contracts.md` (the `/admin/*` HTTP surface), `docs/cr-reference.md`
  (regenerated — new kind, new fields), `docs/console.md` (the new
  endpoints and RBAC), `CHANGELOG.md`.
- **Adopter site**: `docs/console-guide.md` (what the new controls do and
  who can reach them), `docs/guides/` if an existing guide walks pipeline or
  signal-source creation (regenerated marker, not hand-edited).
- **Terminology/invariants**: the console write-path statement in
  `.claude/rules/structure.md` (the console has no Kubernetes write path
  and its only write is `POST /channel/inbound`) needs a line for
  `/admin/*`, with the same reasoning this proposal states. That is this
  repository's convention for revising an invariant rather than quietly
  going around it.
