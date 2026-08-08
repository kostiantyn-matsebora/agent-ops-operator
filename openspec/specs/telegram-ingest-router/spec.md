# telegram-ingest-router

## Purpose

Telegram serves exactly one update stream per bot token: a second concurrent
`getUpdates` returns `409`, and passing an offset destructively confirms
updates for every reader. Origination and continuation therefore cannot each
poll for themselves. This capability covers the three-component split — one
router owning the poll loop and classifying each update locally by topic
presence, two receiving adapters that never poll — the delegation of offset
storage, the `ChannelAdapter` port parity that lets an adapter be pushed to,
and the chart bundle that ships it all.

## Requirements

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

The router SHALL forward updates verbatim and SHALL NOT read `Channel`
configuration. Chat-id matching and approver filtering SHALL remain in the
receiving adapters, each against its own contract listing.

The router MAY read its OWN served `SignalSource` — and nothing else — for its
forwarding targets and the env prefix its bot token is projected under, since
adapter CRs carry no configuration or env by invariant and a served CR's
`config` is therefore the only path per-deployment settings can travel.
Classification SHALL NOT depend on that read: no per-message manager
round-trip.

#### Scenario: Non-approved sender is filtered downstream

- **WHEN** an update arrives from a sender not in the served CR's approvers
- **THEN** the receiving adapter drops it silently, and the router is unaware of
  the policy

#### Scenario: Router reads no channel policy

- **WHEN** the router runs
- **THEN** it reads no `Channel` configuration and no chat id or approver list
  from the manager, and classifies every update without contacting it

#### Scenario: Updates are forwarded byte for byte

- **WHEN** the router forwards an update
- **THEN** the receiving adapter gets the Telegram update verbatim, so no field
  is lost to re-encoding on the way through

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

### Requirement: The chart delivers the whole stack as one bundle

The chart SHALL package Telegram as an embedded `telegram-bundle` subchart,
default off, matching the existing bundle subcharts. Enabling it SHALL render
the router, the signal adapter, and the channel adapter as adapter CRs and no
channel-type workload templates.

The bundle SHALL render the chat surface — the `Channel`, the chat
`SignalSource`, and the router's credential-carrying `SignalSource` — under an
explicit `surface.enabled` flag. Enabling it SHALL make every value the chart
cannot guess REQUIRED, failing the render and naming what is missing rather
than installing a surface that cannot work.

The bot credential SHALL be expressible two ways, exactly one at a time: a
reference to an existing Secret, or a supplied token from which the bundle
creates the Secret. Both forms SHALL be referenced by the sending `Channel` and
the polling router alike.

The bundle SHALL NOT render a `Pipeline`: wiring names a profile and a runtime,
which belong to the install rather than to a transport bundle. Because an
unclaimed source answers nobody, the chart SHALL tell the operator what to
apply.

#### Scenario: Disabled by default

- **WHEN** the chart renders with default values
- **THEN** no telegram resources are produced

#### Scenario: Enabling renders implementations only

- **WHEN** `telegram-bundle.enabled=true` with `surface.enabled` false
- **THEN** the output contains the three adapter CRs and no Deployment or
  Service for any of them — the reconcilers create those — and no `Channel`,
  `SignalSource` or `Secret`

#### Scenario: An enabled surface renders its objects

- **WHEN** `surface.enabled=true` with a chat id and a credential
- **THEN** the output also contains the `Channel`, the chat `SignalSource` and
  the router's credential source, with one Secret name referenced by both the
  sending Channel and the polling router — and still no `Pipeline`

#### Scenario: An incomplete surface is refused

- **WHEN** `surface.enabled=true` and the chat id is missing, or no credential
  is given, or both credential forms are given at once
- **THEN** the render fails naming exactly which value is wrong, instead of
  installing a surface that could never answer

#### Scenario: The bundle can own the Secret

- **WHEN** the credential is supplied as a token rather than a Secret reference
- **THEN** the bundle creates the Secret with key `botToken`, and both the
  Channel and the router reference it by name

#### Scenario: The unclaimed sources are announced

- **WHEN** the surface renders
- **THEN** the post-install notes state that nothing answers yet and print the
  `Pipeline` to apply, pre-filled with the rendered source and channel names
