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

The router SHALL also consume the update kind Telegram uses for a selection made
on a message's controls, and SHALL classify it by the SAME rule, read from the
message the control was attached to. A selection on a general-surface message is
an origination; a selection inside a topic is a continuation. No second
classification rule SHALL be introduced.

A selection SHALL be acknowledged to Telegram immediately by the router. The
acknowledgement is unconditional, carries no content, and requires no
configuration — it is stream hygiene like the offset, and the router is the one
component that always holds the token. Acknowledging it downstream would require
giving a credential to a component that deliberately holds none.

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

#### Scenario: A selection is classified by the same rule
- **WHEN** a person selects a control on a general-surface message
- **THEN** the router forwards it to the signal adapter, by the same topic-presence
  rule it applies to a message

#### Scenario: A selection is acknowledged at once
- **WHEN** a selection arrives
- **THEN** the router acknowledges it to Telegram before forwarding, so the
  person's client does not wait on the downstream result

#### Scenario: The signal adapter still holds no credential
- **WHEN** a selection is handled end to end
- **THEN** the signal adapter contacts Telegram at no point, and holds no bot
  token

#### Scenario: A rollout never overlaps two pollers
- **WHEN** the router workload is updated
- **THEN** the old instance is stopped before the new one starts

#### Scenario: Migration never double-polls
- **WHEN** an install migrates from an adapter-modelled router to the
  chart-owned workload following the documented steps
- **THEN** at no point do two consumers of one bot token run simultaneously

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

### Requirement: The router's Bot API endpoint is configuration
The router SHALL resolve the Bot API base URL from the environment variable
`TELEGRAM_API_BASE`, defaulting to `https://api.telegram.org` when unset, and
SHALL use it for its `getUpdates` poll loop. The chart that owns the router's
Deployment SHALL inject it alongside the existing forwarding targets and bot
token, rendering no entry when unset.

Unlike the forwarding targets and the bot token — whose absence exits the
process at startup, because a router that cannot reach its downstreams or
authenticate is useless — an absent `TELEGRAM_API_BASE` is a normal
configuration, and SHALL take the default without comment.

#### Scenario: Default installs are unchanged
- **WHEN** the router runs with `TELEGRAM_API_BASE` unset
- **THEN** it polls `https://api.telegram.org` and starts normally, since the value is optional rather than required

#### Scenario: The poll loop honours the override
- **WHEN** `TELEGRAM_API_BASE` is set to a local endpoint
- **THEN** `getUpdates` is issued against that endpoint, and the router's classification and verbatim forwarding are unchanged

#### Scenario: Redirecting the endpoint does not create a second consumer
- **WHEN** the router is pointed at a local endpoint for testing
- **THEN** there is still exactly one `getUpdates` consumer per deployment, and the single-consumer invariant is unaffected by where it points
