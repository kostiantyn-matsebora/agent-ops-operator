## MODIFIED Requirements

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
