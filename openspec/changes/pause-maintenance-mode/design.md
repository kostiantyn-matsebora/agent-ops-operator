## Context

Three existing mechanisms this design reuses rather than reinvents:

- `internal/chat/pipelines.go`'s `readyPipelines`/`readyCoordinators` already
  gate fan-out on one status condition (`Ready`). Excluding a paused object
  is one more condition check in the same two functions.
- `internal/controller/conversation_controller.go` already holds new
  conversations in `Pending` during a storage outage
  (`enterPendingFor(ctx, &conv, ReasonStorageUnavailable, ...)`), touching
  neither pods nor topics, and never evicting work already running.
  Maintenance mode is the same shape with a different trigger.
- `platform/console/manager.go`'s `Reopen`/`Delete` already call manager
  verbs that are not `/channel/inbound`, over the console's own
  channel-adapter credential. The project's own comment there calls this
  "manager verbs over the same authenticated adapter path as everything
  else." `/admin/*` extends that path. It does not invent one.

See `proposal.md` for the motivation. See `specs/operational-pause-controls/spec.md`
for the full `/admin/*` HTTP contract and the condition-reason vocabulary,
and the other delta specs in this directory for the per-kind behavior.

## Goals / Non-Goals

**Goals:**
- Every pause/resume and the maintenance-mode toggle is a CR write. Nothing
  about enforcement depends on an environment variable, a Helm value read
  only at startup, or an in-memory flag a restart would lose.
- Console and `aops-mcp-server` are two callers of ONE new surface, not two
  independent mechanisms.
- A paused object fails CLOSED for routing. It is excluded from fan-out AND
  from direct addressing. It fails OPEN for anything already running. No
  eviction, no interrupted work.

**Non-Goals:**
- No per-source or per-pipeline RBAC model. Authorization for `/admin/*` is
  binary per identity class — a valid channel-adapter credential, or a
  coordinator-rooted conversation whose bound toolset names the verb. A
  finer-grained "which objects may this caller pause" policy is a credible
  follow-up, not this change.
- No scheduled or time-boxed maintenance windows. `spec.paused` is a flat
  on/off an operator, or an authorized caller, sets and clears by hand.
- No change to how a conversation ALREADY dispatched behaves. Maintenance
  mode and a paused Pipeline both affect what gets ADMITTED or ADDRESSED
  next. Neither touches a run already in flight.

## Decisions

### D-1: `Paused` is its own condition, not a reason on `Ready`

`Ready` on a Pipeline/Coordinator means "wiring resolves" today. A
SignalSource has no `Ready` at all — `Wired`/`Served` instead. Folding "an
operator turned this off" into any of those would make a dashboard read
"this is broken" for an object working exactly as configured.

A sibling condition keeps two questions independently answerable: is this
valid, and is this running. It costs each reconciler one more
`apimeta.SetStatusCondition` call beside the one it already makes.

**Alternative considered:** a `status.phase`-style enum. Rejected — none of
the three kinds has a phase today. A condition is additive: an upgrade
leaves old objects simply missing it, read as absent meaning not paused. A
new required enum field is not additive the same way.

### D-2: Fan-out is excluded by condition, addressing by an explicit check

`readyPipelines`/`readyCoordinators` gain one more filter clause (`Paused
!= True`), so every caller of `PipelinesForSource` is correct for free.

Addressing is different. `terminology.md` already documents that
`/<pipeline>` resolution is "a plain Get BY NAME — no claim check, no Ready
check" by design, so it has no existing gate to extend.

This change adds ONE new, narrow check there — paused, not Ready — rather
than making addressing start honoring `Ready` generally, which would be a
much bigger, unrelated behavior change.

### D-3: `MaintenanceMode` is a new CRD, not a ConfigMap or an env flag

Every other operational fact in this project is a watched CR. That is what
lets the manager react without a restart, and lets the console and
`aops-mcp-server` read and write the SAME object a `kubectl get` shows.

A ConfigMap would work for the read side, but the project has no precedent
for typing or condition-reporting on one. Ten of the console's watched
kinds are already CRDs.

An eleventh is a smaller conceptual jump than a kind of object the console
has never rendered.

**Alternative considered:** a well-known annotation on the manager's own
Deployment. Rejected — that couples enforcement to a workload identity the
console has no RBAC to read today, since it watches CRs, not workload
annotations.

It also reads oddly under GitOps, where an annotation is normally
generated rather than hand-edited.

### D-4: The singleton is a naming convention, not a webhook

The manager honors exactly the object named `default`. Anything else in
the namespace is read but never enforced, and says so in its own status.

This avoids standing up a validating webhook for one object. The cost is a
second `MaintenanceMode` silently doing nothing if someone creates one,
mitigated by the status condition making that visible rather than silent.

### D-5: `/admin/*` accepts two identity classes, with a server-side cross-check on the agent one

The console's channel-adapter bearer is already a precedent-bearing
credential for a manager verb that is not `/channel/inbound` (`Reopen`,
`Delete`, Context above). A coordinated agent's own token is the credential
`aops-mcp-server` already forwards for `/coordinate/*`.

Accepting a coordinator token for `/admin/*` with no further check would
let ANY agent wired into ANY Coordinator pause ANY SignalSource or Pipeline
in the install the moment the tool is merely listed.

`--allowedTools` is CLIENT-enforced, so a compromised or simply buggy
runtime could call the MCP server regardless of what its `allowedTools`
said.

The manager therefore re-derives, server-side, what that conversation was
ACTUALLY bound to at dispatch — the same `EffectiveAllowedTools` record the
runtime pod was given — and refuses the call if the matching
`mcp__aops__<verb>` pattern is absent from it.

This is the same argument this project already makes for pod-exec RBAC
(`.claude/rules/wiring.md`, `allowPodExecution`). A client's self-report is
never itself a security boundary. Here it applies to an API the manager
owns, not to the Kubernetes API.

**Alternative considered:** no cross-check, trusting operators to wire
these tools only into trusted Pipelines and Coordinators. Rejected — the
whole point of binding tools per Pipeline is that an install can safely
wire a capability's tools without auditing every agent's cooperation.

Removing that guarantee for exactly the six tools that can take the
install down is the wrong place to start trusting the client.

### D-6: The console reaches `/admin/*` from authenticated UI, never from the general chat composer

A chat command (`/close`, `/exit`) only ever affects the CONVERSATION it
was typed into. That is a bounded blast radius. Pausing a SignalSource or
flipping maintenance mode affects objects outside that conversation, and
outside that channel entirely.

Routing it through the same interception mechanism as `/close` would let
anyone who can message ANY bound channel disable anything in the install.

The console's own login (`authEnabled`) is the existing, narrower boundary.
"Whoever can administer this console" is already a smaller, intentionally
granted set than "whoever this install's pipelines let message a bot."

### D-7: Pause/resume is a deliberate, narrow exception to the console's read-only configuration view

`platform/console/ui/src/pages/Config.tsx` states its own position already:
"Read-only, and that is a position rather than a gap: Pipelines are the
wiring, the wiring is GitOps-managed, and a console that edits them
competes with helmfile."

That stance is about WIRING — what a SignalSource, Pipeline or Coordinator
is configured to do. `spec.paused` is not wiring. It does not change what
is wired, only whether a wired object is currently active.

That is the same distinction `kubectl cordon` makes for a node: it changes
schedulability, never the node's spec template.

So this change does not make the Configuration page editable in general.
It adds ONE narrow control — pause/resume and the maintenance switch — to
an otherwise still-read-only detail view.

It states why here, rather than leaving a reader of `Config.tsx` to wonder
why a "read-only" page has a button on it.

**Alternative considered:** keep the console fully read-only and reach
pause/resume only through `aops-mcp-server` and `kubectl`/GitOps. Rejected
— the proposal's own ask is console reach, and a maintenance toggle an
operator cannot find without a terminal defeats the purpose of having a
console during an incident.

## Risks / Trade-offs

- **[Risk]** A paused `SignalSource` still accepts the adapter's push and
  discards it, rather than refusing the HTTP call.
  **→ Mitigation:** deliberate (see the `maintenance-mode` and
  `signal-source-model` specs). A signal adapter retrying a rejected POST
  would turn a planned pause into a retry storm the moment it resumes. The
  response names the drop reason, so an adapter's own logs stay honest even
  though the HTTP status reads success.
- **[Risk]** `/admin/*`'s cross-check reads a record (`EffectiveAllowedTools`)
  computed at the conversation's last dispatch, which could lag a very
  recent Pipeline rewiring.
  **→ Mitigation:** the same staleness already exists for every other tool
  call in flight, since a runtime pod's `mcp.json` is fixed at pod start.
  This change does not widen that window. Closing it generally is out of
  scope here.
- **[Risk]** Maintenance mode holds NEW conversations `Pending` indefinitely
  if an operator forgets to clear it.
  **→ Mitigation:** no new risk. This is the same operator-visible
  `Pending` state the storage breaker already produces, surfaced
  identically in `kubectl get` and the console's queue view.
- **[Trade-off]** Two auth classes on one HTTP surface is more branching
  than a single scheme.
  **Accepted:** the alternative — a single shared secret both console and
  `aops-mcp-server` present — would mean handing the console's bearer to
  every coordinated agent, or the reverse. That is a larger privilege grant
  than either caller needs today.

## Migration Plan

- Additive only. `spec.paused` defaults `false` on all three kinds. An
  absent `MaintenanceMode` object reads as not-active. No existing manifest
  changes behavior on upgrade.
- RBAC changes only for the new kind. `chart/templates/rbac.yaml` already
  grants the manager full CRUD on `signalsources`, `pipelines` and
  `coordinators` specs (it reconciles claims onto Pipelines today), so
  `spec.paused` needs no new verb there. `MaintenanceMode` is added to that
  same resource list, plus its own `/status` entry. This is a chart change,
  not a data migration.
- Rollback is deleting the new CRD and reverting the three `spec.paused`
  additions. No stored data depends on either surviving. Both are pure
  operator-set flags with no derived state elsewhere.
