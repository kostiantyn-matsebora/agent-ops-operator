## MODIFIED Requirements

### Requirement: Inbound messages enter through the shared router
The manager SHALL expose `POST /channel/inbound` accepting `{channel, threadId, text, sender?}` as the CONTINUATION path only: `threadId` is REQUIRED, and the message SHALL be routed through the transport-neutral router as a reply input on the matching conversation (busy-ack preserved). Resulting acks SHALL flow back to the adapter as `send` operations, and the message SHALL be relayed to the conversation's sibling channels as attributed text. In-process (registry) providers SHALL use the same operation pipeline so routing behavior is identical for built-in and external types.

The ORIGINATION branch is removed: command parsing and default-profile conversation creation no longer occur here, and a message in an unrecognized thread is no longer adopted as a new conversation. A request without `threadId` SHALL be rejected with a message naming the signal path as the origination route. Channel implementations MAY omit inbound entirely when a separate component handles their ingest.

#### Scenario: Threaded reply is queued serially
- **WHEN** an adapter posts an inbound message whose thread id matches a conversation with an inflight unit
- **THEN** a reply input is appended (not dispatched concurrently) and a busy-ack `send` op is emitted

#### Scenario: Missing thread id is refused, not originated
- **WHEN** an adapter posts an inbound message with no thread id
- **THEN** the request is rejected with a message naming the signal path, and no Conversation is created

#### Scenario: Unknown thread is not adopted
- **WHEN** an adapter posts an inbound message whose thread id matches no conversation
- **THEN** no conversation is created or adopted

#### Scenario: Reply still mirrors to sibling channels
- **WHEN** a reply arrives on one channel of a multi-channel conversation
- **THEN** it is relayed to the sibling channels as attributed text, unchanged from before
