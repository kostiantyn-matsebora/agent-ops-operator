# telegram-channel-adapter

## ADDED Requirements

### Requirement: Telegram runs as an external reference adapter, not in the manager
The manager SHALL contain no Telegram-specific code (no poller, no Bot API client, no bot-token reads). A reference adapter in `channel-telegram/` (own binary and image, precedent `runtime-claude/`) SHALL serve Channels with `type: telegram`, consuming the channel adapter contract: getUpdates long-polling, offset persistence, approver filtering by Telegram user id, topic creation via `createForumTopic`, and message sends with the existing HTML parse mode and general-topic fallback. Routing-visible behavior (commands, adoption, default profile, busy-acks) SHALL be unchanged from the in-process implementation.

#### Scenario: End-to-end Telegram flow through the adapter
- **WHEN** a Telegram user sends `/agents` to a bot whose adapter serves a `type: telegram` Channel
- **THEN** the profile listing arrives in Telegram, produced by the shared router and delivered via the adapter's `send` op handling

#### Scenario: Manager has no Telegram surface
- **WHEN** the manager runs with no Telegram adapter deployed
- **THEN** it performs no Telegram API calls and no bot-token secret reads, and non-Telegram channels work normally

#### Scenario: Approver filtering stays enforced
- **WHEN** a Telegram update arrives from a user id not in the channel's configured approvers
- **THEN** the adapter drops it without posting to `/channel/inbound`

### Requirement: Adapter owns its credentials and config parsing
The adapter SHALL read the bot token from its own environment/mounted Secret and parse its channel settings (chat id, approvers, polling enablement, feed thread) from `spec.config` of the Channels it serves, reporting config errors on the Channel's status condition rather than crashing.

#### Scenario: Invalid config is surfaced on the Channel
- **WHEN** a `type: telegram` Channel's `config` lacks a required field (e.g., chat id)
- **THEN** the adapter sets a not-ready condition with the reason on that Channel and continues serving other Channels

### Requirement: Single getUpdates consumer preserved
Exactly one getUpdates consumer per bot token SHALL hold at all times: the chart SHALL run the adapter single-replica with a `Recreate` strategy, and the documented migration SHALL sequence old-poller shutdown before adapter start so both are never live simultaneously.

#### Scenario: Upgrade never double-polls
- **WHEN** an install migrates from the in-process poller to the adapter following the documented steps
- **THEN** at no point do two getUpdates consumers use the same bot token

### Requirement: Chart deploys the adapter opt-in
The chart SHALL template the adapter Deployment (image, token secret name, resources) gated on `telegramAdapter.enabled`, default **false** (Telegram becomes opt-in; the default out-of-box channel is the web channel from the sibling change). The shared manager↔adapter auth token SHALL be provisioned by the chart and injected into both deployments as environment.

#### Scenario: Disabled by default
- **WHEN** the chart renders with default values
- **THEN** no Telegram adapter resources are produced

#### Scenario: Enabled renders a working pairing
- **WHEN** `telegramAdapter.enabled=true` with a bot-token secret name set
- **THEN** the rendered Deployment mounts the bot token, carries the shared adapter auth token, and runs `replicas: 1` with `strategy: Recreate`
