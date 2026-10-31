## Purpose

The contract every backend for conversation state meets, the conformance
suite that defines it, and the one chart selector that picks a backend.
Conversation, ConversationInput and Signal are state, and this is the seam
that lets an install choose where state lives.

## ADDED Requirements

### Requirement: State is reached only through the store contract
The manager SHALL read and write conversations and conversation inputs only
through the state store contract. No manager code path SHALL reach a state
record through a Kubernetes client, a Kubernetes cache or a Kubernetes watch
directly.

The Signal record joins the contract with `signal-persistence`, as the
contract extension that change owns.

The contract SHALL expose domain records and an opaque revision token. It
SHALL NOT expose Kubernetes object metadata, labels, annotations, a namespace
argument or a selector string.

The contract SHALL be derived from what the manager does with state. It
SHALL NOT carry an operation, an option or a type that exists because one
backend offers it.

#### Scenario: A state access outside the store is a build failure
- **WHEN** a manager package imports the Kubernetes client to read or write a
  Conversation or a ConversationInput
- **THEN** the manager's own test suite fails, naming the package

#### Scenario: Configuration kinds are untouched
- **WHEN** the manager reads a Pipeline, Channel, SignalSource or any other
  configuration kind
- **THEN** it reads it through the Kubernetes client as before, since
  configuration is not state and has no backend

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
of indexes: a conversation by `initiationId`, by `pipelineRef` and phase,
by phase, by the parent that invoked it (its direct members), by the
word-chain its name was allocated from, and the whole set. Every backend
SHALL serve every declared index.

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
- **WHEN** a caller needs a lookup the index set does not declare
- **THEN** the contract offers no way to express it, and the index set is
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

The one change an existing run accepts is growth: its delivery-tracked flag
and its routed-to-parent flag going from false to true, and channels being
added to its delivered set. No field is cleared, removed or overwritten.

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

### Requirement: An existing run accepts only growth
The contract SHALL offer one operation on an existing run, keyed by the
conversation and the run's id: set its delivery-tracked flag, set its
routed-to-parent flag, and add channels to its delivered set. Each effect
only grows the run, so the operation SHALL take no revision from the caller,
SHALL NOT fail with a conflict, SHALL be idempotent, and SHALL commute with
itself and with an append.

The operation SHALL fail as not-found when the conversation or the run does
not exist, so a caller whose run aged out of the bounded history can tell it
from success.

#### Scenario: An update applies once and again changes nothing
- **WHEN** the same update is applied twice to one run, then another update
  adds a channel already present and a new one
- **THEN** the flags are set, the delivered set holds each channel once in
  first-appearance order, and no other run changed

#### Scenario: Concurrent updates and an append all land
- **WHEN** several updates and an append reach one conversation at the same
  moment, none holding a revision
- **THEN** every one succeeds and the listing holds every effect

#### Scenario: A run that is not there is not-found
- **WHEN** an update names a run or a conversation that does not exist
- **THEN** it fails as not-found and changes nothing

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

### Requirement: The contract offers no multi-record atomicity
The contract SHALL NOT offer a transaction across records. Every manager path
that writes two records SHALL tolerate a failure between the two and SHALL
be re-derivable from whichever write landed.

#### Scenario: A crash between a conversation write and its input write
- **WHEN** the manager stops after writing a conversation and before writing
  its input payload
- **THEN** the next reconciliation either completes the input or records the
  conversation as owed one, and nothing is silently lost

### Requirement: The conformance suite defines the contract
There SHALL be one conformance suite, and every backend SHALL pass all of it
to be shipped. Every requirement in this capability SHALL be one or more
scenarios of the suite. A backend-specific scenario SHALL NOT exist.

The suite SHALL also measure, per backend, the bytes written per run, the
allocations per write and the records resident after a listing, and SHALL
report them so backends compare on one table.

#### Scenario: A backend that fails one scenario is not shippable
- **WHEN** a backend passes every scenario but one
- **THEN** its build fails and the chart cannot select it

#### Scenario: The bench runs against every backend
- **WHEN** the suite runs
- **THEN** it reports the same measurements for every backend it covers

### Requirement: The `kube` backend preserves today's behaviour
The `kube` backend SHALL store state as the same custom resources as before
the store existed: same schema, same field names, same status subresource.

An object written through it SHALL be indistinguishable from one written
before. `kubectl` SHALL read and list them unchanged.

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
when the conversation closes or is deleted. No behaviour SHALL depend on a
Kubernetes owner reference being honoured. The `kube` backend MAY still set
the owner reference.

#### Scenario: Release without a garbage collector
- **WHEN** a conversation closes in an environment where owner-reference
  garbage collection does not run
- **THEN** its pod and ConfigMap are deleted by the manager within the same
  reconciliation
