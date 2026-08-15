## ADDED Requirements

### Requirement: The console marks conversations with unseen activity
The conversation list SHALL distinguish conversations whose console thread has
activity newer than the viewer's watermark from those it does not, and each row
SHALL carry its read state. Unreadness SHALL be derived from the console's OWN
thread binding and no other channel's.

A conversation the console merely observes — one with no console thread — SHALL
NOT be shown as unread. The console holds no watermark on it and has no standing
to call it new; this is the same reach boundary the close action draws.

#### Scenario: A new answer marks its conversation unread
- **WHEN** an agent posts an answer to a conversation whose console thread was previously read
- **THEN** that conversation is marked unread in the list

#### Scenario: Reading it elsewhere does not clear the console
- **WHEN** a conversation bound to both Telegram and the console is read in Telegram
- **THEN** it is still unread in the console

#### Scenario: An observed conversation is never unread
- **WHEN** a conversation has no console thread
- **THEN** it is not shown as unread regardless of its activity

### Requirement: Unreadness is answered for the viewer, not for the console
Unreadness SHALL be derived from the watermark of the IDENTITY making the
request, where the console can resolve one, and from the console channel's own
watermark otherwise.

The console SHALL derive the reader key by hashing the resolved identity with a
salt supplied to it as a credential, and SHALL send only that key upstream. It
SHALL NOT send an address, and no identity SHALL be recoverable from what is
stored.

Where authentication proves only possession of a shared token, all holders
SHALL resolve to one identity and therefore share one watermark.

#### Scenario: One operator reading does not clear it for another
- **WHEN** two operators are authenticated as different identities and one opens an unread conversation
- **THEN** it is read for that one and still unread for the other

#### Scenario: A shared token is one reader
- **WHEN** two people are authenticated by the same static token
- **THEN** they share one watermark, and either one reading clears it for both

#### Scenario: No address is stored
- **WHEN** a read is reported for an authenticated operator
- **THEN** the conversation records an opaque key from which the operator's identity cannot be recovered

### Requirement: A reader's own actions mark a conversation read for that reader
Starting a conversation from the console, and sending a message in one, SHALL
advance the acting reader's own watermark, and SHALL NOT advance any other
reader's.

A conversation SHALL therefore never be presented as unread to the person who
just created it, and SHALL remain unread to colleagues who have not seen it.

#### Scenario: Starting a conversation does not make it unread for you
- **WHEN** an operator starts a conversation from the console and returns to the list before any answer arrives
- **THEN** it is not marked unread for them

#### Scenario: …but it is new to everybody else
- **WHEN** another operator views the list after that conversation is created
- **THEN** it is unread for them

#### Scenario: Replying keeps it read for the replier
- **WHEN** an operator sends a message in a conversation
- **THEN** their own watermark advances past their message rather than the conversation becoming unread to them

### Requirement: Unread is a server-side filter, and its count is computed before filtering
The list SHALL offer an unread-only filter evaluated server-side alongside the
existing filters, so pagination and the total match count stay correct.

The response SHALL carry an unread count computed over ALL conversations BEFORE
any filter is applied, so the count never moves because a filter hid something.
A count-only form SHALL be available for surfaces that need the number without
the rows.

#### Scenario: Unread-only narrows server-side
- **WHEN** the operator turns on the unread filter
- **THEN** only unread conversations are returned, with a total match count and correct pagination

#### Scenario: The count does not move when the view narrows
- **WHEN** the operator applies a phase filter that hides unread conversations
- **THEN** the unread count is unchanged

#### Scenario: The count is available without the rows
- **WHEN** a surface requests the count-only form
- **THEN** it receives the unread and total counts and no conversation rows

### Requirement: Opening a conversation marks its thread read
Opening a conversation's detail view SHALL report its console thread read up to
that conversation's current activity, and SHALL keep reporting as further
activity arrives while the view remains open.

The console SHALL report only a watermark it read from the conversation's own
state, never a locally generated "now", and SHALL report nothing when the
watermark would not advance.

#### Scenario: Opening clears the mark
- **WHEN** the operator opens an unread conversation
- **THEN** its console thread is reported read and the row is no longer unread

#### Scenario: An open conversation stays read
- **WHEN** an agent answers while the operator has the detail view open
- **THEN** the watermark advances and the conversation does not become unread

#### Scenario: Re-opening a read conversation writes nothing
- **WHEN** the operator re-opens a conversation whose watermark already covers its latest activity
- **THEN** no read is reported

### Requirement: The console marks a selected batch read
The console SHALL offer an action that marks the selected rows read in one
gesture. It SHALL take an explicit list of conversation names from the operator's
selection, SHALL NOT accept a filter or an "everything matching" scope, and SHALL
be bounded at 50 names — the list page size — enforced by the server.

Selection SHALL be available whether or not this console may write to
conversations, since marking read is not an instruction to an agent.

Each conversation SHALL be reported with its own outcome. A conversation the
console holds no thread on SHALL be reported skipped with the reason naming the
fix, exactly as the close action reports it.

#### Scenario: A selection is marked read
- **WHEN** the operator selects four unread conversations and marks them read
- **THEN** all four console threads are reported read and the result reports four marked

#### Scenario: Observed conversations are skipped, not marked
- **WHEN** a batch includes a conversation with no console thread
- **THEN** it is reported skipped with the reason that the console holds no thread on it, and the rest are still marked

#### Scenario: An oversized batch is refused
- **WHEN** a mark-read request carries more than 50 names
- **THEN** the request is rejected and nothing is marked

#### Scenario: Nothing selected disables the action
- **WHEN** no conversation is selected
- **THEN** the mark-read action is not available

#### Scenario: There is no filter-scoped sweep
- **WHEN** the operator has a filter applied and marks read
- **THEN** only the selected rows on the current page are marked

### Requirement: Marking read is authenticated and attributed
A read report SHALL require authentication and SHALL be logged against the
resolved identity, like every other action the console takes.

It SHALL NOT be gated by the install-wide write gate: that gate makes the console
a strict viewer by removing its ability to instruct an agent or start work, and a
read watermark does neither. A console that could show a backlog but never clear
it would be broken in the way this capability exists to fix.

#### Scenario: An unauthenticated report is refused
- **WHEN** an unauthenticated mark-read request arrives
- **THEN** it is refused with 401 and nothing is marked

#### Scenario: A read-only console can still mark read
- **WHEN** the console is configured read-only and a mark-read request arrives
- **THEN** it is served, and the composer and origination action remain absent

#### Scenario: The write is attributed
- **WHEN** a mark-read request is served
- **THEN** it is logged against the identity that ordered it
