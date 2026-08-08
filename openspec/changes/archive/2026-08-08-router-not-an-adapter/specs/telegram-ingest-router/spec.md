# telegram-ingest-router — delta

## MODIFIED Requirements

### Requirement: The router holds no channel configuration
The router SHALL forward updates verbatim and SHALL NOT read `Channel`
configuration. Chat-id matching and approver filtering SHALL remain in the
receiving adapters, each against its own contract listing.

The router SHALL NOT read any CR, and SHALL NOT contact the manager at all. Its
forwarding targets and bot token SHALL arrive as environment, injected by the
chart that renders the two receiving adapters — that chart already knows both
Service names, so fetching them back through an authenticated contract taught it
nothing it did not render itself. The router therefore holds no adapter token
and is outside the adapter trust domain.

Missing configuration SHALL fail startup with a message naming what is absent,
rather than leaving the process idle.

#### Scenario: Non-approved sender is filtered downstream
- **WHEN** an update arrives from a sender not in the served CR's approvers
- **THEN** the receiving adapter drops it silently, and the router is unaware of
  the policy

#### Scenario: Router reads no channel policy
- **WHEN** the router runs
- **THEN** it reads no `Channel` configuration and no chat id or approver list
  from the manager, and classifies every update without contacting it

#### Scenario: Router reads nothing from the manager
- **WHEN** the router runs
- **THEN** it makes no request to the manager, holds no adapter token, and
  classifies every update from the update alone

#### Scenario: Updates are forwarded byte for byte
- **WHEN** the router forwards an update
- **THEN** the receiving adapter gets the Telegram update verbatim, so no field
  is lost to re-encoding on the way through

#### Scenario: Misconfiguration is loud
- **WHEN** a forwarding target or the bot token is absent from the environment
- **THEN** the process exits naming the missing values, so the pod crash-loops
  with the reason visible rather than running and doing nothing

### Requirement: One process reads a bot token's update stream
Exactly one component SHALL call `getUpdates` for a given bot token at any time.
Telegram ingest SHALL be split into a router that owns the polling loop and two
receiving adapters that never poll. The router SHALL classify each update
locally by topic presence and forward it in-cluster: no topic id → the signal
adapter; topic id present → the channel adapter. Classification SHALL require no
manager round-trip.

The single-consumer rule SHALL be enforced STRUCTURALLY: one router workload per
bot token, single-instance with a recreate rollout, rather than by one process
multiplexing several tokens. A router SHALL serve exactly one token.

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

#### Scenario: A rollout never overlaps two pollers
- **WHEN** the router workload is updated
- **THEN** the old instance is stopped before the new one starts

#### Scenario: Migration never double-polls
- **WHEN** an install migrates from an adapter-modelled router to the
  chart-owned workload following the documented steps
- **THEN** at no point do two consumers of one bot token run simultaneously

### Requirement: The chart delivers the whole stack as one bundle
The chart SHALL package Telegram as an embedded `telegram-bundle` subchart,
default off. Enabling it SHALL render the two RECEIVING adapters as adapter CRs.
The router SHALL be rendered as a chart-owned `Deployment` — it produces no
signals and serves no CR, so it is not an adapter and SHALL NOT have a
`SignalAdapter` CR or any served CR of its own. It SHALL carry no
ServiceAccount token.

The bundle SHALL render the chat surface — the `Channel` and the chat
`SignalSource` — under an explicit `surface.enabled` flag, together with the
router, since without a bot token there is nothing to poll. The chat
`SignalSource` SHALL take the surface's own name by default, so one name
identifies the whole surface.

The bot credential SHALL be expressible two ways, exactly one at a time: a
reference to an existing Secret, or a supplied token from which the bundle
creates the Secret. Both forms SHALL be referenced by the sending `Channel` and
the polling router alike.

The bundle SHALL NOT render a `Pipeline`.

#### Scenario: Enabling renders implementations only
- **WHEN** `telegram-bundle.enabled=true` with `surface.enabled` false
- **THEN** the output contains the two adapter CRs and no Deployment or Service
  for them — the reconcilers create those — and no router, `Channel`,
  `SignalSource` or `Secret`

#### Scenario: An enabled surface renders its objects
- **WHEN** `surface.enabled=true` with a chat id and a credential
- **THEN** the output also contains the `Channel`, the chat `SignalSource` under
  the same name, the bot Secret, and the router Deployment — and no
  `SignalAdapter` for the router and still no `Pipeline`

#### Scenario: Disabled by default
- **WHEN** the chart renders with default values
- **THEN** no telegram resources are produced

#### Scenario: An incomplete surface is refused
- **WHEN** `surface.enabled=true` and the chat id is missing, or no credential
  is given, or both credential forms are given at once
- **THEN** the render fails naming exactly which value is wrong, instead of
  installing a surface that could never answer

#### Scenario: The bundle can own the Secret
- **WHEN** the credential is supplied as a token rather than a Secret reference
- **THEN** the bundle creates the Secret with key `botToken`, and the Channel and
  the router Deployment both reference it by name

#### Scenario: The unclaimed sources are announced
- **WHEN** the surface renders
- **THEN** the post-install notes state that nothing answers yet and print the
  wiring to declare, pre-filled with the rendered source and channel names

#### Scenario: The router carries no Kubernetes identity
- **WHEN** the router workload renders
- **THEN** it disables ServiceAccount token automounting and no Role,
  RoleBinding or adapter token is created for it

## REMOVED Requirements

### Requirement: Offset persistence is delegated to a receiving adapter
**Reason**: Not removed as behavior — offset delegation is unchanged and still
required. This requirement is restated below because its final sentence ("The
router SHALL require no Kubernetes permissions and no ServiceAccount token") is
now carried by the bundle requirement above, where the workload is actually
declared.

**Migration**: The delegation itself is preserved verbatim in the requirement
below; only its home moves.

## ADDED Requirements

### Requirement: Offset persistence is delegated downstream
The router SHALL own the in-flight offset value but SHALL NOT own its storage:
it SHALL obtain the offset at startup and report each confirmed offset
downstream, where a receiving adapter persists it through the existing adapter
state API. Because storage never moves, replacing the router workload SHALL NOT
lose the cursor.

#### Scenario: Restart resumes from the persisted offset
- **WHEN** the router restarts
- **THEN** it resumes from the persisted offset rather than re-reading the full
  backlog

#### Scenario: A replayed batch is harmless
- **WHEN** the router crashes before an offset is persisted and replays a batch
- **THEN** signals collapse on fingerprint and the system converges

#### Scenario: Re-modelling the router keeps the cursor
- **WHEN** the router moves from an adapter CR to a chart-owned Deployment
- **THEN** the new workload resumes from the same persisted offset, because the
  offset lives in the channel adapter and was never the router's to hold
