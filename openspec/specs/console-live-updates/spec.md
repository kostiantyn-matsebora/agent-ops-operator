# console-live-updates Specification

## Purpose
The console holds one stream for its whole lifetime and still reloaded its data
over HTTP whenever anything changed, blanking the page each time. This
capability covers how a LOADED console stays current: what an event carries, how
it is applied, when a refetch is still the right answer, and the rule that a
view which has painted never goes back to a spinner.

## Requirements

### Requirement: A change event carries what changed
An event announcing a change SHALL carry the changed object, not only its
identity. A consumer told that something changed SHALL NOT have to ask what it
now is.

This SHALL hold for every kind the console renders. An event that can only be
answered by a request is a request with extra steps.

The object SHALL be carried in the SHAPES that view serves it in, projected by
the SAME code the snapshot endpoints use. A consumer SHALL NOT have to
reconstruct a projection the server already performs, because a second
implementation of it eventually disagrees with the first.

#### Scenario: A delta carries its object
- **WHEN** a watched object is created or updated
- **THEN** the event carries the object as the console would have fetched it

#### Scenario: An applied entry equals a fetched one
- **WHEN** the same object state is applied from an event and fetched from a
  snapshot
- **THEN** the two are the same shape, field for field

#### Scenario: A message carries its content
- **WHEN** a message is appended to a conversation
- **THEN** the event carries the message itself, and no request is made to learn
  what it said

### Requirement: Events are applied to what is already loaded
A stream event SHALL be APPLIED to the data the console holds. It SHALL NOT be
used as a signal to discard that data and load it again.

Applying SHALL be defined once per kind, in one place. A view SHALL NOT
implement its own handling of the same event, because two applications of one
event drift and the drift shows as two views disagreeing about the same object.

#### Scenario: A message appears without a request
- **WHEN** a message arrives for a conversation the reader has open
- **THEN** it appears in that conversation, and no request is made

#### Scenario: One event updates every view holding that object
- **WHEN** an object appears in a list and in a detail view at once
- **THEN** a single event updates both, from one applier

### Requirement: A view that has painted never returns to loading
Once a view has rendered its data, arriving events SHALL NOT put it back into a
loading state. A reader SHALL never see content replaced by a spinner because
something changed.

Identity of a cached view SHALL NOT depend on how many times it has changed:
folding a revision into it makes every change a cache miss, and a cache miss is
the spinner this requirement forbids.

#### Scenario: A burst of changes does not blank the page
- **WHEN** several events arrive in quick succession for the view on screen
- **THEN** the content stays on screen throughout and updates in place

#### Scenario: Sending a message does not reload the conversation
- **WHEN** a person sends a message and the echo, the acknowledgement and the
  answer all arrive
- **THEN** the conversation is never replaced by a loading state

#### Scenario: Change count does not identify a view
- **WHEN** a view's cached data is looked up
- **THEN** the lookup does not depend on a count of changes seen

### Requirement: Every page is live on the same path
Every page the console serves SHALL stay current through this one mechanism.
None SHALL keep a private refresh of its own.

A page that cannot be updated from an event SHALL say why in the code that
excludes it, so the exception is a decision rather than an omission.

#### Scenario: The rule holds per page
- **WHEN** each page is open and a relevant object changes
- **THEN** it updates in place, with no request and no loading state

### Requirement: A refetch is an exception with a stated reason
Loading data over HTTP SHALL be limited to cases the stream cannot serve, and
each SHALL be named where it happens:

- FIRST LOAD, when the console holds nothing yet.
- RESYNC, when the client has provably missed events — a reconnect, or an
  activity gap the manager reported.
- An EXPLICIT ACTION by the reader.
- A DERIVED value that decays with TIME rather than with change, such as a rate.
  A rate is not wrong because something changed; it is wrong because time
  passed, and no event announces that.
- An AGGREGATE the browser cannot reconstruct from the object that changed — a
  count across every kind, a graph resolved over all of them, a relation BETWEEN
  objects, or an answer the manager alone computes. Recomputing one in the
  browser would be a second implementation of what the server says.

A timed refetch for anything else SHALL NOT exist.

An aggregate re-read SHALL keep its view on screen — the identity of the view
does not change, so the read happens underneath what is already rendered. A
burst of changes SHALL cost one re-read per view rather than one per change.

#### Scenario: A reconnect reloads
- **WHEN** the stream reconnects, or reports a gap
- **THEN** the console reloads, because it cannot know what it missed

#### Scenario: Rates still refresh on a timer
- **WHEN** a view shows a rate over a window
- **THEN** it may refresh on a timer, and the reason is stated where it is set

#### Scenario: Nothing else polls
- **WHEN** the console is inspected for timed refreshes
- **THEN** every one names a decaying value, and none exists to observe change

#### Scenario: An aggregate re-reads without blanking
- **WHEN** an object changes and a view derived from many objects is on screen
- **THEN** it re-reads, its content stays on screen throughout, and a burst
  costs one re-read rather than one per change

### Requirement: The cache is bounded
Applying events keeps a view's data current for as long as it is held, so
holding it forever is a decision the console SHALL NOT make by default.

- Data for a view NOT CURRENTLY ON SCREEN SHALL be evicted after a bounded
  idle period. A conversation read an hour ago SHALL NOT still occupy memory.
- A view remounted after that bound SHALL load fresh rather than render
  whatever was last applied to it.
- Nothing SHALL be persisted beyond the browsing session. The console SHALL
  write no cache to disk, so closing the tab SHALL leave nothing behind.

The bound exists for MEMORY, not for correctness: correctness comes from the
resync rule, which replaces applied state wholesale whenever the client may
have missed an event.

#### Scenario: An unused view is evicted
- **WHEN** a view has not been on screen for longer than the bound
- **THEN** its data is released, and returning to it loads fresh

#### Scenario: A held view stays current without refetching
- **WHEN** a view is on screen and events arrive for it
- **THEN** it stays current from those events, and the bound does not cause a
  refetch while it is held

#### Scenario: Nothing outlives the tab
- **WHEN** the console is closed and reopened
- **THEN** it starts from a snapshot, having stored nothing locally
