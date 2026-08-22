## MODIFIED Requirements

### Requirement: Outbound messages are semantic, not pre-rendered

A `send` operation SHALL carry a typed message rather than display text. The
manager SHALL emit exactly these kinds, and SHALL NOT emit transport markup in
any of them:

- `signal` — `{pipeline, source, title, labels{}, body, blocks[], inputRef}`
- `answer` — `{body, blocks[], status}`
- `relay` — `{origin, sender, body}`
- `notice` — `{level, body, blocks[]}`

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

`blocks[]` is the parsed structure of the body: an ordered list of labelled
sections plus a folded region. The manager SHALL populate it and SHALL keep
`body` populated alongside as the flattened equivalent, so a message is never
carried by `blocks[]` alone.

`blocks[]` SHALL follow AGENT-REPORTED text rather than the message kind carrying
it. A run that failed and explained itself is reported as a `notice`, and that
explanation SHALL carry blocks exactly as an `answer` does. A message whose body
the manager composed itself — a listing, a refusal, a usage error — SHALL carry
none.

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

#### Scenario: A failed run's explanation is still structured

- **WHEN** a run fails, reports its own explanation, and that explanation goes
  out as a `notice`
- **THEN** the notice carries blocks, because the body is the agent's text —
  while a listing the manager composed carries none

#### Scenario: Structure and prose travel together

- **WHEN** an agent answer is parsed into blocks
- **THEN** the message carries both `blocks[]` and a `body` flattened from them,
  and an adapter reading only `body` renders a complete message

### Requirement: Presentation limits belong to the adapter

Escaping, length limits, chunking, truncation, and any decision to render
content as an attachment SHALL be the adapter's responsibility. The manager
SHALL NOT truncate message bodies, SHALL NOT escape for any transport, and SHALL
NOT declare a maximum message size.

Deciding what sits above the fold is NOT a presentation limit. The manager MAY
move content from above the fold into it, because which part is the summary is a
question about MEANING and is answerable without knowing any transport. It
remains forbidden from removing content: every word an agent reported SHALL
remain in the message.

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

#### Scenario: Folding is not truncating

- **WHEN** the manager demotes over-long content into the fold
- **THEN** the message still carries every word, and the adapter is still the one
  that decides how to split or attach it

### Requirement: Adapters declare the contract version they speak

`GET /channel/ops` SHALL require the polling adapter to declare the outbound
contract version it supports, and SHALL reject an absent or unsupported
declaration with 400 naming what is expected, rather than delivering operations
it cannot interpret.

The current version is 3, which adds `blocks[]`. An adapter declaring version 2
SHALL continue to be served: it receives the message with `body` populated and
`blocks[]` omitted. Version 2 renders a complete message, so serving it is
correct rather than degraded — unlike the string-valued version 1, which is
still refused because it has no field to render at all.

#### Scenario: Outdated adapter fails loudly

- **WHEN** an adapter built against the string-valued contract polls for
  operations
- **THEN** the manager responds 400 naming the required contract version, and no
  empty messages are posted

#### Scenario: Current adapter polls normally

- **WHEN** an adapter declares the supported version
- **THEN** operations are delivered as before

#### Scenario: A version 2 adapter keeps working

- **WHEN** an adapter that has not been upgraded declares version 2
- **THEN** it receives messages with a flattened `body` and no `blocks[]`, and
  posts exactly what it posts today
