## ADDED Requirements

### Requirement: Addressing an existing conversation is gated by ownership

The router's `HandleCommand` SHALL refuse a sender whose resolved role is
`user` when addressing an EXISTING conversation by name, if that
conversation's `spec.originReader` names a different `{channel, key}` pair
than the sender's own.

The sender's identity pair SHALL be `{channel: <the receiving Channel's
name>, key: <the `sender` argument HandleCommand already receives>}`. The
role lookup uses the raw `sender` string. The ownership comparison hashes it
with the function `originReader.key` is stamped with, as the access-control
spec states.

Addressing a Pipeline fresh SHALL remain ungated.

#### Scenario: A user addresses someone else's conversation

- **WHEN** a `user`-role sender addresses a conversation owned by a
  different identity pair
- **THEN** the router refuses it with a message naming ownership as the
  reason, and the conversation is not touched

#### Scenario: A user starts a new conversation

- **WHEN** a `user`-role sender addresses a Pipeline fresh
- **THEN** the router admits it, exactly as before

#### Scenario: Responder and administrator are not gated

- **WHEN** a `responder`-role or `administrator`-role sender addresses any
  existing conversation
- **THEN** the router admits it
