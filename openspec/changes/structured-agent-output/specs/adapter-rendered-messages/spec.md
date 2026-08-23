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

A FREE-TEXT BODY IS MARKDOWN IN THE NAMED SUBSET, PLUS THE BLOCK GRAMMAR. Both
are read by the ADAPTER, which is how this contract has always treated prose:
the manager names a language and each surface renders what it can.

The manager SHALL NOT parse either one. It carries no parsed representation of a
body, and a message SHALL NOT gain a field holding one — that would be a second
copy of text the message already has, and one the manager cannot keep in step
with a body it does not read.

A `signal` IS DIFFERENT AND IS NOT PROSE. Its structured fields are the message,
and an adapter renders a CARD from them. The grammar SHALL NOT be applied to a
signal's body, which is a machine document or a person's typed words. This is
the one place an adapter needs a second renderer.

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
- **THEN** the adapter parses that body exactly as it parses an `answer`,
  because the grammar follows the TEXT and not the message kind

#### Scenario: The agent's text arrives unaltered

- **WHEN** an agent's output contains block tags
- **THEN** the body the adapter receives is byte-for-byte what the agent printed,
  and no field beside it holds another version of the same text

#### Scenario: A signal is a card, not prose

- **WHEN** a `signal` message is delivered
- **THEN** the adapter renders its structured fields as a card, and no part of
  its body is read as the block grammar

### Requirement: Presentation limits belong to the adapter

Escaping, length limits, chunking, truncation, and any decision to render
content as an attachment SHALL be the adapter's responsibility. The manager
SHALL NOT truncate message bodies, SHALL NOT escape for any transport, and SHALL
NOT declare a maximum message size.

The manager SHALL NOT move content between blocks either. Which part is the
summary is a judgement the AGENT already made by choosing what goes inside
`<details>`, and a length budget is a guess at it — one that cuts a markdown
table from its header. See `structured-agent-output`.

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

#### Scenario: A long answer stays long

- **WHEN** an agent writes far more above the fold than a reader wants
- **THEN** the message carries it unchanged, and the adapter is still the one
  that decides how to split or attach it

### Requirement: Adapters declare the contract version they speak

`GET /channel/ops` SHALL require the polling adapter to declare the outbound
contract version it supports, and SHALL reject an absent or unsupported
declaration with 400 naming what is expected, rather than delivering operations
it cannot interpret.

THE VERSION DOES NOT MOVE FOR THE BLOCK GRAMMAR. No field is added and no field
changes meaning: a body that was markdown is now markdown plus a grammar, read
by the same component that already read the markdown.

An adapter that does not implement the grammar renders the tags literally.
That is prevented by `AgentProfile.spec.sharedOutputFormat`, which is OFF by
default — no agent emits a tag until an install turns it on, and an install
turns it on when its adapters understand it. The compatibility boundary is the
PROFILE, not the wire.

#### Scenario: Outdated adapter fails loudly

- **WHEN** an adapter built against the string-valued contract polls for
  operations
- **THEN** the manager responds 400 naming the required contract version, and no
  empty messages are posted

#### Scenario: Current adapter polls normally

- **WHEN** an adapter declares the supported version
- **THEN** operations are delivered as before

#### Scenario: An adapter that never learns the grammar keeps working

- **WHEN** an adapter has no parser for block tags
- **THEN** it serves profiles that have not opted in and sees the prose it always
  saw, with no version to negotiate and nothing to refuse
