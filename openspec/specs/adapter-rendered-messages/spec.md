# adapter-rendered-messages

## Purpose

The manager composes MEANING; adapters compose PRESENTATION. Outbound operations
carry typed messages and topic descriptors with markdown-valued free text, and
every presentation decision — escaping, length, chunking, markup, thread naming —
belongs to the adapter that knows the transport.

## Requirements

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

### Requirement: Topic creation carries a descriptor the adapter names from

An `ensure-topic` operation SHALL carry
`{conversation, pipeline, source, title, labels{}, kind}` rather than a rendered
title string. The adapter SHALL derive the thread name from that descriptor
using its own template and enforce its own naming limits, so a deployment may
name topics after their route or source without the manager choosing a format.

#### Scenario: Adapter names the topic

- **WHEN** an `ensure-topic` op is delivered for an alert-originated conversation
- **THEN** the adapter composes the thread name itself and returns the resulting
  thread id

#### Scenario: Transport naming limits are the adapter's problem

- **WHEN** a descriptor would render a name longer than the transport allows
- **THEN** the adapter truncates or reshapes it, and the manager is unaware of
  the limit

### Requirement: Presentation limits belong to the adapter

Escaping, length limits, chunking, truncation, and any decision to render
content as an attachment SHALL be the adapter's responsibility. The manager
SHALL NOT truncate message bodies, SHALL NOT escape for any transport, and SHALL
NOT declare a maximum message size.

#### Scenario: Oversized body is the adapter's call

- **WHEN** a `signal` body exceeds what the transport accepts in one message
- **THEN** the adapter splits it, truncates it, or sends it as an attachment —
  and the operation succeeds

#### Scenario: Markup-bearing payloads are safe

- **WHEN** a signal payload contains `<`, `>`, or `&`
- **THEN** it reaches the adapter unescaped and the adapter escapes it for its
  own transport, with no content lost

#### Scenario: Full payloads reach the adapter

- **WHEN** a signal's payload is stored out of line as a `ConversationInput`
- **THEN** the message body carries the full payload inline and `inputRef`
  names the `ConversationInput`, so no adapter needs Kubernetes access

### Requirement: In-process providers render like external adapters

Channel implementations registered in-process SHALL consume the same typed
messages and own their presentation identically. Presentation logic SHALL NOT
re-enter the manager through a built-in provider.

#### Scenario: Built-in provider owns its rendering

- **WHEN** an in-process provider receives a `notice`
- **THEN** it renders the message itself, and no shared manager-side renderer
  exists for it to call

### Requirement: Adapters declare the contract version they speak

`GET /channel/ops` SHALL require the polling adapter to declare the outbound
contract version it supports, and SHALL reject an absent or unsupported
declaration with 400 naming what is expected, rather than delivering operations
it cannot interpret.

#### Scenario: Outdated adapter fails loudly

- **WHEN** an adapter built against the string-valued contract polls for
  operations
- **THEN** the manager responds 400 naming the required contract version, and no
  empty messages are posted

#### Scenario: Current adapter polls normally

- **WHEN** an adapter declares the supported version
- **THEN** operations are delivered as before

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
