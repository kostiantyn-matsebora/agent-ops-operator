## ADDED Requirements

### Requirement: Adapter conformance is verified black-box, against the built binary
A conformance suite SHALL exist that verifies an adapter implementation against
the adapter contracts by running its **built binary** and speaking HTTP to it,
with a fake manager standing in for the real one. The suite SHALL live in the
root Go module.

It SHALL NOT be a package that adapter modules import. Every adapter module is
required to have no dependency outside its own directory, and a shared test
package would put one into all of them. Driving the binary is also the more
faithful form: it verifies the artifact that ships rather than a library the
artifact happens to contain, which matches the fact that adapters are
out-of-process by definition.

#### Scenario: No adapter module gains a dependency
- **WHEN** the conformance suite is added and every adapter is covered by it
- **THEN** each adapter module's dependency set is unchanged, and the eight-module boundary holds

#### Scenario: A new adapter joins by being listed
- **WHEN** a new adapter module is added to the repository
- **THEN** it is covered by adding it to the suite's list of implementations under test, with no change to its own source

### Requirement: The suite needs no cluster, no network and no credentials
The suite SHALL run with no Kubernetes cluster, no outbound network access and
no third-party credentials, so that it can gate every pull request including
those from forks. Adapters whose implementation reaches a third-party system
SHALL be pointed at a local double through their configured endpoint.

#### Scenario: The suite gates a fork pull request
- **WHEN** a pull request originates from a fork with no access to repository secrets
- **THEN** the conformance suite runs in full and its result gates the pull request

### Requirement: The channel adapter conformance set
A channel adapter under test SHALL demonstrate, against the fake manager:

1. **Long-poll** — it requests operations with a wait, and handles both an empty
   return and a returned operation.
2. **Contract declaration** — it sends `contract=` on the operations request, so
   an implementation reading a retired message field is rejected rather than
   posting empty messages and looking healthy.
3. **Typed message handling** — it renders each message type the contract
   defines from the typed payload, and never requires a pre-rendered text field.
4. **Acknowledgement** — it marks an operation done, and marks it done exactly
   once for a given operation id even when the same operation is delivered
   twice, because operations are at-least-once.
5. **Inbound push** — it posts inbound messages with a `threadId`.
6. **Channel listing and status** — it lists the channels it serves for its
   adapter name and reports configuration validity as a status update rather
   than by crashing.
7. **No relay loop** — an outbound post it makes is never submitted back as
   inbound.

#### Scenario: A duplicate operation is acknowledged once and acted on once
- **WHEN** the fake manager delivers the same operation id twice
- **THEN** the adapter's externally visible effect occurs once and the operation is acknowledged, rather than the effect being duplicated

#### Scenario: An adapter that omits the contract declaration is rejected
- **WHEN** an implementation requests operations without declaring `contract=`
- **THEN** the fake manager refuses it and the suite fails that implementation

#### Scenario: Invalid configuration is reported, not fatal
- **WHEN** a served channel carries configuration the adapter cannot accept
- **THEN** the adapter reports the error as a status update and continues serving its other channels

### Requirement: The signal adapter conformance set
A signal adapter under test SHALL demonstrate, against the fake manager:

1. **Normalized emission** — it posts signals carrying `fingerprint`, `labels`,
   `kind` and payload, and does not perform grouping, cooldown or recurrence,
   which are manager-side concerns.
2. **Authentication** — it presents its bearer token, and does not proceed
   silently when the manager rejects it.
3. **Source scoping** — the signals it emits name the source it serves.
4. **Failure handling** — a rejected or unavailable manager results in retry or
   a reported error, never in a silent drop with a healthy-looking process.

A chat-originating signal adapter SHALL additionally demonstrate that every
signal it emits carries the channel label, since the manager refuses a chat
signal it could not answer.

#### Scenario: Grouping is not performed adapter-side
- **WHEN** an adapter observes several related upstream events
- **THEN** it emits a signal per event with its own fingerprint, leaving grouping to the manager

#### Scenario: A rejected post is not silently dropped
- **WHEN** the fake manager rejects a signal post
- **THEN** the adapter retries or surfaces the failure, and the suite can observe that it did

#### Scenario: A chat signal always carries its channel
- **WHEN** a chat-originating adapter emits a signal
- **THEN** the channel label is present, so the manager can answer where the person typed
