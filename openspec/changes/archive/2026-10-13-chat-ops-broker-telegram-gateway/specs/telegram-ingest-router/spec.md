## MODIFIED Requirements

### Requirement: One process reads a bot token's update stream
Exactly one component SHALL call `getUpdates` for a given bot token at any time. Telegram ingest SHALL be split into a router that owns the polling loop and two receiving adapters that never poll.

The router SHALL classify each update locally by topic presence and forward it in-cluster. No topic id goes to the signal adapter. A topic id present goes to the channel adapter. Classification SHALL require no manager round-trip.

The router is also the SOLE holder of the bot credential, inbound and outbound alike (chat-ops-broker-telegram-gateway). Its `POST /telegram/call/{method}` endpoint relays every outbound Bot API call on the channel adapter's behalf. The channel adapter never holds a copy of the token to send with.

The router SHALL also consume the update kind Telegram uses for a selection made on a message's controls. It SHALL classify it by the SAME rule, read from the message the control was attached to.

A selection on a general-surface message is an origination. A selection inside a topic is a continuation. No second classification rule SHALL be introduced.

A selection SHALL be acknowledged to Telegram immediately by the router. The acknowledgement is unconditional, carries no content, and requires no configuration. It is stream hygiene like the offset, and the router is the one component that always holds the token.

Acknowledging it downstream would require giving a credential to a component that deliberately holds none.

The single-consumer rule SHALL be enforced STRUCTURALLY: one router workload per bot token, single-instance with a recreate rollout, rather than by one process multiplexing several tokens. A router SHALL serve exactly one token.

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

### Requirement: The router's Bot API endpoint is configuration
The router SHALL resolve the Bot API base URL from the environment variable `TELEGRAM_API_BASE`, defaulting to `https://api.telegram.org` when unset, and SHALL use it for its `getUpdates` poll loop AND for every call it relays for the channel adapter.

The chart that owns the router's Deployment SHALL inject it alongside the existing forwarding targets and bot token, rendering no entry when unset.

This value SHALL never be settable on the channel adapter (`telegram-channel-adapter`'s own requirement for this capability is REMOVED): the router is the only Bot API caller left, so it is the only process with a base URL to configure.

An absent `TELEGRAM_API_BASE` is a normal configuration and SHALL take the default without comment, unlike the forwarding targets and the bot token, whose absence exits the process at startup.

#### Scenario: Default installs are unchanged
- **WHEN** the router runs with `TELEGRAM_API_BASE` unset
- **THEN** it polls and relays to `https://api.telegram.org` and starts normally, since the value is optional rather than required

#### Scenario: The poll loop honours the override
- **WHEN** `TELEGRAM_API_BASE` is set to a local endpoint
- **THEN** `getUpdates` and every relayed call are issued against that endpoint, and the router's classification and verbatim forwarding are unchanged

#### Scenario: Redirecting the endpoint does not create a second consumer
- **WHEN** the router is pointed at a local endpoint for testing
- **THEN** there is still exactly one `getUpdates` consumer per deployment, and the single-consumer invariant is unaffected by where it points

## ADDED Requirements

### Requirement: The router relays outbound Bot API calls for the channel adapter
The router SHALL serve `POST /telegram/call/{method}`, relaying the request body and its `Content-Type` to the real Bot API verbatim, and relaying the Bot API's own response — status and body — back verbatim.

This is the outbound half of the router holding the bot credential alone (chat-ops-broker-telegram-gateway). The channel adapter composes a Bot API call exactly as before. It issues the call to this endpoint instead of to `api.telegram.org` directly, and holds no copy of the token.

The router SHALL listen for the endpoint on the port set by the chart value `router.callPort` (default `8080`), and the chart's NetworkPolicy SHALL admit the channel adapter on that port.

The endpoint SHALL require the gateway token, a bearer token distinct from the router's inbound push to the channel adapter's `POST /updates`, which carries no bearer token.

It is the `gatewayToken` the channel adapter sends (Requirement: Adapter owns its gateway binding and config parsing).

The router SHALL read it from `TELEGRAM_CALL_TOKEN`. The chart projects that from the `gatewayToken` key of the credential Secret, generated once at install. An unset `TELEGRAM_CALL_TOKEN` SHALL exit the process at startup, naming it, the same as the forwarding targets and the bot token.

The channel adapter's existing inbound endpoint (`POST /updates` in `channels/telegram/main.go`, the router's push target, unchanged by this change) can only inject a fabricated inbound message, which the adapter still filters before acting on it — network-level trust is enough there.

This endpoint can SEND, with the live bot credential, any Bot API method at all. A caller must additionally prove it was handed the token.

#### Scenario: A JSON method call is relayed
- **WHEN** the channel adapter posts a `sendMessage`-shaped JSON body to `/telegram/call/sendMessage` with a valid bearer token
- **THEN** the router forwards it to the real Bot API and relays the response verbatim

#### Scenario: A multipart upload is relayed
- **WHEN** the channel adapter posts a `sendDocument` multipart body
- **THEN** the router forwards the body and its `Content-Type` unchanged, with no knowledge of the encoding

#### Scenario: A Telegram-side error is relayed, not absorbed
- **WHEN** the real Bot API answers `ok:false`, with or without `parameters.retry_after`
- **THEN** the router relays that response verbatim, and the channel adapter's own retry logic reads it exactly as it would a direct response

#### Scenario: A missing or wrong token is refused
- **WHEN** a call to `/telegram/call/{method}` carries no bearer token, or the wrong one
- **THEN** the router refuses it with 401 before making any call to the real Bot API
