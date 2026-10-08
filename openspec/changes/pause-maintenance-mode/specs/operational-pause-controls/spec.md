# operational-pause-controls

## Purpose

The shared shape of an operational pause: the `Paused` condition convention
common to `SignalSource`, `Pipeline`, `Coordinator` and `MaintenanceMode`,
and the `/admin/*` HTTP surface that toggles all four — the one write the
console and `aops-mcp-server` cannot make directly, since neither holds
Kubernetes write RBAC.

## ADDED Requirements

### Requirement: A Paused condition is a sibling of Ready, never folded into it

`SignalSource`, `Pipeline` and `Coordinator` SHALL each carry a status
condition of type `Paused`, independent of `Ready` / `Served` / `Wired`:

| Status | Reason | Meaning |
|---|---|---|
| `True` | `Paused` | `spec.paused` is `true` |
| `False` | `NotPaused` | `spec.paused` is `false` or unset |

Pausing an object SHALL NOT change its `Ready` / `Served` / `Wired`
computation. A misconfigured object and a deliberately paused one SHALL
remain two different signals to an operator reading `kubectl get` — the
first a condition the object got wrong, the second one an operator chose.

#### Scenario: Pausing a healthy object leaves Ready true
- **WHEN** a correctly wired, `Ready=True` Pipeline is paused
- **THEN** `Ready` stays `True` and `Paused` becomes `True`

#### Scenario: Resuming clears the condition
- **WHEN** a paused SignalSource, Pipeline or Coordinator has `spec.paused`
  set back to `false`
- **THEN** its `Paused` condition becomes `False` with reason `NotPaused`

### Requirement: The /admin/* surface performs pause, resume and maintenance toggles

The manager SHALL expose six verbs under `/admin/*`, each a `POST` with a
JSON body, patching exactly one spec field and returning the result:

| Verb | Path | Body | Patches |
|---|---|---|---|
| Pause a source | `POST /admin/pause-signal-source` | `{"name"}` | that SignalSource's `spec.paused` to `true` |
| Resume a source | `POST /admin/resume-signal-source` | `{"name"}` | that SignalSource's `spec.paused` to `false` |
| Pause a route | `POST /admin/pause-pipeline` | `{"kind": "pipeline"\|"coordinator", "name"}` | the named Pipeline's or Coordinator's `spec.paused` to `true` |
| Resume a route | `POST /admin/resume-pipeline` | `{"kind", "name"}` | the named Pipeline's or Coordinator's `spec.paused` to `false` |
| Set maintenance mode | `POST /admin/set-maintenance-mode` | `{}` | the honored `MaintenanceMode` singleton's `spec.paused` to `true` |
| Clear maintenance mode | `POST /admin/clear-maintenance-mode` | `{}` | the honored `MaintenanceMode` singleton's `spec.paused` to `false` |

A successful call SHALL respond `200` with the patched object's name and
its resulting `paused` value. A malformed body (missing `name`, missing or
unrecognized `kind`) SHALL be refused `400` naming what was missing. A
`name` that resolves to no such object SHALL be refused `404` naming it.

The two maintenance verbs SHALL act ONLY on the already-honored singleton.
Where none exists, the call SHALL be refused `404` naming its absence. The
manager SHALL NOT create one. That remains the chart's responsibility.

#### Scenario: Pausing a signal source
- **WHEN** an authenticated caller posts `{"name": "alertmanager"}` to
  `/admin/pause-signal-source`
- **THEN** that SignalSource's `spec.paused` becomes `true` and the
  response names it with `paused: true`

#### Scenario: Pausing a coordinator by kind
- **WHEN** an authenticated caller posts `{"kind": "coordinator", "name": "ops-lead"}`
  to `/admin/pause-pipeline`
- **THEN** the Coordinator named `ops-lead` is paused, and a Pipeline of the
  same name is untouched

#### Scenario: An unrecognized kind is refused
- **WHEN** a caller posts `{"kind": "agent", "name": "x"}` to `/admin/pause-pipeline`
- **THEN** the request is refused `400` naming the unrecognized kind

#### Scenario: Maintenance mode requires the chart-rendered singleton
- **WHEN** no `MaintenanceMode` object named `default` exists and a caller
  posts to `/admin/set-maintenance-mode`
- **THEN** the request is refused `404` naming the missing singleton, and
  no object is created

### Requirement: Two identity classes may call /admin/*, with different bounds

`/admin/*` SHALL accept exactly two presented bearer identities, the same
two classes the manager's other authenticated surfaces already use:

1. **An adapter token** — the manager's master adapter secret, or any
   derived `ChannelAdapter` token (the same acceptance `anyAdapterAuth`
   already applies to the introspection surfaces, and the console's own
   credential). This class MAY call all six verbs.
2. **A coordinator-rooted conversation's own token** —
   `coordinator:<name>:<conversation>`, derived and compared exactly as
   `/coordinate/*` already does. This class MAY call a verb ONLY when the
   calling conversation's OWN bound tool allowlist — what it was actually
   given at dispatch, never what it merely claims — contains the matching
   `mcp__aops__<tool>` pattern for that verb. The manager SHALL re-derive
   this from its own record of what was bound. The caller's declared
   `allowedTools` is never itself the permission.

A missing or non-matching token SHALL be refused `401`. An authenticated
conversation whose own bound tools lack the matching pattern SHALL be
refused `403`, naming which tool was required.

Unlike `/coordinate/invoke`'s bound to a Coordinator's own `agents[]` list,
no such narrower scope applies here. A named SignalSource, Pipeline,
Coordinator or MaintenanceMode is not a member of any conversation's own
subtree.

The bound-tool check is therefore the entire authorization for this class.
That is stated here deliberately, rather than left to read as a narrower
check that was simply never written.

#### Scenario: The console's credential reaches every verb
- **WHEN** the console calls `/admin/pause-pipeline` with its own
  channel-adapter token
- **THEN** the call succeeds, regardless of any conversation's tool bindings

#### Scenario: A bound tool authorizes a coordinator-rooted caller
- **WHEN** a conversation's materialized toolsets include
  `mcp__aops__pause_signal_source`, and it calls `/admin/pause-signal-source`
  with its own coordinator token
- **THEN** the call succeeds

#### Scenario: An unbound tool is refused regardless of the caller's own claim
- **WHEN** a conversation's materialized toolsets do NOT include
  `mcp__aops__set_maintenance_mode`, and it calls
  `/admin/set-maintenance-mode` with its own valid coordinator token
- **THEN** the call is refused `403` naming the missing tool, even if the
  caller's own `--allowedTools` declaration claims it

#### Scenario: Neither identity is presented
- **WHEN** a request to any `/admin/*` verb carries no bearer token, or one
  that matches neither class
- **THEN** the request is refused `401`
