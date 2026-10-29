## Purpose

The contract every backend for conversation state meets, the conformance
suite that defines it, and the one chart selector that picks a backend.
Conversation, ConversationInput and Signal are state, and this is the seam
that lets an install choose where state lives.

## ADDED Requirements

### Requirement: State is reached only through the store contract
The manager SHALL read and write conversations, conversation inputs and
signals only through the state store contract. No manager code path SHALL
reach a state record through a Kubernetes client, a Kubernetes cache or a
Kubernetes watch directly.

The contract SHALL expose domain records and an opaque revision token. It
SHALL NOT expose Kubernetes object metadata, labels, annotations, a namespace
argument or a selector string.

The contract SHALL be derived from what the manager does with state. It
SHALL NOT carry an operation, an option or a type that exists because one
backend offers it.

#### Scenario: A state access outside the store is a build failure
- **WHEN** a manager package imports the Kubernetes client to read or write a
  Conversation, ConversationInput or Signal
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
of indexes: a conversation by `initiationId`, by `pipelineRef` and phase, by
phase, and a signal by source and by phase. Every backend SHALL serve every
declared index.

The contract SHALL NOT accept a free-form selector. Adding an index SHALL be
a contract change with a conformance scenario, never a per-backend feature.

#### Scenario: Equality routing finds the open conversation
- **WHEN** a signal arrives whose `initiationId` matches a conversation that
  is not Closed
- **THEN** the index returns that conversation and no other

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

#### Scenario: An expired signal is invisible before it is gone
- **WHEN** a signal's expiry instant has passed and the backend has not yet
  removed it
- **THEN** a lookup by key and a listing by source both omit it

#### Scenario: Expiry is observable on the feed
- **WHEN** the store removes an expired record
- **THEN** the feed carries a delete event for its key

### Requirement: Runs are append-only sub-records of a conversation
A conversation's runs SHALL be appended, never rewritten, and SHALL list in
the order they were appended. Appending a run SHALL NOT require rewriting the
conversation's control record.

#### Scenario: A run appends without a control-record conflict
- **WHEN** a run is appended while another actor holds the control record at
  an older revision
- **THEN** the append succeeds and the other actor's later control-record
  write is judged on the control record's revision alone

#### Scenario: Runs read back in order
- **WHEN** runs 1 to N were appended
- **THEN** listing returns them in that order, every one of them

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

It SHALL be the default backend and SHALL render no bundle.

#### Scenario: Objects are byte-identical
- **WHEN** the same conversation lifecycle runs before and after the store
- **THEN** the Conversation and ConversationInput objects differ in nothing
  but timestamps and revisions

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
