# web-channel-type

## ADDED Requirements

### Requirement: Web channel is a built-in type on the generic Channel CRD
A web channel SHALL be declared as `spec.type: web` on the (already generic) Channel CRD, with web-specific settings under the opaque `spec.config` — no CRD schema change. The in-process web provider SHALL be the serving implementation for `type: web` and SHALL validate its own config, reporting problems on the Channel's Ready condition. Channels of other types SHALL be unaffected.

#### Scenario: Valid web channel is accepted and served in-process
- **WHEN** a Channel is applied with `spec.type: web`
- **THEN** the manager's registry resolves it to the in-process web provider and no adapter-queue ops are exposed for it

#### Scenario: Other channel types unaffected
- **WHEN** web support ships and a `type: telegram` Channel exists
- **THEN** the telegram channel's adapter-served behavior is unchanged

### Requirement: Web provider satisfies the chat Provider contract
The web provider SHALL register in the channel `Registry` under type `web`. `EnsureTopic` SHALL return a unique synthesized string thread id (e.g. `web-<nanos>`) so the landed async topic-ensure flow works unmodified. `Send` SHALL publish the message to in-memory subscribers of the conversation's event stream (acks are ephemeral and not persisted).

#### Scenario: Conversation on a web channel gets a thread id
- **WHEN** a Conversation referencing a web Channel is reconciled with `status.threadId` unset
- **THEN** the ensure-topic op completes in-process and a unique synthesized string thread id lands in `status.threadId` without any external service call

#### Scenario: Router acks reach connected browsers
- **WHEN** the router sends a busy-ack for a web-channel conversation while a browser is subscribed to that conversation's event stream
- **THEN** the ack is delivered to the subscriber and is not written to the Conversation CR

### Requirement: Web inbound uses the landed shared Router
The web chat API SHALL feed inbound messages through the landed transport-neutral `Router` (`internal/chat/router.go`) — the same code path external adapters use via `/channel/inbound` — so commands, default-profile handling, reply queueing, and busy-acks behave identically across channel types.

#### Scenario: Command creates a task conversation regardless of transport
- **WHEN** a message `/k8s-engineer check node pressure` arrives via the web chat API on a channel whose profile `k8s-engineer` exists
- **THEN** the router creates a task Conversation with that profile, a channelRef to the web Channel, and one task input — identical in shape to the Telegram path

#### Scenario: Plain message uses the channel default profile
- **WHEN** a non-command message arrives on a web Channel with `defaultProfileRef` set
- **THEN** the router creates a task Conversation using the default profile

#### Scenario: Reply to a busy conversation is queued with an ack
- **WHEN** a message arrives for an existing conversation that has an inflight work unit
- **THEN** the router appends a reply input (preserving strict serialism) and sends a "noted / queued" ack via the channel's Provider

#### Scenario: Routing parity with adapter-served channels
- **WHEN** the same command text enters via the web chat API and via `/channel/inbound`
- **THEN** the resulting Conversations are identical in shape (profile, channelRef, input)
