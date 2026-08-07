# telegram-channel-adapter

## Purpose

The reference Telegram channel adapter: an external, dependency-free process serving type=telegram via the adapter contract.

## Requirements

### Requirement: Telegram runs as an external reference adapter, not in the manager
The manager SHALL contain no Telegram-specific code (no poller, no Bot API client, no bot-token reads). A reference adapter in `channel-telegram/` (own binary and image, precedent `runtime-claude/`) SHALL serve Channels with `adapter: telegram`, consuming the channel adapter contract: getUpdates long-polling, offset persistence, approver filtering by Telegram user id, topic creation via `createForumTopic`, and message sends with the existing HTML parse mode and general-topic fallback. Routing-visible behavior (commands, adoption, default profile, busy-acks) SHALL be unchanged from the in-process implementation.

#### Scenario: End-to-end Telegram flow through the adapter
- **WHEN** a Telegram user sends `/agents` to a bot whose adapter serves a `adapter: telegram` Channel
- **THEN** the profile listing arrives in Telegram, produced by the shared router and delivered via the adapter's `send` op handling

#### Scenario: Manager has no Telegram surface
- **WHEN** the manager runs with no Telegram adapter deployed
- **THEN** it performs no Telegram API calls and no bot-token secret reads, and non-Telegram channels work normally

#### Scenario: Approver filtering stays enforced
- **WHEN** a Telegram update arrives from a user id not in the channel's configured approvers
- **THEN** the adapter drops it without posting to `/channel/inbound`

### Requirement: Adapter owns its credentials and config parsing
The adapter SHALL resolve each served channel's bot token from the projected credential environment advertised by the contract's `credentialEnvPrefix` (env `<prefix>botToken`), falling back to the `TELEGRAM_BOT_TOKEN` environment variable for channels without `credentialsSecretRef` (hand-deployed back-compat). It SHALL parse its channel settings (chat id, approvers, polling enablement, feed thread) from `spec.config` of the Channels it serves, reporting config errors — including a missing token from both sources — on the Channel's status condition rather than crashing.

#### Scenario: Per-channel token resolved from projection
- **WHEN** a served Channel's listing entry maps `botToken` to a projected env var
- **THEN** the adapter uses that token for this channel's polling, topic creation, and sends

#### Scenario: Fallback token preserved
- **WHEN** a served Channel has no `credentialsSecretRef` and `TELEGRAM_BOT_TOKEN` is set
- **THEN** the adapter serves it with the fallback token exactly as before

#### Scenario: Invalid config is surfaced on the Channel
- **WHEN** a `adapter: telegram` Channel's `config` lacks a required field (e.g. chat id) or no token is resolvable
- **THEN** the adapter sets a not-ready condition with the reason on that Channel and continues serving other Channels

### Requirement: Single getUpdates consumer preserved
Exactly one getUpdates consumer per **bot token** SHALL hold at all times: the adapter SHALL run one polling loop per distinct token across its served channels (channels sharing a token share a loop), the adapter workload SHALL run single-instance (`singleton` via ChannelAdapter: replicas 1, Recreate), and the documented migration SHALL sequence old-workload shutdown before the reconciler-owned workload starts so two consumers of one token are never live simultaneously.

#### Scenario: Two bots poll independently, once each
- **WHEN** the adapter serves two channels with distinct projected tokens
- **THEN** it runs exactly two getUpdates loops — one per token — inside the single pod

#### Scenario: Upgrade never double-polls
- **WHEN** an install migrates from the helm-deployed adapter to the ChannelAdapter-managed one following the documented steps
- **THEN** at no point do two getUpdates consumers use the same bot token

### Requirement: Chart deploys the adapter opt-in
The chart SHALL ship the Telegram adapter as a `ChannelAdapter` CR (gated on `telegramAdapter.enabled`, default **false**) instead of a bespoke Deployment template; the chart SHALL contain no channel-type-specific workload templates. The `ChannelAdapter` reconciler owns the workload, auth injection, and credential projection; the chart's remaining role is the CRD and the gated CR.

#### Scenario: Disabled by default
- **WHEN** the chart renders with default values
- **THEN** no Telegram adapter resources are produced

#### Scenario: Enabled renders only a CR
- **WHEN** `telegramAdapter.enabled=true`
- **THEN** the rendered output contains a `ChannelAdapter` for `adapter: telegram` and no Deployment for it (the reconciler creates the workload)
