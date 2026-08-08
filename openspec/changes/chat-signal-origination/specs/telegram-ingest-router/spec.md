## ADDED Requirements

### Requirement: One process reads a bot token's update stream

Exactly one component SHALL call `getUpdates` for a given bot token at any time.
Telegram ingest SHALL be split into a router that owns the polling loop and two
receiving adapters that never poll. The router SHALL classify each update
locally by topic presence and forward it in-cluster: no topic id → the signal
adapter; topic id present → the channel adapter. Classification SHALL require no
manager round-trip.

#### Scenario: Only the router polls

- **WHEN** the telegram stack is running
- **THEN** exactly one `getUpdates` loop exists per distinct bot token, in the
  router, and neither adapter polls

#### Scenario: General-surface message is an origination

- **WHEN** an update arrives with no topic id
- **THEN** the router forwards it to the signal adapter, which normalizes it and
  posts `/signal/inbound`

#### Scenario: Topic message is a continuation

- **WHEN** an update arrives with a topic id
- **THEN** the router forwards it to the channel adapter, which posts
  `/channel/inbound` with that thread id exactly as today

#### Scenario: Migration never double-polls

- **WHEN** an install migrates from the single polling adapter to the split stack
  following the documented steps
- **THEN** at no point do two consumers of one bot token run simultaneously

### Requirement: The router holds no channel configuration

The router SHALL forward raw updates and SHALL NOT read channel or source
configuration. Chat-id matching and approver filtering SHALL remain in the
receiving adapters, each against its own contract listing.

#### Scenario: Non-approved sender is filtered downstream

- **WHEN** an update arrives from a sender not in the served CR's approvers
- **THEN** the receiving adapter drops it silently, and the router is unaware of
  the policy

#### Scenario: Router needs no contract listing

- **WHEN** the router runs
- **THEN** it reads no channel or source configuration from the manager

### Requirement: Offset persistence is delegated to a receiving adapter

The router SHALL own the in-flight offset value but SHALL NOT own its storage:
it SHALL obtain the offset at startup and report each confirmed offset
downstream, where a receiving adapter persists it through the existing adapter
state API. The router SHALL require no Kubernetes permissions and no
ServiceAccount token.

#### Scenario: Restart resumes from the persisted offset

- **WHEN** the router restarts
- **THEN** it resumes from the persisted offset rather than re-reading the full
  backlog

#### Scenario: A replayed batch is harmless

- **WHEN** the router crashes before an offset is persisted and replays a batch
- **THEN** signals collapse on fingerprint and the system converges

#### Scenario: No RBAC for the router

- **WHEN** the router workload is created
- **THEN** it carries no ServiceAccount token and no Role or RoleBinding is
  created for it

### Requirement: Channel adapters can receive pushed updates

`ChannelAdapter` SHALL support `spec.port` with the same semantics
`SignalAdapter` already has: when set, the reconciler owns the Service and
injects the listen address into the workload.

#### Scenario: Reconciler owns the channel adapter Service

- **WHEN** a `ChannelAdapter` declares `spec.port`
- **THEN** the reconciler creates and owns the Service and injects the listen
  address, and the chart ships no Service for it

### Requirement: The chart delivers the whole stack under one flag

The chart SHALL render the router, the signal adapter, and the channel adapter
as adapter CRs gated on the existing `telegramAdapter.enabled` flag, default
false, together with a sample Pipeline pairing the chat `SignalSource` with its
`Channel`. The chart SHALL contain no channel-type workload templates.

#### Scenario: Disabled by default

- **WHEN** the chart renders with default values
- **THEN** no telegram resources are produced

#### Scenario: Enabling renders CRs only

- **WHEN** `telegramAdapter.enabled=true`
- **THEN** the output contains the three adapter CRs and no Deployment or
  Service for any of them — the reconcilers create those
