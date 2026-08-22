## MODIFIED Requirements

### Requirement: Outbound messages are semantic, not pre-rendered

A `send` operation SHALL carry a typed message rather than display text. The
manager SHALL emit exactly these kinds, and SHALL NOT emit transport markup in
any of them:

- `signal` — `{pipeline, source, title, labels{}, body, inputRef}`
- `answer` — `{body, status}`
- `relay` — `{origin, sender, body}`
- `notice` — `{level, body}`

A message of any kind MAY additionally carry `choices` — a list of offered
actions, each naming what it does and the addressed text it stands for. Choices
are a STRUCTURED field like `labels`, not prose: the manager states which actions
are on offer and SHALL NOT state how they are presented, whether the transport
has controls for them, or how many it can show at once.

A message of any kind MAY additionally carry `inReplyTo` — the transport's own
handle for the message this one answers. It SHALL be OPAQUE: the manager
receives it from the surface that originated the message, stores and returns it
unaltered, and SHALL NOT parse, validate, compare or construct one. It is the
same category as a thread id, which the contract already treats this way. That a
message answers another is MEANING, and it is what lets an adapter offer an
action on somebody's own words without holding state to remember them.

A `signal` message SHALL name the Pipeline that originated the conversation and
the SignalSource the input came from, so a reader can tell which route produced
a topic and which source fired. Both SHALL be optional-valued so a conversation
with no originating pipeline, or one created before provenance was recorded,
renders without them rather than failing.

Free-text fields SHALL be markdown drawn from a subset named by the contract;
structured fields SHALL stay typed so an adapter may render them as it chooses.
No component under `internal/` SHALL compose HTML or any other transport
dialect.

#### Scenario: The manager emits no transport markup

- **WHEN** any outbound message is produced — an ack, a listing, a relayed user
  message, or an agent answer
- **THEN** it carries structured fields and markdown text, with no HTML tags or
  transport-specific escaping

#### Scenario: Labels stay structured

- **WHEN** a `signal` message carries labels
- **THEN** they reach the adapter as a map, not as a formatted string

#### Scenario: A reply handle is carried without being understood

- **WHEN** a message carries `inReplyTo`
- **THEN** the manager returns the value exactly as the originating surface
  supplied it, and no component under `internal/` parses or interprets it

#### Scenario: Choices stay structured

- **WHEN** a message offers a set of actions
- **THEN** they reach the adapter as typed entries, not as a formatted list, and
  carry no instruction on how to present them

#### Scenario: A card names its route and its source

- **WHEN** an alert from source `vm-alerts` opens a conversation through pipeline
  `prod-oncall`
- **THEN** the `signal` message carries both names as fields, and the reader can
  tell which route produced the topic without inspecting any CR

#### Scenario: Missing provenance degrades, never fails

- **WHEN** a conversation has no originating pipeline, or predates provenance
  being recorded
- **THEN** the message omits those fields and the adapter renders what remains

#### Scenario: An adapter may render minimally

- **WHEN** an adapter concatenates the fields and posts them as plain text
- **THEN** that is a conforming implementation — the contract requires ownership
  of presentation, not fidelity

## ADDED Requirements

### Requirement: Choices degrade to a list
An adapter whose transport offers no selectable controls SHALL render `choices`
as text naming each action and the addressed form it stands for. Dropping them
SHALL NOT be conformant: they are the reader's only account of what is on offer.

A message carrying choices SHALL remain intelligible when they are rendered as
text, so no adapter is obliged to implement controls in order to say what the
manager meant.

#### Scenario: A plain adapter renders the same offer
- **WHEN** a message carrying choices reaches an adapter with no controls
- **THEN** each choice appears as text naming the action and its addressed form

#### Scenario: Choices are never silently dropped
- **WHEN** an adapter cannot render controls
- **THEN** it still presents the choices rather than posting the body alone
