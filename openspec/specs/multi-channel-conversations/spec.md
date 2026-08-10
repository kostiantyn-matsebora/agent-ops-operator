# multi-channel-conversations

## Purpose

Conversations bound to any number of channels with per-channel thread bindings: every bound channel fully mirrors the conversation (fanned-out replies and acks, attributed cross-channel relay of user messages), and multi-channel conversations force manager-owned result delivery so no surface mistakes silence for success.
## Requirements
### Requirement: Conversations bind to any number of channels with per-channel threads
`Conversation.spec.channelRefs` SHALL be a list of channel references and `status.threads` a list of `{channel, threadId}` bindings (**BREAKING**: replaces the single `channelRef`/`threadId`; migration documented). Topic creation SHALL be ensured per bound channel (stable op id per conversation×channel; completion writes that channel's binding); inbound thread resolution SHALL match `(channel, threadId)` pairs; a conversation SHALL dispatch its first unit once at least one binding exists (a broken channel never deadlocks the conversation, its topic catches up later). Single-entry `channelRefs` SHALL behave exactly as the old single-channel model.

#### Scenario: Topics created on every bound channel
- **WHEN** a conversation bound to `home-ops` (telegram) and `web` is reconciled with no thread bindings
- **THEN** ensure-topic ops are enqueued for both channels and each completion lands its own `{channel, threadId}` binding

#### Scenario: One broken channel does not block dispatch
- **WHEN** one bound channel's adapter is down but another channel's topic lands
- **THEN** the conversation dispatches its pending input; the broken channel's topic is still ensured (regenerated) for later

#### Scenario: Inbound resolves per channel-thread pair
- **WHEN** two channels coincidentally use the same thread id string for different conversations
- **THEN** an inbound message resolves by its own channel's binding only — no cross-channel collision

### Requirement: Bound channels fully mirror the conversation
Every bound channel SHALL receive the whole conversation: router acks fan out to all bound channels; a user message arriving on one channel SHALL be relayed to the sibling channels as a `relay` message whose attribution stays STRUCTURED (`origin`, `sender`), so each surface decides how to mark somebody else's words; agent replies SHALL be fanned out by the manager (see below). Channel implementations MUST NOT re-ingest their own outbound posts as inbound (no relay loops).

#### Scenario: Telegram and web chat repeat each other
- **WHEN** a user writes in the telegram topic of a conversation also bound to the web channel
- **THEN** the web channel receives the attributed user message and, later, the same agent reply and acks as telegram

#### Scenario: Relay never loops
- **WHEN** a relayed attributed message is posted to a sibling channel
- **THEN** it is not fed back through `/channel/inbound` (or any provider's inbound path) as a new user message

### Requirement: Manager fans out agent replies on multi-channel conversations
For conversations bound to one or more channels, `POST /work/done` SHALL fan the run's result out as one `send` op per bound channel targeting that channel's thread, carrying an `answer` message with the result as its markdown body. Failed or empty results SHALL fan out a short failure `notice` — mirrored surfaces never mistake silence for success. Each bound channel's adapter renders the same semantic message independently, so one answer MAY be presented differently per surface (chunked, collapsed, attached) without the manager knowing any transport's limits. Composition uses the existing op pipeline only.

#### Scenario: One answer, every surface
- **WHEN** a run completes with a result on a conversation bound to telegram and web
- **THEN** both channels receive the result as a message in the conversation's own thread

#### Scenario: Failure is visible on every surface
- **WHEN** a run finishes with status failed and no result
- **THEN** every bound channel receives a short failure notice in the thread

#### Scenario: The same answer may look different per surface
- **WHEN** a long answer fans out to two channels whose transports have different limits
- **THEN** each adapter renders it for its own surface — one may split it, another may show it whole — from the identical semantic message

#### Scenario: Relayed user messages are semantic too
- **WHEN** a user message on one bound channel is mirrored to its siblings
- **THEN** each sibling receives a `relay` message carrying origin, sender, and body as fields, and composes the attribution itself

### Requirement: Closing a conversation ends it on every bound channel
Closing a conversation SHALL apply to the whole conversation, not to the channel
the command arrived on: the farewell message SHALL be fanned out to every bound
thread and one `close-topic` operation SHALL be enqueued per bound thread, each
addressed to that channel's serving adapter. A bound channel that never obtained
a thread SHALL be skipped without blocking the others.

#### Scenario: All bound threads are archived
- **WHEN** `/close` is sent in one thread of a conversation bound to three channels
- **THEN** all three threads receive the farewell message and a `close-topic` operation

#### Scenario: Unbound channel is skipped
- **WHEN** a conversation is closed while one of its bound channels has no thread id yet
- **THEN** no `close-topic` operation is enqueued for that channel and the other channels are still archived

#### Scenario: A stalled channel does not hold the others
- **WHEN** one channel's adapter never completes its `close-topic` operation
- **THEN** the other channels are archived and the conversation is deleted after the grace period

