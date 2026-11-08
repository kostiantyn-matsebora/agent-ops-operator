# state-store Specification

## Purpose
The contract every backend for conversation state meets, the conformance
suite that defines it, and the one chart selector that picks a backend.
Conversation, ConversationInput and Signal are state, and this is the seam
that lets an install choose where state lives.

The `signal-persistence` change
(not yet archived) introduced `Signal` and its lifecycle. This change absorbed
its contract work and supersedes its direct-Kubernetes tasks.

## Requirements

### Requirement: State is reached only through the store contract
The manager SHALL read and write conversations, conversation inputs and
signals, each signal source's cooldown window and receipt counters, and the
key-value state adapters persist through `/channel/state` and
`/signal/state`, only through the state store contract.

Every other component SHALL read state only through the manager's state read
surface.

No code path in any component SHALL reach a state record through a Kubernetes
client, a Kubernetes cache or a Kubernetes watch directly.

The contract is one operation set over typed collections (see the
requirement of that name below). What the store holds is this closed inventory, and nothing
else:

| Collection | Record | Capabilities | Declared indexes | `kube` location |
|---|---|---|---|---|
| conversations | conversation control record | collection | by `initiationId`, by `pipelineRef`+phase, by phase, by parent, by word-chain, whole set | `Conversation` |
| runs | run, appended to a conversation | appended, grow-only edit, combined read with the parent | — | `Conversation.status.runs[]` |
| signals | signal | collection, expiring | whole set, one source's non-terminal, one source's non-terminal of one initiation id, one source's in any phase | `Signal` |
| cooldown windows | one source's window | collection | — | `SignalSource.status.cooldown` |
| receipt counters | one source's total and latest | collection | — | `SignalSource.status.receivedTotal`, `.lastReceived` |
| adapter state | one string per surface and key | collection | — | the `agentops.dev/adapter-state-<key>` annotation on `Channel` / `SignalSource` |
| blobs | an input payload | blobs | — | `ConversationInput` |

The contract SHALL expose domain records and an opaque revision token. It
SHALL NOT expose Kubernetes object metadata, labels, annotations, a namespace
argument or a selector string.

The contract SHALL be derived from what the manager does with state. It
SHALL NOT carry an operation, an option or a type that exists because one
backend offers it.

#### Scenario: A state access outside the store is a build failure
- **WHEN** a manager package imports the Kubernetes client to read or write a
  Conversation, a ConversationInput or a Signal, reads or writes a
  `SignalSource`'s `status.cooldown`, `status.receivedTotal` or
  `status.lastReceived`, or names the adapter state annotation prefix
- **THEN** the manager's own test suite fails, naming the package

#### Scenario: A state access in another component is a build failure
- **WHEN** the console or the housekeeping module names the `conversations`,
  `conversationinputs` or `signals` resources, or a `SignalSource`'s
  `status.cooldown`, `status.receivedTotal` or `status.lastReceived`, anywhere
  but its own client of the state read surface
- **THEN** that module's own test suite fails, naming the file and line

#### Scenario: Configuration kinds are untouched
- **WHEN** the manager reads a Pipeline, Channel, SignalSource or any other
  configuration kind
- **THEN** it reads it through the Kubernetes client as before, since
  configuration is not state and has no backend
- **AND** the cooldown window and the receipt counters, which the manager
  writes onto a SignalSource's status, go through the store as state
- **AND** so does the adapter state annotation on a Channel or SignalSource

### Requirement: Writes are compare-and-swap on an opaque revision
Every write of an existing record SHALL carry the revision the writer last
read. A write whose revision is not the record's current revision SHALL be
refused with a distinguishable conflict error and SHALL change nothing.

A create SHALL be refused when the key already exists.

#### Scenario: Two actors race for one record
- **WHEN** the leader's reconciler and an HTTP handler each read revision N
  and both write
- **THEN** exactly one write succeeds, the other receives a conflict, and the
  record carries exactly one of the two changes

#### Scenario: A stale claim is never merged
- **WHEN** a writer presents a revision older than the current one
- **THEN** the store refuses the write without applying any field of it

### Requirement: Lookup is by key or by a declared index
The contract SHALL offer lookup by key and listing by a declared, closed set
of indexes. A conversation SHALL be listable:

- by `initiationId`
- by `pipelineRef` and phase
- by phase
- by the parent that invoked it (its direct members)
- by the word-chain its name was allocated from
- as the whole set

Every backend SHALL serve every declared index.

The contract SHALL NOT accept a free-form selector. Adding an index SHALL be
a contract change with a conformance scenario, never a per-backend feature.

#### Scenario: Equality routing finds the open conversation
- **WHEN** a signal arrives whose `initiationId` matches a conversation that
  is not Closed
- **THEN** the index returns that conversation and no other

#### Scenario: Members are found by their parent
- **WHEN** a conversation invoked two members and one of those members
  invoked a third
- **THEN** the parent index returns the two direct members and not the third

#### Scenario: A word-chain lists every name allocated from it
- **WHEN** records were created under one word-chain, with and without a
  numeric suffix, and others under a different one
- **THEN** the word-chain index returns exactly the records of that chain

#### Scenario: The whole set can be listed
- **WHEN** the admission gate, the status view or the signal router needs
  every conversation
- **THEN** the whole-set index returns every record that has not expired,
  and no filter is applied

#### Scenario: A query outside the index set is not expressible
- **WHEN** a caller names a lookup the index set does not declare
- **THEN** the store refuses it as an unknown index, and the index set is
  extended by a change rather than worked around

### Requirement: The change feed is at-least-once and resumable
The contract SHALL offer a change feed over state records. A consumer SHALL
be able to resume the feed from a point it kept.

After resuming it SHALL observe every change made after that point, including
changes made while it was not consuming. Duplicate deliveries are permitted.
A gap is not.

Each event SHALL carry the record's key, its revision and the record as of
that revision.

#### Scenario: A new leader resumes from state alone
- **WHEN** a manager replica becomes leader after another replica held
  leadership and wrote records
- **THEN** its reconciler observes every record changed since the point it
  last consumed, with no in-memory hand-off from the former leader

#### Scenario: No gap across a consumer disconnect
- **WHEN** a consumer loses its feed connection while records change, then
  resumes from its kept point
- **THEN** it observes every change made during the disconnect

#### Scenario: A stale snapshot is caught by the write, not the read
- **WHEN** a consumer acts on the record an event carried and the record
  changed in between
- **THEN** its compare-and-swap write is refused and a later event carries
  the newer record

### Requirement: A record may carry an expiry
A record SHALL be able to carry an expiry instant. Past that instant the
store SHALL NOT return the record from a lookup or a listing, and SHALL
eventually remove it. How the removal happens is the backend's own.

#### Scenario: An expired record is invisible before it is gone
- **WHEN** a record's expiry instant has passed and the backend has not yet
  removed it
- **THEN** a lookup by key and every listing that would have returned it
  both omit it

#### Scenario: Expiry is observable on the feed
- **WHEN** the store removes an expired record
- **THEN** the feed carries a delete event for its key

### Requirement: Runs are append-only sub-records of a conversation
A conversation's runs SHALL be appended, never rewritten, and SHALL list in
the order they were appended.

An existing run accepts only growth, stated in the next Requirement.

The appended capability SHALL name its parent record type and SHALL offer
one combined read returning the parent record, its revision and the items in
append order in a single read.

An append SHALL take no revision from the caller and SHALL NOT fail with a
conflict. Whether an append also moves the control record's revision is the
backend's layout, and a caller's later control-record write tolerates it the
way it tolerates any conflict.

#### Scenario: An append needs no revision and never conflicts
- **WHEN** two actors append a run each to the same conversation at the same
  moment, neither holding a revision
- **THEN** both appends succeed and the listing holds both runs

#### Scenario: Runs read back in order
- **WHEN** runs 1 to N were appended
- **THEN** listing returns them in that order, every one of them

#### Scenario: A record and its runs in one read
- **WHEN** a record with runs is read with the combined operation
- **THEN** it returns the record and revision a plain read returns, and the runs
  in append order. An absent record is refused as not found by the plain read's
  rule.

### Requirement: An existing run accepts only growth
The appended capability SHALL offer one grow-only edit of an existing item,
keyed by the parent and the item's id, applying a merge the declaration
supplies. For runs the merge sets the delivery-tracked flag, sets the
routed-to-parent flag and adds channels to the delivered set. The merge
SHALL be idempotent and commutative, which the conformance suite proves for
every declaration, so the edit SHALL take no revision from the caller, SHALL
NOT fail with a conflict, and SHALL commute with an append.

The edit SHALL fail as not-found when the parent or the item does not exist,
so a caller whose run aged out of the bounded history can tell it from
success.

#### Scenario: An update applies once and again changes nothing
- **WHEN** the same edit is applied twice to one run, then another edit adds a
  channel already present and a new one
- **THEN** the flags are set, the delivered set holds each channel once in
  first-appearance order, and no other run changed

#### Scenario: Concurrent updates and an append all land
- **WHEN** several edits and an append reach one conversation at the same
  moment, none holding a revision
- **THEN** every one succeeds and the listing holds every effect

#### Scenario: A run that is not there is not-found
- **WHEN** an edit names a run or a conversation that does not exist
- **THEN** it fails as not-found and changes nothing

#### Scenario: A declared merge that does not commute is refused
- **WHEN** a declaration supplies a merge for which two edits in either order
  give different items
- **THEN** the conformance suite fails for that declaration on every backend

### Requirement: Input payloads are blobs by key
An input's payload SHALL be stored once under its own key, read by that key,
and deleted by that key. A payload SHALL NOT be embedded in the conversation's
control record.

#### Scenario: A pruned input releases its payload
- **WHEN** the queue entry an input belongs to is pruned
- **THEN** the payload is deleted by its key and a later read of that key
  reports it absent

### Requirement: Scope is fixed when the store is constructed
A store instance SHALL serve exactly one install's state, fixed at
construction. No operation SHALL take a scope, a namespace or a tenant
argument.

#### Scenario: Two installs never see each other
- **WHEN** two installs share one backend
- **THEN** a key written by one is absent from the other's lookups, listings
  and feed

### Requirement: A record carries a backend-minted identity
A stored record SHALL carry an opaque identity and the instant it was first
stored, both minted by the backend. The identity SHALL stay the same for the
record's whole life and SHALL differ between two records created one after
the other under the same key. Neither SHALL be settable by a caller: a write
carrying them SHALL have them ignored.

#### Scenario: Identity is stable across updates and new after a recreate
- **WHEN** a record is read, updated, deleted and created again under its key
- **THEN** its identity and creation instant are unchanged by the update, and
  the recreated record has a different identity

#### Scenario: A caller cannot choose the identity
- **WHEN** a create or an update carries an identity, a creation instant or a
  deletion marker
- **THEN** the record read back carries the backend's own values

### Requirement: Deletion is requested, then completed by releasing finalizers
A record SHALL carry a set of finalizers, written by the manager through an
ordinary update. A delete of a record holding none SHALL remove it at once.
A delete of a record holding any SHALL NOT remove it: the record SHALL stay
readable by lookup, by listing and on the feed, carrying a deletion-requested
instant, until an update leaves its finalizer set empty, which SHALL remove
it. A repeated delete SHALL keep the first instant. On a record marked for
deletion an update SHALL NOT add a finalizer, and SHALL be refused with a
distinguishable deleting error.

Marking SHALL appear on the feed as a change to the record. Removal SHALL
appear as a delete event.

A create under the key of a record still held SHALL be refused as already
existing.

#### Scenario: A record with no finalizer is removed at once
- **WHEN** a record holding no finalizer is deleted
- **THEN** a lookup finds nothing, the feed carries a delete event, and
  deleting the absent key again is not an error

#### Scenario: A held record stays visible with the marker
- **WHEN** a record holding a finalizer is deleted, and deleted again later
- **THEN** it is still returned by lookup and listing carrying the instant
  of the first delete, and the feed carried a change to it carrying the marker

#### Scenario: Releasing the last finalizer removes the record
- **WHEN** a marked record holding two finalizers has one released, then the
  other, the second update also changing its phase
- **THEN** the first update leaves the record readable and marked, the second
  succeeds, the record is gone, and the feed carries a delete event

#### Scenario: A finalizer cannot be added to a marked record
- **WHEN** an update adds a finalizer to a record marked for deletion
- **THEN** it is refused as deleting and changes nothing

#### Scenario: A key still held cannot be created again
- **WHEN** a create names the key of a marked record
- **THEN** it is refused as already existing

### Requirement: Instants are UTC
Every instant the contract returns SHALL be in UTC, whatever location the
backend decoded it in or the caller wrote it in. The same instant SHALL
compare equal.

#### Scenario: Instants read back in UTC whatever was written
- **WHEN** a record and a run are written with instants in another zone
- **THEN** every instant read back is in UTC and equals the instant written

### Requirement: A signal is created once under its key
The contract SHALL offer creating a signal under a key the caller derives. A
create under a key already held SHALL be refused with the distinguishable
already-exists error in every phase the signal can be in, and SHALL change
nothing. Publication SHALL treat that refusal as the duplicate of a message it
already stored, with no lookup before the create.

A create SHALL accept the `sourceRef`, the opaque reference to the source the
signal arrived on, so a backend that links a record to its owner can.

No behaviour SHALL depend on a backend turning that `sourceRef` into an owner
reference or any other cascade. The `sourceRef` is still recorded, and the release
requirement below reads it to keep a successor source's signals.

#### Scenario: A second create under one key is the duplicate
- **WHEN** a signal is created twice under one key, the second time with a
  different payload
- **THEN** the second is refused as already existing and the stored payload is
  the first's

#### Scenario: A race has one winner
- **WHEN** several creates under one key run at the same moment
- **THEN** exactly one succeeds and every other is refused as already existing

#### Scenario: An expired signal does not hold its key
- **WHEN** a create names the key of a signal whose expiry has passed and the
  backend has not yet removed it
- **THEN** it succeeds and the stored record is the new one: an invisible
  record is not a duplicate

#### Scenario: A finished signal still holds its key
- **WHEN** a create names the key of a signal whose phase is terminal and not
  yet expired
- **THEN** it is refused as already existing

### Requirement: A signal's status is written by compare-and-swap, its message never
An update of a signal SHALL carry the revision the writer last read, SHALL
change only the status fields and the expiry, and SHALL be refused with the
conflict error when the revision is not current, applying no field of it.
Whatever an update carries in the publication fields, the identity or the
creation instant SHALL be ignored. Writing an absent or expired signal SHALL
fail as not-found.

The contract SHALL NOT offer a write to a signal that takes no revision: every
status write is the consumer's or housekeeping's, runs on the leader through
one work queue, and repeats its pass when it loses a race.

#### Scenario: A stale status write is never merged
- **WHEN** a writer presents a revision older than the current one
- **THEN** the store refuses it as a conflict and the record keeps the
  winner's status

#### Scenario: The message cannot be rewritten
- **WHEN** an update carries a different source, initiation id, payload, title
  and labels
- **THEN** the record read back carries the publication's values and the
  update's status

#### Scenario: Every status field round-trips
- **WHEN** a signal is updated with a phase, attempts, first-attempt, completion
  and consumption instants, the consumer's identity and three claimants in
  three different states
- **THEN** a read returns every field as written and the claimants in order

### Requirement: Signal lookup is by key or by a declared index
The contract SHALL offer lookup of a signal by key and listing by a declared,
closed set of signal indexes:
- the whole set
- one source's non-terminal signals
- one source's non-terminal signals of one initiation id
- one source's signals in any phase, read by the release of that source

Every backend SHALL serve every one. A terminal signal SHALL never be returned by a
non-terminal index, so a flapping source's finished records do not slow the
per-source cap. A signal whose phase is absent SHALL count as non-terminal.

The contract SHALL NOT accept a free-form selector. Listing order is
unspecified. The contract SHALL offer one ordering, arrival, by creation
instant and then name.

#### Scenario: The cap counts only what is live
- **WHEN** a source has live, partially routed and terminal signals and
  another source has a live one
- **THEN** the non-terminal-by-source index returns the first source's live and
  partially routed signals and nothing else

#### Scenario: Earlier messages of one conversation are found
- **WHEN** a source holds three live signals of one initiation id, one live
  signal of another, and a terminal one of the first
- **THEN** the non-terminal-by-initiation index returns exactly the three

#### Scenario: Arrival order is creation then name
- **WHEN** two signals share a creation instant
- **THEN** arrival orders them by name, and an earlier creation instant
  arrives first

#### Scenario: A query outside the signal indexes is not expressible
- **WHEN** a caller names an index the contract does not declare
- **THEN** the store refuses it as an unknown index

### Requirement: A terminal signal expires instead of being deleted
The consumer SHALL write a signal's expiry in the same update that makes its
phase terminal: the completion instant plus that phase's time to live. The
store SHALL apply the contract's expiry rules to it: invisible to lookup and
listing once passed, and removed with a delete event on the feed.

No second writer SHALL delete a terminal signal, so no conditional delete
exists.

The one named exception is the release of a deleted source. It removes every
signal of that source in any phase.

A terminal signal found without an expiry, written by a version that did not
set one, SHALL be given one from its completion instant by the consumer's next
pass.

#### Scenario: A terminal write carries its expiry
- **WHEN** an update sets a Routed phase, a completion instant and an expiry
  an hour ahead
- **THEN** a read returns all three, and an update moving the expiry into the
  past makes the signal invisible

#### Scenario: A terminal signal found without an expiry is given one
- **WHEN** a signal with a terminal phase and a completion instant carries no
  expiry, and the consumer makes its next pass
- **THEN** the signal is updated with an expiry of its completion instant plus
  that phase's time to live

#### Scenario: An expired signal is invisible, then a delete event
- **WHEN** a signal's expiry has passed
- **THEN** a lookup and every listing omit it, and the feed carries a delete
  event for its key once the backend removes it

### Requirement: A signal feed is at-least-once and resumable
The contract SHALL offer a change feed over signals with the semantics of the
conversation feed: events carry the key, the revision and the record, a
consumer resumes from a point it kept and observes every change made after it,
duplicates are permitted and a gap is not. A consumer SHALL be able to start
with no point and then list every signal, so a new leader finds every
non-terminal signal from stored state alone.

#### Scenario: A change and a removal are on the feed
- **WHEN** a signal is created and then deleted
- **THEN** the feed carries a put with the record at the created revision, and
  a delete event for its key

#### Scenario: Resuming sees what happened while away
- **WHEN** a consumer drops its feed, a signal is created and another is
  updated, and the consumer resumes from its kept point
- **THEN** it observes both changes

### Requirement: A signal is removed at once when asked
A delete of a signal SHALL remove it immediately and SHALL NOT be an error when
the key is absent. A signal holds no finalizer and has no marked state.

#### Scenario: Delete is immediate and repeatable
- **WHEN** a signal is deleted and then deleted again
- **THEN** the first removes it, a read reports not-found, and the second
  succeeds

### Requirement: A source's cooldown window is state
The cooldown window of one source, the initiation ids last admitted on it and
when, SHALL be a collection keyed by the source. A source with nothing
recorded SHALL read as not found, which the router's helper SHALL read as an
empty window and not as an error. A replace SHALL go through the collection's
compare-and-swap, retried by the helper, so the caller presents no revision.
The leader is the only writer and serialises each source. A lost write costs
a duplicate investigation after a restart, never a stuck signal. A caller
SHALL tolerate a failed replace by logging it. Two sources SHALL never see
each other's window.

The window's location under the `kube` backend SHALL stay
`SignalSource.status.cooldown`, written by the same status merge patch as
before. The contract moves the access and leaves that object where it is.

#### Scenario: Nothing recorded reads empty
- **WHEN** a window is read for a source that has none
- **THEN** the collection reports not found and the helper returns an empty
  window and no error

#### Scenario: A replace is whole and per source
- **WHEN** two sources each have a window and one is replaced, then cleared
- **THEN** a read of that source returns exactly the replacement, then an empty
  window, and the other source's window never changed

#### Scenario: Instants read back in UTC
- **WHEN** an entry is written with an instant in another zone
- **THEN** it reads back as the same instant in UTC

### Requirement: A source's receipt counters are state
The receipt counters of one source, the total routed and the latest instant,
SHALL be a collection keyed by the source. Counting one routed signal SHALL
go through the collection's compare-and-swap, retried by the router's helper
(the total grows by one and the latest instant becomes the given one), so
concurrent counts are all kept and no increment is lost. A source the
backend does not know SHALL read as not found, which the helper reads as
zero, and SHALL refuse a count as not found, which the helper logs. Under the
`kube` backend the counters SHALL stay `SignalSource.status.receivedTotal`
and `lastReceived`.

#### Scenario: Concurrent receipts are all counted
- **WHEN** several receipts are recorded on one source at the same moment
- **THEN** the total equals their number, the latest instant reads back in UTC,
  and another source's counters did not move

### Requirement: A deleted source's signals are released by the manager
The manager SHALL release a source through a helper over the contract. The helper:

- lists the source's signals by the any-phase source index
- deletes each one under the revision it listed
- clears the source's cooldown window

The receipt counters are not cleared. Under the `kube` backend they live on the source object, which release does not touch.

Release is idempotent and raises no error for a source with none.

The manager SHALL call it when a `SignalSource` is observed gone. It SHALL also release the orphan a pass finds, so no cleanup depends on an owner reference being honoured. The `kube` backend MAY still set the owner reference.

Release is a helper, not an operation of the contract. The any-phase source index exists for it.

A name can be reused by a successor source, so the release SHALL decide from the source that holds the name at the moment of each delete.

After listing the signals, the release SHALL read the source by that name from the authoritative store of configuration, never from a cache. Under the `kube` backend that is the Kubernetes API server.

A signal recorded under the identity of the source that holds the name now SHALL be kept. Every other signal of the name SHALL be deleted under the revision it was listed at.

A conflict SHALL re-read that signal and decide again, with exponential back-off and a bounded number of attempts.

The cooldown window SHALL be cleared only when no source holds the name. The caller supplies no identity, so a successor created at any point during the release keeps its signals.

#### Scenario: Release removes one source and no other
- **WHEN** a source with a live, a terminal signal and a window is released
  twice, beside a second source with a live signal and a window
- **THEN** the first source's signals and window are gone, and the second's are
  untouched

#### Scenario: A successor's signals outlive the departed release
- **WHEN** a source is released by name while a signal recorded under the
  identity of a source that now holds that name exists
- **THEN** the previous incarnation's signals are gone and the successor's stay,
  and under `kube` so does the successor's window

#### Scenario: A signal that changes during release is decided again
- **WHEN** a listed signal is modified before its delete is attempted
- **THEN** the delete conflicts, the signal is read again, and the decision is
  made on the version read last

#### Scenario: Release without a garbage collector
- **WHEN** a `SignalSource` is deleted in an environment where owner-reference
  garbage collection does not run
- **THEN** its signals are removed by the manager within its next reconciliation

### Requirement: Signal scope and instants follow the conversation rules
A signal store SHALL serve exactly one install, fixed at construction, and SHALL
carry a backend-minted identity and creation instant that a caller cannot set.
Every instant it returns SHALL be in UTC.

#### Scenario: Two installs never see each other's signals
- **WHEN** two installs share one backend
- **THEN** a key written by one is absent from the other's lookups and listings

#### Scenario: Identity is minted and stable
- **WHEN** a create carries an identity and a creation instant, the signal is
  updated, and then deleted and created again under its key
- **THEN** the identity is the backend's, unchanged by the update, and
  different after the recreate

#### Scenario: Instants read back in UTC
- **WHEN** a signal is updated with instants in another zone
- **THEN** every instant read back is in UTC and equals the one written

### Requirement: The contract offers no multi-record atomicity
The contract SHALL NOT offer a transaction across records. Every manager path
that writes two records SHALL tolerate a failure between the two and SHALL
be re-derivable from whichever write landed. That includes a signal's terminal
update followed by the conversation it routed to. A status write and its expiry
are one update (see the terminal-signal requirement), so they need no such
tolerance.

#### Scenario: A crash between a conversation write and its input write
- **WHEN** the manager stops after writing a conversation and before writing
  its input payload
- **THEN** the next reconciliation either completes the input or records the
  conversation as owed one, and nothing is silently lost

### Requirement: The conformance suite defines the contract
There SHALL be one conformance suite, generic over the record type the way
the contract is, and every backend SHALL pass all of it to be shipped. The
suite SHALL run every capability's scenarios against every field of the
inventory, driven by the inventory itself, so a backend's test is one call
and a kind of state cannot be left out of it. Every requirement in this
capability SHALL be one or more scenarios of the suite. A backend-specific
scenario and a kind-specific scenario SHALL NOT exist.

The suite SHALL also measure, per backend, the bytes written per run, the
allocations per write and the records resident after a listing, and SHALL
report them so backends compare on one table.

#### Scenario: A backend that fails one scenario is not shippable
- **WHEN** a backend passes every scenario but one
- **THEN** its build fails and the chart cannot select it

#### Scenario: The bench runs against every backend
- **WHEN** the suite runs
- **THEN** it reports the same measurements for every backend it covers

#### Scenario: A kind of state cannot be left out of the suite
- **WHEN** a collection is added to the inventory and a backend has no layout
  for it
- **THEN** that backend's single conformance test fails, naming the field

### Requirement: The `kube` backend preserves today's behaviour
The `kube` backend SHALL store state as the same custom resources as before
the store existed: same schema, same field names, same status subresource.
That covers a `Signal` and the cooldown list on `SignalSource` status.

An object written through it SHALL be indistinguishable from one written
before. `kubectl` SHALL read and list them unchanged.

A signal's source identity maps to the controller owner reference naming its
`SignalSource`, its status to the status subresource, and its expiry to the
same annotation a conversation uses. The owner reference mapping is best-effort
and no behaviour depends on it.

The record's finalizers map to the object's finalizers, its deletion marker to
the object's deletion timestamp, its identity to the object's UID, and the
parent and word-chain lookups to the two labels the manager always wrote.
Metadata the record does not model, whoever set it, SHALL survive a write
through the store.

It SHALL be the default backend and SHALL render no bundle.

#### Scenario: Objects are byte-identical
- **WHEN** the same conversation lifecycle runs before and after the store
- **THEN** the Conversation and ConversationInput objects differ in nothing
  but timestamps and revisions, and the one annotation carrying `expiresAt`
  on a record that sets one (no CRD schema change)

#### Scenario: A signal is the object publication always wrote
- **WHEN** a signal is created through the store and another by the call
  publication made before the store existed
- **THEN** their specs and owner references are equal, and the first carries
  no annotation unless it set an expiry

#### Scenario: The system pack passes unchanged
- **WHEN** the system pack runs against a default install
- **THEN** every lane passes without a test being edited

### Requirement: Exactly one backend is selected
The chart SHALL expose one selector naming the state backend. The render
SHALL fail, naming the value, on any of:

- a backend the chart does not know
- a backend whose bundle is not enabled
- more than one backend's bundle enabled

The manager SHALL learn its backend from the chart alone and SHALL refuse to
start on a name it does not implement.

#### Scenario: An unknown backend fails the render
- **WHEN** the selector names a backend the chart has no bundle for
- **THEN** the render fails and the message names the value

#### Scenario: The default needs nothing
- **WHEN** the selector is left at its default
- **THEN** the render enables no bundle and the manager starts on the `kube`
  backend

### Requirement: Workloads are released by the manager, not by ownership
The manager SHALL delete a conversation's runtime pod and ConfigMap itself
when the conversation closes or is deleted, and SHALL release a deleted
source's signals itself. No behaviour SHALL depend on a Kubernetes owner
reference being honoured. The `kube` backend MAY still set the owner
reference.

#### Scenario: Release without a garbage collector
- **WHEN** a conversation closes in an environment where owner-reference
  garbage collection does not run
- **THEN** its pod and ConfigMap are deleted by the manager within the same
  reconciliation

### Requirement: The contract is one operation set over typed collections
The contract SHALL consist of capability interfaces generic over the record
type: a collection (get, create, compare-and-swap update, delete, list by a
declared index, feed), an expiring collection, an appended sub-record with
its parent type, and blobs. A backend SHALL implement each capability once,
for every record type, and SHALL NOT implement an operation that names one
kind of state.

Every kind of state SHALL be one typed field of one inventory struct. The field's type SHALL name exactly the capabilities that kind has.

A capability a kind does not have SHALL NOT be callable on it.

An index SHALL be typed on the record it indexes. A collection of another record SHALL NOT accept it.

Adding a kind of state SHALL be one field with its declaration: name, indexes, expiry, and a merge for a grow-only edit. For the `kube` backend alone it also adds a layout onto an existing object.

It SHALL NOT add an operation to the contract or code to a backend's primitives.

Domain operations that read more than one record or decide something (the
release of a source, the replace of a window, a receipt count) SHALL live
above the contract, in the package that owns the decision, as helpers over
the typed collections.

#### Scenario: A kind of state is added without touching a backend
- **WHEN** a new collection is declared in the inventory with its fixture
- **THEN** every backend passes the conformance suite for it with only a
  layout added under `kube`, and no primitive of any backend changed

#### Scenario: An absent capability is not callable
- **WHEN** code appends to the conversation collection or sets an expiry on a
  conversation
- **THEN** it does not compile, since the conversation field's type carries
  neither capability

#### Scenario: An index of one kind is refused for another
- **WHEN** code lists the signal collection by a conversation index
- **THEN** it does not compile

#### Scenario: The inventory is the whole of what the store holds
- **WHEN** a reader asks what state the store keeps
- **THEN** the inventory struct answers completely, one field per kind, with
  its capabilities and indexes beside it

### Requirement: A delete MAY be conditional on a revision
A collection SHALL offer a delete conditional on a revision, beside its unconditional delete.

A caller that read a record and decided on it SHALL be able to remove exactly that revision.

A revision that is not the record's current one SHALL be refused with the conflict error. The record SHALL stay intact.

An absent key SHALL be refused with the not-found error.

A record whose removal is held SHALL be marked for deletion, not removed, as for the unconditional delete.

The unconditional delete remains for a caller that never read first.

#### Scenario: A stale revision is refused
- **WHEN** a record is rewritten after a caller read it
- **AND** the caller deletes it under the revision it read
- **THEN** the delete fails with the conflict error
- **AND** the record is unchanged

#### Scenario: The current revision removes the record
- **WHEN** a caller deletes a record under its current revision
- **THEN** the record is removed

#### Scenario: The source release deletes under the revision it read
- **WHEN** a signal is rewritten between the release's decision and its delete
- **THEN** the signal is not deleted on the stale revision
- **AND** the release reads it again and decides again

### Requirement: A listing and a feed event can carry a record's appended items
A collection that has an appended capability SHALL offer one listing that
returns each record with its appended items, over every declared index of
that collection, and a put event on its feed SHALL carry the record's
appended items as of that revision. Both return what the combined read
returns for each record. How a backend joins the items to the records is its
own.

#### Scenario: A listing with items equals the combined read
- **WHEN** records with and without runs exist and the whole set is listed
  with runs
- **THEN** every record is returned with exactly the runs the combined read
  returns for it, in append order

#### Scenario: A put after an append carries the run
- **WHEN** a run is appended and the record is then updated
- **THEN** the put event the update produces carries the appended run

### Requirement: An adapter's persisted key-value state is a collection
The key-value state an adapter persists through the manager SHALL be stored
only through the contract, keyed by the kind of surface (channel or signal
source), its name and the adapter's key. A write SHALL replace the value and
the last write SHALL win. A read of a key never written SHALL report it
absent in the way each adapter contract documents. Under the `kube` backend a
value SHALL stay the annotation `agentops.dev/adapter-state-<key>` on the
Channel or SignalSource, so a cursor written before this requirement reads
back after it.

Deleting a Channel or SignalSource SHALL release its keys by the manager's
act, so no behaviour depends on an owner reference being honoured.

#### Scenario: A cursor survives an adapter restart
- **WHEN** an adapter writes a cursor, restarts and reads the same key
- **THEN** it reads the value it wrote

#### Scenario: A cursor written before the contract reads back after it
- **WHEN** an object already carries the adapter state annotation and the
  manager reads the key through the collection
- **THEN** it returns that value, and a write through the collection leaves
  the same annotation on the same object

#### Scenario: Two surfaces never share a key
- **WHEN** a channel and a signal source each store a value under one key
- **THEN** each reads its own value
