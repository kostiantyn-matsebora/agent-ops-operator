## MODIFIED Requirements

### Requirement: Telegram runs as an external reference adapter, not in the manager
The manager SHALL contain no Telegram-specific code: no poller, no Bot API client, no bot-token reads. It SHALL also contain no Telegram-specific presentation — HTML composition, entity escaping, the 4096-character message limit, and forum-topic naming limits SHALL live in the adapter alone.

A reference adapter in `channels/telegram/` (own binary and image, precedent `runtimes/claude/`) SHALL serve Channels whose `spec.adapter` names its `ChannelAdapter` CR.

It SHALL consume the channel adapter contract: offset persistence on behalf of the router, approver filtering by Telegram user id, topic creation via `createForumTopic`, and message sends with HTML parse mode and general-topic fallback.

The adapter SHALL hold NO Telegram credential of its own (chat-ops-broker-telegram-gateway). Every Bot API call it composes SHALL be issued to the telegram-ingest-router's `POST /telegram/call/{method}` endpoint, never directly to the Bot API. The router is the sole holder of the bot token, both directions.

The adapter SHALL render each typed outbound message for Telegram: composing HTML from markdown bodies, escaping payload content so markup-bearing text cannot break parsing, splitting or truncating oversized messages, and deriving forum-topic names from the `ensure-topic` descriptor within Telegram's own naming limit.

It MAY present an oversized `signal` payload as a document rather than text.

Routing-visible behavior (commands, adoption, busy-acks) SHALL be whatever the shared router implements. The adapter adds no routing rules of its own, so a command naming a Pipeline reaches Telegram users through exactly the same path as any other surface.

#### Scenario: End-to-end Telegram flow through the adapter
- **WHEN** a Telegram user sends `/pipelines` to a bot whose adapter serves a Channel naming it
- **THEN** the listing of addressable Pipelines arrives in Telegram, produced by the shared router as a `notice` message and rendered by the adapter

#### Scenario: Manager has no Telegram surface
- **WHEN** the manager runs with no Telegram adapter deployed
- **THEN** it performs no Telegram API calls and no bot-token secret reads, and non-Telegram channels work normally

#### Scenario: Approver filtering stays enforced
- **WHEN** a Telegram update arrives from a user id not in the channel's configured approvers
- **THEN** the adapter drops it without posting to `/channel/inbound`

#### Scenario: Oversized message is split, not failed
- **WHEN** a message renders longer than Telegram's per-message limit
- **THEN** the adapter splits it into consecutive messages at Telegram's limit, truncating only a single unsplittable run, and the operation completes successfully

#### Scenario: Markup in a payload does not break the post
- **WHEN** a signal payload contains `<`, `>`, or `&`
- **THEN** the adapter escapes it and the message posts with the content intact

#### Scenario: Topic name comes from the descriptor
- **WHEN** an `ensure-topic` descriptor would render a name longer than Telegram allows
- **THEN** the adapter shortens it and creates the topic

### Requirement: Adapter owns its gateway binding and config parsing
The adapter SHALL resolve each served channel's GATEWAY BINDING — where the router is, and the shared secret that authenticates to it — from the projected credential environment (`<prefix>gatewayUrl`, `<prefix>gatewayToken`). It SHALL fall back to `TELEGRAM_GATEWAY_URL`/`TELEGRAM_GATEWAY_TOKEN` for channels without `credentialsSecretRef` (hand-deployed back-compat).

This REPLACES resolving a bot token (chat-ops-broker-telegram-gateway): the adapter holds no Telegram credential at all now, only the binding to the router that does.

It SHALL parse its channel settings (`chatId`, `approvers`, `feedThreadId`, `deleteTopicOnConversationDelete`) from `spec.config` — there is no `pollingEnabled`, because this adapter never polls — of the Channels it serves, reporting config errors — including an unresolved gateway binding — on the Channel's status condition rather than crashing.

#### Scenario: Per-channel gateway binding resolved from projection
- **WHEN** a served Channel's listing entry maps `gatewayUrl`/`gatewayToken` to projected env vars
- **THEN** the adapter sends this channel's Bot API calls through that gateway, authenticated with that token

#### Scenario: Fallback binding preserved
- **WHEN** a served Channel has no `credentialsSecretRef` and `TELEGRAM_GATEWAY_URL`/`TELEGRAM_GATEWAY_TOKEN` are set
- **THEN** the adapter serves it with the fallback binding exactly as before

#### Scenario: Invalid config is surfaced on the Channel
- **WHEN** a Channel naming the telegram adapter has a `config` lacking a required field (e.g. `chatId`) or no resolvable gateway binding
- **THEN** the adapter sets a not-ready condition with the reason on that Channel and continues serving other Channels

### Requirement: The gateway endpoint is configuration, not a compiled-in constant
The adapter SHALL resolve the router's call endpoint per channel exactly as it resolves `gatewayToken` (Requirement: Adapter owns its gateway binding and config parsing). There is no Bot API base URL on this adapter any more (chat-ops-broker-telegram-gateway).

`TELEGRAM_API_BASE` and the credential Secret's `apiBase` key are RETIRED on this component. The Bot API root is the ROUTER's concern now, specified by `telegram-ingest-router`'s own "The router's Bot API endpoint is configuration" requirement.

This generalizes the standing rule — **an adapter's outbound base URL is configuration, never a constant** — one level further: the OUTBOUND CALL ITSELF, not merely its base URL, is now a credential-projected value, because the adapter has no third party left to address directly.

#### Scenario: No Bot API base URL on this adapter
- **WHEN** the adapter's rendered Deployment or credential Secret is inspected
- **THEN** neither carries `TELEGRAM_API_BASE` or an `apiBase` key — only `gatewayUrl`/`gatewayToken`

#### Scenario: A channel cannot redirect its own gateway
- **WHEN** a served `Channel`'s `spec.config` contains a `gatewayUrl`, `apiBase` or `endpoint` key
- **THEN** it has no effect on where the adapter sends its calls

## ADDED Requirements

### Requirement: Outbound Bot API calls are relayed through the router
The adapter SHALL issue every Bot API call it composes to the resolved gateway's `POST /telegram/call/{method}`, bearing the resolved `gatewayToken` as a bearer token in the `Authorization` header, instead of calling the Bot API directly.

The relayed response SHALL be parsed exactly as a direct Bot API response would be, including `parameters.retry_after` on a `429`.

#### Scenario: Every outbound method is relayed
- **WHEN** the adapter sends a message, uploads a document, creates, closes, reopens or deletes a forum topic, or registers commands
- **THEN** each is issued as a call to the gateway's `/telegram/call/{method}` endpoint, never to `api.telegram.org`

#### Scenario: A relayed 429 is retried exactly as before
- **WHEN** the gateway relays a `429` with `parameters.retry_after`
- **THEN** the adapter waits and retries on the same terms this capability already specifies for a direct Bot API response

#### Scenario: An unreachable gateway is an explicit failure
- **WHEN** the dial to the gateway fails or times out for a relayed call
- **THEN** the adapter surfaces the call as failed and retries it on the same terms as a failed direct Bot API call, never reporting it as sent

## REMOVED Requirements

### Requirement: Adapter owns its credentials and config parsing
**Reason**: renamed and rewritten to "Adapter owns its gateway binding and config parsing". The adapter no longer resolves a bot token at all, only a gateway binding.

**Migration**: a served Channel's credential Secret now projects `gatewayUrl`/`gatewayToken` instead of `botToken`. The chart renders this automatically for the telegram bundle.

### Requirement: pollingEnabled channel setting
**Reason**: this adapter never polls. The telegram-ingest-router is the only `getUpdates` consumer, so the setting selects nothing.

**Migration**: delete `pollingEnabled` from the Channel's `spec.config`. A leftover key is ignored.

### Requirement: The Bot API endpoint is configuration, not a compiled-in constant
**Reason**: renamed and rewritten to "The gateway endpoint is configuration, not a compiled-in constant". This adapter no longer calls the Bot API directly, so it has no base URL of its own to configure. `telegram-ingest-router` owns the Bot API root now.

**Migration**: drop `telegram.apiBase`'s effect on the channel adapter. Set it on the router side only — already true, since `telegram-ingest-router`'s own requirement for this value is unchanged by this delta.
