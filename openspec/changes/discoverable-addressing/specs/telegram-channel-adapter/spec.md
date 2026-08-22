## ADDED Requirements

### Requirement: The adapter registers the vocabulary Telegram can express
The adapter SHALL register commands from the manager's vocabulary with Telegram,
scoped to each chat it serves, so Telegram renders its own command control in
the composer and completes what a person types. That control is the entry
point: it appears without a message being posted and does not scroll away.

Both kinds SHALL be registered — the built-in commands and the addressable
Pipelines — because a person is choosing between them in one composer and a menu
listing only half of what may be typed teaches that the other half does not
exist.

The adapter SHALL filter and adapt the vocabulary against Telegram's own rules
for command names, description length and list size. Those rules SHALL live in
the adapter. The manager SHALL publish names unchanged, and an entry Telegram
cannot express SHALL be adapted or omitted locally — never renamed in the
manager, never carried back as a constraint on the vocabulary, and never
resolved by renaming the Pipeline itself.

#### Scenario: Both kinds become Telegram commands
- **WHEN** the adapter reads the vocabulary
- **THEN** it registers the built-in commands and the Ready Pipelines for each
  served chat, and Telegram completes both as a person types

#### Scenario: Telegram's rules stay in the adapter
- **WHEN** the manager's vocabulary is inspected
- **THEN** it carries no Telegram naming rule, length limit or ordering
  constraint

#### Scenario: The Pipeline is never renamed to fit
- **WHEN** a Pipeline's name cannot be registered verbatim
- **THEN** the Pipeline CR is unchanged, the manager's published name is
  unchanged, and only the adapter's presentation differs

### Requirement: A transport-local spelling is translated back on receipt
Where a Pipeline's name cannot be a command name on the transport, the adapter
MAY register a transport-local spelling of it, and SHALL translate that spelling
back to the Pipeline's real name before anything leaves the adapter.

The mapping SHALL be confined to the adapter. No component outside it — and
nothing the manager stores, publishes or records — SHALL see the alternate
spelling.

The adapter SHALL present the same spelling everywhere it names that Pipeline to
a person on that transport, so what the menu completes and what a listing prints
are the same string.

The mapping SHALL be INJECTIVE BY CONSTRUCTION rather than by detection. Only
characters that cannot occur in a Kubernetes object name may be introduced, so
the reverse is a pure function of the string and two Pipelines can never share
one spelling. An entry the mapping cannot render injectively SHALL simply not be
registered — it stays typable, and nothing is refused, reported or conditioned
on it.

#### Scenario: A hyphenated pipeline autocompletes
- **WHEN** a Pipeline whose name Telegram rejects as a command is published
- **THEN** the adapter registers a spelling Telegram accepts, and completing it
  starts a conversation on that Pipeline

#### Scenario: The alternate spelling never escapes the adapter
- **WHEN** a conversation is started through the completed form
- **THEN** the signal, the conversation and every record of it name the
  Pipeline exactly as the manager published it

#### Scenario: The listing and the menu agree
- **WHEN** the adapter posts a listing naming Pipelines
- **THEN** each is named in the same spelling the menu completes

#### Scenario: Two Pipelines can never share a spelling
- **WHEN** any two Pipelines are published
- **THEN** their transport-local spellings differ, because the mapping only
  introduces characters a Kubernetes name cannot contain

#### Scenario: The real name still works
- **WHEN** a person types the Pipeline's real name as a command
- **THEN** it is routed as before, whether or not a spelling was registered

### Requirement: The adapter re-registers only when its own view changes
The adapter SHALL refetch the vocabulary when the revision it observes differs
from the one it last fetched, and SHALL call Telegram only when the ADAPTED
result differs from what it last registered.

Registration is rate-limited by the transport, so a vocabulary change that
produces an identical registered list SHALL produce no transport call.

#### Scenario: An inconsequential change causes no registration
- **WHEN** the vocabulary changes in a way that does not alter the adapted list
- **THEN** the adapter refetches and makes no Telegram registration call

#### Scenario: Startup registers once
- **WHEN** the adapter starts and reads the vocabulary
- **THEN** it registers once per served chat and does not repeat while the
  adapted list is unchanged

### Requirement: The adapter renders choices as inline controls
The adapter SHALL render a message's `choices` as controls attached to that
message, not as controls attached to the chat. A chat-wide control is shown to
every member of a shared surface and replaces their own composer, which is not
acceptable on an operations chat several people read.

Selecting a choice offered in answer to a message somebody wrote SHALL send that
original message to the chosen Pipeline. The adapter SHALL recover the original
from the transport's own reply linkage rather than from state held for the
purpose, so nothing is retained between the offer and the selection.

#### Scenario: Choices attach to the message
- **WHEN** a message carrying choices is posted
- **THEN** the controls appear on that message and no member's composer is
  altered

#### Scenario: One selection sends the original message
- **WHEN** a person selects a Pipeline offered in answer to their ambiguous
  message
- **THEN** that message is delivered to the chosen Pipeline without being
  retyped

#### Scenario: Nothing is held between offer and selection
- **WHEN** the adapter restarts between posting the offer and a person selecting
  from it
- **THEN** the selection still works, because the original is recovered from the
  transport

#### Scenario: An expired offer refuses rather than misfires
- **WHEN** the original message can no longer be recovered from the transport
- **THEN** the person is told to send the addressed form, and nothing is
  delivered to any Pipeline
