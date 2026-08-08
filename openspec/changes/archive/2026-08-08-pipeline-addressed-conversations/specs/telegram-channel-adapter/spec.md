# telegram-channel-adapter — delta

## MODIFIED Requirements

### Requirement: Telegram runs as an external reference adapter, not in the manager
The manager SHALL contain no Telegram-specific code (no poller, no Bot API client, no bot-token reads). A reference adapter in `channel-telegram/` (own binary and image, precedent `runtime-claude/`) SHALL serve Channels with `adapter: telegram`, consuming the channel adapter contract: getUpdates long-polling, offset persistence, approver filtering by Telegram user id, topic creation via `createForumTopic`, and message sends with the existing HTML parse mode and general-topic fallback. Routing-visible behavior (commands, adoption, default profile, busy-acks) SHALL be whatever the shared router implements — the adapter adds no routing rules of its own, so a command naming a Pipeline reaches Telegram users through exactly the same path as any other surface.

#### Scenario: End-to-end Telegram flow through the adapter
- **WHEN** a Telegram user sends `/agents` to a bot whose adapter serves a `adapter: telegram` Channel
- **THEN** the listing of addressable Pipelines arrives in Telegram, produced by the shared router and delivered via the adapter's `send` op handling
