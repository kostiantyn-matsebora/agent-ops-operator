## MODIFIED Requirements

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

It SHALL be selectable as `kube` and SHALL render no bundle. It SHALL NOT be
the default.

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
- **WHEN** the system pack runs against an install that selects `kube`
- **THEN** every lane passes without a test being edited

#### Scenario: Selecting it renders no bundle
- **WHEN** the selector names `kube`
- **THEN** the render enables no backend bundle

### Requirement: Exactly one backend is selected
The chart SHALL expose one selector naming the state backend, `nats` by
default. The render SHALL fail, naming the value, on any of:

- a backend the chart does not know
- a backend whose bundle is not enabled
- more than one backend's bundle enabled

The default SHALL enable the `nats` bundle without the adopter setting a
second value. The manager SHALL learn its backend from the chart alone and
SHALL refuse to start on a name it does not implement.

#### Scenario: An unknown backend fails the render
- **WHEN** the selector names a backend the chart has no bundle for
- **THEN** the render fails and the message names the value

#### Scenario: The default needs nothing
- **WHEN** the selector is left at its default
- **THEN** the render includes the `nats` bundle without the adopter
  enabling it, and the manager starts on the `nats` backend

#### Scenario: Pinning kube needs one value
- **WHEN** the selector names `kube`
- **THEN** the render succeeds with no bundle, without setting
  `nats.enabled`

#### Scenario: A bundle left on under another backend fails
- **WHEN** the selector names `kube` and `nats.enabled` is true
- **THEN** the render fails and names `nats.enabled`

### Requirement: A deleted source's signals are released by the manager
The manager SHALL release a source through a helper over the contract. The helper:

- lists the source's signals by the any-phase source index
- deletes each one under the revision it listed
- clears the source's cooldown window
- clears the source's receipt counters

Release is idempotent and raises no error for a source with none.

The manager SHALL call it when a `SignalSource` is observed gone. It SHALL also release the orphan a pass finds, so no cleanup depends on an owner reference being honoured. The `kube` backend MAY still set the owner reference.

#### Scenario: A released source leaves no state behind
- **WHEN** a source is released and no other source holds its name
- **THEN** its signals, its cooldown window and its receipt counters are gone

#### Scenario: Another source of the same name keeps the counters
- **WHEN** a source is released while another source holds the same name
- **THEN** the window and the counters are left alone

### Requirement: Deleting a conversation releases what it owns
Deleting a conversation, by any path, SHALL release the out-of-line body of every input still queued on it, before the record is let go. Under the `nats` backend it SHALL also remove the conversation's runs, and a periodic sweep SHALL remove the runs and the blobs a crash between the two steps left without an owner, after a grace long enough that a blob written ahead of the input naming it is not taken for an orphan.

#### Scenario: A queued input's blob goes with the conversation
- **WHEN** a conversation with a queued input held out of line is deleted
- **THEN** that blob is gone and a blob no queue names is untouched

### Requirement: Run history is bounded the same on every backend
A conversation SHALL keep its newest ten runs on every backend, the oldest falling off. Growing a run that has aged out SHALL report not-found.

### Requirement: A reader's stream survives silence
The conversation and signal streams SHALL write a keep-alive while they have nothing to send. A reader SHALL treat a stream silent past several keep-alives as dead and relist before it resumes.

