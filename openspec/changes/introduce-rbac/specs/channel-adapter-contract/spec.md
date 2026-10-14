## MODIFIED Requirements

### Requirement: Inbound messages enter through the shared router
The manager SHALL expose `POST /channel/inbound` accepting `{channel, threadId, text, sender?}` as the CONTINUATION path only: `threadId` is REQUIRED, and the message SHALL be routed through the transport-neutral router as a reply input on the matching conversation (busy-ack preserved). Resulting acks SHALL flow back to the adapter as `send` operations, and the message SHALL be delivered to every OTHER bound channel that did not already display it as a `relay` message carrying its attribution as fields rather than composed into the text. Whether the originating surface displayed it is the serving adapter's declaration (`ChannelAdapter.spec.echoesOwnMessages`), not a property of the message — the decision is made PER DESTINATION. In-process (registry) providers SHALL use the same operation pipeline so routing behavior is identical for built-in and external types.

The ORIGINATION branch is removed: command parsing and default-profile conversation creation no longer occur here, and a message in an unrecognized thread is no longer adopted as a new conversation. A request without `threadId` SHALL be rejected with a message naming the signal path as the origination route. Channel implementations MAY omit inbound entirely when a separate component handles their ingest.

A request that omits `sender` SHALL be treated as role `user` with no identity pair.

It is therefore refused on a conversation carrying an `originReader`. It is accepted on one carrying none.

A reply whose sender
resolves to role `user` SHALL be refused when the target conversation's
`spec.originReader` names a DIFFERENT `{channel, key}` pair than the
sender's own.

The refusal applies to the reply, not the thread. The request
is rejected rather than accepted and delivered.

A reply to a target conversation carrying NO `originReader` at
all SHALL be accepted regardless of the sender's role. A reply
from a sender whose role is `responder` or `administrator` SHALL NEVER be
refused by this rule.

This is the role-and-ownership decision `access-control` defines, applied
uniformly here for every channel this contract serves, for a reply on
`/channel/inbound`. Addressing an existing conversation by name is outside
this contract.

#### Scenario: Threaded reply is queued serially
- **WHEN** an adapter posts an inbound message whose thread id matches a conversation with an inflight unit
- **THEN** a reply input is appended (not dispatched concurrently) and a busy-ack `send` op is emitted

#### Scenario: Missing thread id is refused, not originated
- **WHEN** an adapter posts an inbound message with no thread id
- **THEN** the request is rejected with a message naming the signal path, and no Conversation is created

#### Scenario: Unknown thread is not adopted
- **WHEN** an adapter posts an inbound message whose thread id matches no conversation
- **THEN** no conversation is created or adopted

#### Scenario: Reply still reaches every channel that has not shown it
- **WHEN** a reply arrives on one channel of a multi-channel conversation
- **THEN** it is delivered as a `relay` message to every other bound channel, attributed by each adapter's own rendering, and to the originating one too when its adapter declares that it does not echo a person's own message

#### Scenario: A user's reply to someone else's conversation is refused

- **WHEN** a `user`-role sender posts an inbound reply whose thread
  resolves to a conversation owned by a different identity pair
- **THEN** the request is refused with a message naming ownership as the
  reason, and no reply input is appended

#### Scenario: A user's reply to an unowned conversation is accepted

- **WHEN** a `user`-role sender posts an inbound reply to a conversation
  carrying no `originReader`
- **THEN** the reply is routed normally, exactly as for any other role

#### Scenario: A responder's reply is never refused by ownership

- **WHEN** a `responder`-role sender posts an inbound reply to a
  conversation owned by a different identity pair
- **THEN** the reply is routed normally
