## MODIFIED Requirements

### Requirement: State is reached only through the store contract
The manager SHALL read and write conversations, conversation inputs and
signals, each signal source's cooldown window and receipt counters, and the
key-value state adapters persist through `/channel/state` and
`/signal/state`, only through the state store contract. Every other component
SHALL read state only through the manager's state read surface. No code path
in any component SHALL reach a state record through a Kubernetes client, a
Kubernetes cache or a Kubernetes watch directly.

The contract is one operation set over typed collections (next
requirement). What the store holds is this closed inventory, and nothing
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
  configuration is not state and has no backend. Two fields the manager
  writes onto a SignalSource's status are the exception: the cooldown window
  and the receipt counters are state, and go through the store. So is the
  adapter state annotation on a Channel or SignalSource.

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

#### Scenario: Concurrent edits and an append all land
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
The manager SHALL release a source through a helper over the contract: list
its signals by the any-phase source index, delete each under the revision it
listed, and clear its cooldown window. The receipt counters are not cleared:
under the `kube` backend they live on the source object, which release does
not touch. It is idempotent and raises no error for a source with none.

The manager SHALL call it when a `SignalSource` is observed gone. It SHALL also
release the orphan a pass finds, so no cleanup depends on an owner reference
being honoured. The `kube` backend MAY still set the owner reference.

Release is a helper, not an operation of the contract, and the any-phase
source index exists for it.

A name can be reused by a successor source, so the release SHALL decide from
the source that holds the name at the moment of each delete. After listing the
source's signals it SHALL read the source by that name from the Kubernetes API,
never from a cache. A signal recorded under the identity of the source that
holds the name now SHALL be kept.

Every other signal of the name SHALL be deleted under the revision it was
listed at. A conflict SHALL re-read that signal and decide again, with
exponential back-off and a bounded number of attempts.

The cooldown window SHALL be cleared only when no source holds the name. The
caller supplies no identity, so a successor created at any point during the
release keeps its signals.

#### Scenario: Release removes one source and no other
- **WHEN** a source with a live, a terminal signal and a window is released
  twice, beside a second source with a live signal and a window
- **THEN** the first source's signals and window are gone, and the second's are
  untouched

#### Scenario: A successor's signals survive a release made with only the name
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

## ADDED Requirements

### Requirement: The contract is one operation set over typed collections
The contract SHALL consist of capability interfaces generic over the record
type: a collection (get, create, compare-and-swap update, delete, list by a
declared index, feed), an expiring collection, an appended sub-record with
its parent type, and blobs. A backend SHALL implement each capability once,
for every record type, and SHALL NOT implement an operation that names one
kind of state.

Every kind of state SHALL be one typed field of one inventory struct, and the
field's type SHALL name exactly the capabilities that kind has. A capability a
kind does not have SHALL NOT be callable on it. An index SHALL be typed on
the record it indexes and SHALL NOT be accepted by a collection of another
record.

Adding a kind of state SHALL be one field with its declaration (name,
indexes, expiry, merge for a grow-only edit) and, for the `kube` backend
alone, its layout onto an existing object. It SHALL NOT add an operation to
the contract or code to a backend's primitives.

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
