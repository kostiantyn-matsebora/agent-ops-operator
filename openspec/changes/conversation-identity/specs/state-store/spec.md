## MODIFIED Requirements

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
| conversations | conversation control record | collection | by `correlationId`, by `ID`, by `pipelineRef`+phase, by phase, by parent, by word-chain, whole set | `Conversation` |
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

### Requirement: Lookup is by key or by a declared index
The contract SHALL offer lookup by key and listing by a declared, closed set
of indexes. A conversation SHALL be listable:

- by `correlationId`, the `initiationId` the conversation adopted at creation
- by `ID`, the identity the backend minted for it
- by `pipelineRef` and phase
- by phase
- by the parent that invoked it (its direct members)
- by the word-chain its name was allocated from
- as the whole set

Every backend SHALL serve every declared index.

The contract SHALL NOT accept a free-form selector. Adding an index SHALL be
a contract change with a conformance scenario, never a per-backend feature.

#### Scenario: Equality routing finds the open conversation
- **WHEN** a signal arrives whose `initiationId` matches the `correlationId` of a conversation that
  is not Closed
- **THEN** the index returns that conversation and no other

#### Scenario: A conversation is found by its identity
- **WHEN** two conversations were created one after the other under the same name, the first was deleted, and a caller lists by the first one's `ID`
- **THEN** the index returns nothing, and listing by the second one's `ID` returns the second and no other

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

## ADDED Requirements

### Requirement: A create returns the record as stored

`Create` on a collection SHALL return the record as the backend stored it, with the identity and creation instant the backend minted, and its revision. The caller SHALL NOT have to read the record back to learn them. A create refused because the key exists SHALL return no record and the existing conflict error.

This SHALL hold for every collection of the inventory and every backend, asserted by the conformance suite over every field of the inventory, so a backend that returns the caller's own input unchanged fails it. A collection whose record carries no identity SHALL return it as stored, which is the input.

#### Scenario: The minted identity is in the returned record

- **WHEN** a conversation is created with no identity
- **THEN** the returned record carries a non-empty `ID` and the `CreatedAt` the store holds, and a `Get` of the same key returns the same two values

#### Scenario: A caller-supplied identity is not echoed

- **WHEN** a create carries an `ID` and a `CreatedAt`
- **THEN** the returned record carries the backend's own values, as `A record carries a backend-minted identity` requires of the stored one

#### Scenario: A conflicting create returns no record

- **WHEN** a create is refused because the key exists
- **THEN** the conflict error is returned and no record is, and the existing record is unchanged

#### Scenario: Every kind of state is covered

- **WHEN** a collection is added to the inventory
- **THEN** the conformance suite asserts its create returns the stored record, with no per-kind test written for it
