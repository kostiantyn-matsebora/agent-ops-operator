## MODIFIED Requirements

### Requirement: Bound channels fully mirror the conversation
Every bound channel SHALL receive the whole conversation: router acks fan out to all bound channels; a user message SHALL be delivered to every bound channel EXCEPT the surface it entered on, as a `relay` message whose attribution stays STRUCTURED (`origin`, `sender`), so each surface decides how to mark somebody else's words; agent replies SHALL be fanned out by the manager (see below). Channel implementations MUST NOT re-ingest their own outbound posts as inbound (no relay loops).

Delivery of a user message SHALL NOT be scoped to SIBLING channels. The origin SURFACE is the only destination excluded, so a conversation bound to one channel still delivers a person's message to that channel when the message entered elsewhere — and a conversation whose surface does not display what a person typed receives it back and renders it.

The no-relay-loops rule is load-bearing here: a message may be delivered toward the same transport it entered through when that transport serves several surfaces, so an implementation that re-ingested its own outbound posts would loop.

#### Scenario: Telegram and web chat repeat each other
- **WHEN** a user writes in the telegram topic of a conversation also bound to the web channel
- **THEN** the web channel receives the attributed user message and, later, the same agent reply and acks as telegram

#### Scenario: Two surfaces on one transport
- **WHEN** a conversation binds two surfaces served by one adapter and a person writes on the first
- **THEN** the second receives the message and the first does not, decided by surface rather than by transport

#### Scenario: A viewer receives its own users' messages
- **WHEN** a person types on a surface that has no transport echo of its own
- **THEN** the manager delivers the message to that surface and the viewer renders it, confirming whatever it showed optimistically

#### Scenario: Relay never loops
- **WHEN** a relayed attributed message is posted to a sibling channel
- **THEN** it is not fed back through `/channel/inbound` (or any provider's inbound path) as a new user message
