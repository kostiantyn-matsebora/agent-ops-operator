## MODIFIED Requirements

### Requirement: The console marks conversations with unseen activity
The conversation list SHALL carry, per row, the COUNT of messages on the
console's own thread that the viewer has not seen. A row SHALL be unread
when that count is above zero.

Unreadness SHALL be derived from the console's OWN thread binding and no
other channel's.

A message SHALL count when it is after the reader's watermark AND its kind
is one a person has to read: the signal that opened or advanced the
conversation, an agent answer, or a relayed message from a person on
another channel.

A message SHALL NOT count when it is a manager acknowledgement or notice, a
message the reader typed here, a listing or a refusal answering the reader,
or an inline run event. A phase change SHALL NOT count.

A conversation the console merely observes — one with no console thread —
SHALL NOT be shown as unread. The console holds no watermark on it and has
no standing to call it new.

#### Scenario: A new answer marks its conversation unread
- **WHEN** an agent posts an answer to a conversation whose console thread was previously read
- **THEN** that conversation's count is one and it is marked unread

#### Scenario: The acknowledgement does not count
- **WHEN** the reader sends a message and the manager acknowledges it and dispatches a run
- **THEN** the conversation's count stays zero for that reader

#### Scenario: Two answers count twice
- **WHEN** a signal advances a read conversation and the agent then answers
- **THEN** the count is two

#### Scenario: Reading it elsewhere does not clear the console
- **WHEN** a conversation bound to both a chat channel and the console is read in the chat channel
- **THEN** it is still unread in the console

#### Scenario: An observed conversation is never unread
- **WHEN** a conversation has no console thread
- **THEN** it carries no count regardless of its activity

### Requirement: Unread is a server-side filter, and its count is computed before filtering
The list SHALL offer an unread-only filter evaluated server-side by the
counting rule alongside the existing filters, so pagination and the total
match count stay correct.

The response SHALL carry the SUM of unread counts over ALL conversations
BEFORE any filter is applied, so the badge never moves because a filter hid
something.

A count-only form SHALL be available for surfaces that need the number
without the rows. The count per scope SHALL be available for the inbox.

#### Scenario: Unread-only narrows server-side
- **WHEN** the operator turns on the unread filter
- **THEN** only conversations with a count above zero are returned, with a total match count and correct pagination

#### Scenario: The badge is a sum
- **WHEN** two conversations hold one and three unread messages
- **THEN** the navigation badge reads four

#### Scenario: The count does not move when the view narrows
- **WHEN** the operator applies a phase filter that hides unread conversations
- **THEN** the unread sum is unchanged

#### Scenario: The count is available without the rows
- **WHEN** a surface requests the count-only form
- **THEN** it receives the unread sum, the per-scope counts and the total, and no conversation rows

### Requirement: Opening a conversation marks its thread read
Opening a conversation SHALL report its console thread read up to the newest
counted message, and SHALL keep reporting as further counted messages arrive
while it stays open.

The console SHALL report only a time it read from the conversation's own
state — the newest counted message's time, or the conversation's activity
time where that is later — never a locally generated "now", and SHALL
report nothing when the watermark would not advance.

#### Scenario: Opening clears the mark
- **WHEN** the operator opens an unread conversation
- **THEN** its console thread is reported read and the count is zero

#### Scenario: An open conversation stays read
- **WHEN** an agent answers while the operator has the conversation open
- **THEN** the watermark advances and the count stays zero

#### Scenario: Re-opening a read conversation writes nothing
- **WHEN** the operator re-opens a conversation whose watermark already covers its newest counted message
- **THEN** no read is reported

## ADDED Requirements

### Requirement: The console marks a conversation unread
The console SHALL offer mark unread per row and over a selection, bounded
and attributed exactly as mark read is.

It SHALL rewind the acting reader's OWN watermark to just before the newest
counted message, so that message counts again for that reader and for
nobody else.

Mark unread SHALL be absent where the console resolves no reader, since the
only watermark to move would be the channel-wide one.

#### Scenario: Marking unread brings one message back
- **WHEN** the operator marks a read conversation unread
- **THEN** its count is one for them and unchanged for every other reader

#### Scenario: No reader, no rewind
- **WHEN** the console has no reader salt
- **THEN** mark unread is not offered
