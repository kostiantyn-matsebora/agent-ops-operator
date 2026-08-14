# telegram-channel-adapter

## Purpose

The reference Telegram channel adapter: an external, dependency-free process serving type=telegram via the adapter contract.
## Requirements
### Requirement: Telegram runs as an external reference adapter, not in the manager
The manager SHALL contain no Telegram-specific code (no poller, no Bot API client, no bot-token reads) **and no Telegram-specific presentation**: HTML composition, entity escaping, the 4096-character message limit, and forum-topic naming limits SHALL live in the adapter alone. A reference adapter in `channel-telegram/` (own binary and image, precedent `runtime-claude/`) SHALL serve Channels with `adapter: telegram`, consuming the channel adapter contract: offset persistence, approver filtering by Telegram user id, topic creation via `createForumTopic`, and message sends with HTML parse mode and general-topic fallback.

The adapter SHALL render each typed outbound message for Telegram: composing HTML from markdown bodies, escaping payload content so markup-bearing text cannot break parsing, splitting or truncating messages that exceed the transport limit, and deriving forum-topic names from the `ensure-topic` descriptor within Telegram's own naming limit. It MAY present an oversized `signal` payload as a document rather than text. Routing-visible behavior (commands, adoption, default profile, busy-acks) SHALL be whatever the shared router implements — the adapter adds no routing rules of its own, so a command naming a Pipeline reaches Telegram users through exactly the same path as any other surface.

#### Scenario: End-to-end Telegram flow through the adapter
- **WHEN** a Telegram user sends `/agents` to a bot whose adapter serves a `adapter: telegram` Channel
- **THEN** the listing of addressable Pipelines arrives in Telegram, produced by the shared router as a `notice` message and rendered by the adapter

#### Scenario: Manager has no Telegram surface
- **WHEN** the manager runs with no Telegram adapter deployed
- **THEN** it performs no Telegram API calls and no bot-token secret reads, and non-Telegram channels work normally

#### Scenario: Approver filtering stays enforced
- **WHEN** a Telegram update arrives from a user id not in the channel's configured approvers
- **THEN** the adapter drops it without posting to `/channel/inbound`

#### Scenario: Oversized message is split, not failed
- **WHEN** a message renders longer than Telegram's per-message limit
- **THEN** the adapter splits or truncates it and the operation completes successfully

#### Scenario: Markup in a payload does not break the post
- **WHEN** a signal payload contains `<`, `>`, or `&`
- **THEN** the adapter escapes it and the message posts with the content intact

#### Scenario: Topic name comes from the descriptor
- **WHEN** an `ensure-topic` descriptor would render a name longer than Telegram allows
- **THEN** the adapter shortens it and creates the topic

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
Exactly one getUpdates consumer per **bot token** SHALL hold at all times. The polling loop SHALL live in the telegram ROUTER component, not in the channel adapter: the router SHALL run one loop per distinct token, its workload SHALL run single-instance (replicas 1, Recreate), and the channel adapter SHALL NOT poll at all — it receives topic messages forwarded by the router and continues posting them to `/channel/inbound`. The documented migration SHALL sequence shutdown of the previously-polling adapter before the router starts, so two consumers of one token are never live simultaneously.

#### Scenario: Two bots poll independently, once each
- **WHEN** the router serves two channels with distinct projected tokens
- **THEN** it runs exactly two getUpdates loops — one per token — inside the single pod

#### Scenario: The channel adapter never polls
- **WHEN** the split stack is running
- **THEN** the channel adapter makes no getUpdates call, and its only Telegram API calls are sends and topic creation

#### Scenario: Upgrade never double-polls
- **WHEN** an install migrates from the polling channel adapter to the router-fronted stack following the documented steps
- **THEN** at no point do two getUpdates consumers use the same bot token

#### Scenario: Offset survives the split
- **WHEN** the migration completes
- **THEN** the router resumes from the offset the previous adapter persisted, rather than re-reading old updates

### Requirement: Chart deploys the adapter opt-in
The chart SHALL ship the Telegram adapter as a `ChannelAdapter` CR (gated on `telegramAdapter.enabled`, default **false**) instead of a bespoke Deployment template; the chart SHALL contain no channel-type-specific workload templates. The `ChannelAdapter` reconciler owns the workload, auth injection, and credential projection; the chart's remaining role is the CRD and the gated CR.

#### Scenario: Disabled by default
- **WHEN** the chart renders with default values
- **THEN** no Telegram adapter resources are produced

#### Scenario: Enabled renders only a CR
- **WHEN** `telegramAdapter.enabled=true`
- **THEN** the rendered output contains a `ChannelAdapter` for `adapter: telegram` and no Deployment for it (the reconciler creates the workload)

### Requirement: Chart-shipped ChannelAdapter declares the telegram config schema
The chart's gated telegram `ChannelAdapter` CR SHALL declare the config contract on its spec: a JSON Schema for `spec.config` declaring `chatId` (string, required), `feedThreadId` (integer), `approvers` (array of integers), and `pollingEnabled` (boolean), plus `credentialKeys` documenting `botToken` (not required — the `TELEGRAM_BOT_TOKEN` fallback exists). The declaration SHALL live beside the `image` reference in the same template so an image bump and its schema update travel in one diff. The adapter binary SHALL be unchanged — it plays no role in the declaration.

#### Scenario: Declaration matches the parser
- **WHEN** the chart renders with `telegramAdapter.enabled=true`
- **THEN** the `ChannelAdapter` for telegram declares exactly the fields the adapter's config parser accepts, with `chatId` required

#### Scenario: Misconfigured channel flagged before the adapter sees it
- **WHEN** a `type: telegram` Channel is created with `config: {}` while the declaring ChannelAdapter exists
- **THEN** the Channel gains `ConfigValid=False` naming `chatId` from the manager, in addition to whatever Ready condition the adapter later reports

#### Scenario: Adapter binary unchanged
- **WHEN** the telegram adapter image runs against a ChannelAdapter with or without the declaration
- **THEN** its behavior is identical — it neither reads nor publishes any schema

### Requirement: Adapter closes forum topics on close-topic operations
The Telegram channel adapter SHALL serve the `close-topic` operation by closing
the corresponding forum topic through the Bot API (`closeForumTopic`) for the
chat the Channel's config names, then completing the operation with an empty
result. A Bot API failure SHALL be reported as the operation's error and SHALL
NOT be retried by the adapter — the manager treats a failed `close-topic` as
terminal.

#### Scenario: Forum topic is closed
- **WHEN** the adapter receives a `close-topic` operation for thread id `9876`
- **THEN** it calls `closeForumTopic` for that message thread and completes the operation with an empty result

#### Scenario: Bot API failure is reported, not retried
- **WHEN** `closeForumTopic` returns an error
- **THEN** the adapter completes the operation with that error and does not retry it

#### Scenario: Duplicate close is tolerated
- **WHEN** the same `close-topic` operation id is delivered twice
- **THEN** the adapter completes it without treating an already-closed topic as a failure

### Requirement: The adapter marks a deleted conversation's topic and closes it again
On `delete-conversation` the Telegram adapter SHALL un-archive the forum topic
if it is closed, post the notice into it, and close it again.

All three steps are required by the transport: a closed forum topic REFUSES
`sendMessage`, and leaving it open after the notice would invite replies into a
conversation that no longer exists — replies the manager drops, because the
thread maps to nothing.

The adapter SHALL NOT delete the forum topic. The transcript above the tombstone
is what a person scrolls back to after an incident, and an archived topic already
refuses replies without destroying it.

#### Scenario: A deleted conversation's archived topic still receives its notice
- **WHEN** `delete-conversation` arrives for a topic that was archived by an earlier close
- **THEN** the topic is un-archived, the notice is posted, and the topic is closed again

#### Scenario: A live topic is marked and archived
- **WHEN** `delete-conversation` arrives for a topic that is still open
- **THEN** the notice is posted and the topic is closed

#### Scenario: The history survives
- **WHEN** a conversation is deleted
- **THEN** its forum topic and every message in it remain readable
