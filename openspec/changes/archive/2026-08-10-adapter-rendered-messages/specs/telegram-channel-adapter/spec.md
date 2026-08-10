## MODIFIED Requirements

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
