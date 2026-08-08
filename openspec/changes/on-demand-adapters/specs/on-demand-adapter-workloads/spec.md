## ADDED Requirements

### Requirement: Adapter workloads run only when something demands them

An adapter's Deployment SHALL run at one replica only while its adapter has
demand, and SHALL otherwise be scaled to zero. Demand SHALL be defined per
adapter kind:

- a `ChannelAdapter` has demand when at least one `Channel` names it via
  `spec.adapter`;
- a `SignalAdapter` has demand when at least one `SignalSource` names it via
  `spec.adapter` **and** a Ready `Pipeline` claims that source.

Demand SHALL be computed from custom-resource state alone, so every manager
replica derives the same answer and a restart converges without warm-up.

#### Scenario: Declared but unused adapter costs nothing

- **WHEN** a `ChannelAdapter` exists and no `Channel` names it
- **THEN** its Deployment is scaled to zero

#### Scenario: Enabled bundle with no sources stays idle

- **WHEN** a `SignalAdapter` exists with a served `SignalSource` that no Ready
  Pipeline claims
- **THEN** its Deployment is scaled to zero, and its Service (if any) remains

#### Scenario: Claiming a source wakes the adapter

- **WHEN** a Ready Pipeline begins claiming a source served by a sleeping
  `SignalAdapter`
- **THEN** the Deployment scales to one

#### Scenario: A channel with no pipeline keeps its adapter awake

- **WHEN** a `Channel` names a `ChannelAdapter` and no Pipeline references that
  Channel
- **THEN** the adapter's Deployment stays at one replica, because a polling
  adapter must remain able to receive

#### Scenario: Demand does not depend on manager process state

- **WHEN** the manager restarts, or a non-leader replica serves requests
- **THEN** demand is computed identically from CR state, with no dependency on
  queued operations or conversation bookkeeping

### Requirement: Sleeping is scaling to zero, never deletion

Removing demand SHALL scale the Deployment to zero while leaving the Deployment,
its ServiceAccount, any owned Service, owner references, and credential
projection intact. Deleting the adapter CR SHALL still remove the workload as
before.

#### Scenario: The workload stays visible

- **WHEN** an adapter has no demand
- **THEN** its Deployment still exists and reports `0/0` replicas

#### Scenario: Deleting the CR still removes everything

- **WHEN** an adapter CR is deleted
- **THEN** its Deployment, ServiceAccount, and Service are removed

#### Scenario: Projection survives sleep

- **WHEN** a sleeping adapter regains demand
- **THEN** the pod starts with the same projected credential envFrom entries it
  would have had without sleeping

### Requirement: The Active condition explains the workload state

Both adapter kinds SHALL carry an `Active` condition: `True` with reason
`HasDemand` when running, `False` with reason `NoServedChannels` or
`NoWiredSources` when scaled to zero. The message SHALL name what would restore
demand — for an adapter whose sources exist but are unclaimed, the offending
source names. `Served` on `Channel`/`SignalSource` SHALL keep its existing
meaning and SHALL NOT be repurposed to report sleeping.

#### Scenario: Idle adapter explains itself

- **WHEN** an adapter is scaled to zero because its sources are unclaimed
- **THEN** `Active=False` with reason `NoWiredSources`, naming the unclaimed
  sources

#### Scenario: Served is unaffected

- **WHEN** an adapter sleeps
- **THEN** the `Served` condition on the CRs it serves is unchanged

### Requirement: Scale-up is immediate, scale-down is delayed

Gaining demand SHALL scale the workload up on the reconcile that observes it.
Losing demand SHALL scale the workload down only after demand has been absent
for an idle grace period, configured on the manager. The pending scale-down
SHALL be observable from the `Active` condition rather than held in process
memory.

#### Scenario: Rewiring does not thrash

- **WHEN** a Channel or Pipeline is deleted and recreated within the grace
  period
- **THEN** the adapter's Deployment is never scaled down

#### Scenario: Wake is not delayed

- **WHEN** demand appears for a sleeping adapter
- **THEN** it scales up without waiting for any grace period

#### Scenario: Countdown survives a manager restart

- **WHEN** the manager restarts while a scale-down is pending
- **THEN** the countdown resumes from the condition's recorded transition rather
  than starting over or scaling down immediately

### Requirement: A shared front-door adapter is active if any side has demand

When one adapter workload is the sole ingress for another adapter — such as a
router forwarding to both a signal adapter and a channel adapter — its demand
SHALL be the union of the demands it fronts. It SHALL NOT sleep while any
component depending on it for input has demand.

#### Scenario: Router stays awake for the channel side

- **WHEN** a router's own source is unclaimed but a `Channel` names the channel
  adapter it forwards to
- **THEN** the router stays at one replica, so the channel adapter does not go
  deaf

#### Scenario: All sides idle sleeps the whole set

- **WHEN** neither the fronted signal source is claimed nor any Channel names
  the fronted channel adapter
- **THEN** every component of the set is scaled to zero
