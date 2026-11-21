## Purpose

The generic HTTP channel adapter in `channels/http/`. A dependency-free module that delivers each agent message to an owner-set URL and accepts replies over HTTP. It composes no presentation and starts no conversation.

## ADDED Requirements

### Requirement: The adapter serves Channels through the channel adapter contract
A channel adapter in `channels/http/` (own dependency-free Go module and image `agentops-channel-http`, listed by `.github/components.sh images` as `channel-http`) SHALL serve `Channel`s whose `spec.adapter` names its `ChannelAdapter`, through `docs/contracts/channel-adapter.md`. It SHALL serve its reply endpoint on the port declared by `ChannelAdapter.spec.port` (`LISTEN_ADDR`). It SHALL read operations from `GET /channel/ops`, acknowledge each, then report `done` or `failed`.

#### Scenario: An operation is delivered and reported
- **WHEN** the manager queues a message for a served Channel
- **THEN** the adapter acknowledges it, delivers it and reports `done`

### Requirement: Delivery is a JSON POST to the Channel's target URL
The adapter SHALL read the target URL from the Channel's `config` and SHALL POST the typed message as JSON, not rendered text. It SHALL send `Authorization: Bearer <token>` from the Channel's own `credentialsSecretRef` key `TOKEN` when one is projected. A 2xx response SHALL report `done`. Any other status, a timeout or a transport error SHALL report `failed`.

The adapter SHALL dial only the owner-set URL and SHALL NOT dial a URL taken from a message. It SHALL NOT re-send after the acknowledgement.

#### Scenario: A 2xx is done
- **WHEN** the target answers `200`
- **THEN** the adapter reports `done`

#### Scenario: A refusal is a final failure
- **WHEN** the target answers `500` or does not answer within the timeout
- **THEN** the adapter reports `failed` and does not re-send

### Requirement: Replies arrive on a per-channel authenticated endpoint
The adapter SHALL accept `POST /reply/{channel}` with a `threadId` and text and SHALL forward it to `/channel/inbound`. The request SHALL carry `Authorization: Bearer <token>` matching that Channel's own `TOKEN`, compared in constant time. A mismatch or absence SHALL return `401`, a token in the URL SHALL be ignored, a missing `threadId` or text SHALL return `400` and an unknown channel SHALL return `404`. Nothing is forwarded on any of these.

The endpoint is reply-only. A reply SHALL NOT start a conversation, and the adapter SHALL NOT re-ingest its own outbound post.

#### Scenario: A reply reaches its thread
- **WHEN** an authenticated reply names a known `threadId`
- **THEN** the adapter forwards it to `/channel/inbound` and answers `202`

#### Scenario: Another channel's token is refused
- **WHEN** a reply carries a token belonging to a different Channel
- **THEN** the adapter returns `401` and forwards nothing

#### Scenario: A reply without a thread is refused
- **WHEN** a reply has no `threadId`
- **THEN** the adapter returns `400`

### Requirement: Configuration faults surface on the Channel and stay isolated
The adapter SHALL validate each Channel's `config` when it loads it. A missing target URL or one that is not `http` or `https` SHALL be reported with `ready: false`, a `reason` and a `message` through `POST /channel/channels/{name}/status`, and its deliveries SHALL report `failed`. A fault on one Channel SHALL NOT affect another.

#### Scenario: A bad target is reported
- **WHEN** a Channel's target URL is missing
- **THEN** the fault is reported through the status endpoint and another Channel keeps delivering

### Requirement: The adapter's page states the target and loop consequences
`docs/integrations/http.md` SHALL state that the target URL is the only host the adapter dials, that egress policy should confine it, and that a receiver replying to every delivery can loop.

#### Scenario: The page carries the required statements
- **WHEN** `docs/integrations/http.md` is read
- **THEN** it states the target rule, the egress recommendation and the loop warning
