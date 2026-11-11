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

## ADDED Requirements

### Requirement: A deleted source's signals are released by the manager
The manager SHALL release a source through a helper over the contract. The helper:

- lists the source's signals by the any-phase source index
- deletes each one not held by the source currently holding the name, under the revision it listed
- clears the source's cooldown window, only when no source holds the name
- clears the source's receipt counters, only when no source holds the name

Release is idempotent and raises no error for a source with none.

The manager SHALL call it when a `SignalSource` is observed gone. It SHALL also release the orphan a pass finds, so no cleanup depends on an owner reference being honoured. The `kube` backend MAY still set the owner reference.

Release is a helper, not an operation of the contract. The any-phase source index exists for it.

A name can be reused by a successor source, so the release SHALL decide from the source that holds the name at the moment of each delete.

After listing the signals, the release SHALL read the source by that name from the authoritative store of configuration, never from a cache. Under the `kube` backend that is the Kubernetes API server.

A signal recorded under the identity of the source that holds the name now SHALL be kept. Every other signal of the name SHALL be deleted under the revision it was listed at.

A conflict SHALL re-read that signal and decide again, with exponential back-off and at most 20 attempts. When the attempts are exhausted the release SHALL return the conflict as an error, leave that signal in place, and let the next pass retry it.

The cooldown window and the receipt counters SHALL be cleared only when no source holds the name. The caller supplies no identity, so a successor created at any point during the release keeps its signals.

#### Scenario: A released source leaves no state behind
- **WHEN** a source is released and no other source holds its name
- **THEN** its signals, its cooldown window and its receipt counters are gone

#### Scenario: Another source of the same name keeps the counters
- **WHEN** a source is released while another source holds the same name
- **THEN** the window and the counters are left alone

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

### Requirement: Deleting a conversation releases what it owns
Deleting a conversation, by any path, SHALL release the body of every input still queued on it.

The release SHALL happen before the record is let go.

Under the `nats` backend it SHALL also remove the conversation's runs.

Under the `nats` backend a periodic sweep SHALL remove the runs and blobs a crash left without an owner.

The sweep SHALL wait a grace of 10 minutes first. A blob written ahead of the input naming it is not an orphan.

#### Scenario: A queued input's blob goes with the conversation
- **WHEN** a conversation with a queued input held out of line is deleted
- **THEN** that blob is gone and a blob no queue names is untouched

#### Scenario: Runs go with the conversation and the sweep spares a fresh blob
- **WHEN** a conversation with runs is deleted under `nats`, and a blob younger than the grace names no input
- **THEN** the runs are gone, the sweep removes orphaned runs and old unowned blobs, and the young blob stays

### Requirement: Run history is bounded the same on every backend
A conversation SHALL keep its newest ten runs on every backend, the oldest falling off. Growing a run that has aged out SHALL report not-found.

#### Scenario: The eleventh run drops the oldest
- **WHEN** an eleventh run is appended to a conversation
- **THEN** ten runs remain, the oldest is gone, and growing it reports not-found

### Requirement: A reader's stream survives silence
The conversation and signal streams SHALL write a keep-alive while they have nothing to send. A reader SHALL treat a stream silent past several keep-alives as dead and relist before it resumes.

#### Scenario: An idle stream emits keep-alives
- **WHEN** a stream has nothing to send for a keep-alive interval
- **THEN** it writes a keep-alive

#### Scenario: A silent stream is relisted
- **WHEN** a reader sees no bytes past several keep-alive intervals
- **THEN** it gives the stream up, relists, and then resumes

