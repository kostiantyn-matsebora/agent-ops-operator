# contract-conformance-suite Specification

## Purpose
TBD - created by archiving change e2e-testing. Update Purpose after archive.

## Requirements

### Requirement: Adapter conformance is verified black-box, against the built binary
A conformance suite SHALL exist that verifies an adapter implementation
against the adapter contracts by running its **built binary** and speaking
HTTP to it, with a fake manager standing in for the real one.

- Lives in the manager's Go module (`platform/manager/`) — the one module
  with dependencies, so it adds none and is not discovered as a component.
- SHALL NOT be a package adapter modules import — every adapter module
  requires no dependency outside its own directory, and a shared test
  package would add one to all of them.
- Drives the binary rather than a library — the more faithful form, since
  it verifies the artifact that ships, matching that adapters are
  out-of-process by definition.

#### Scenario: No adapter module gains a dependency
- **WHEN** the conformance suite is added and every adapter is covered by it
- **THEN** each adapter module's dependency set is unchanged, and the eight-module boundary holds

#### Scenario: A new adapter joins by being listed
- **WHEN** a new adapter module is added to the repository
- **THEN** it is covered by adding it to the suite's list of implementations under test, with no change to its own source

### Requirement: The suite needs no cluster, no network and no credentials
The suite SHALL run with no Kubernetes cluster, no outbound network access
and no third-party credentials, so that it can gate every pull request
including those from forks.

Adapters whose implementation reaches a third-party system SHALL be pointed
at a local double through their configured endpoint.

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

### Requirement: Signal adapter conformance keys on the correlation id
A signal adapter under test SHALL demonstrate, against the fake manager:

1. **Normalized emission** — it posts signals carrying `initiationId`, `labels`,
   `kind` and payload, and it mints the id itself, from its own aggregation,
   leaving cooldown, routing and recurrence to the manager.
2. **Deterministic correlation** — re-observing the same condition, and
   re-sending after a failure, repeat the same `initiationId`.
3. **Authentication** — it presents its bearer token, and does not proceed
   silently when the manager rejects it.
4. **Source scoping** — the signals it emits name the source it serves.
5. **Failure handling** — a rejected or unavailable manager results in retry or
   a reported error, never in a silent drop with a healthy-looking process.
6. **Explicit refusal** — a push-mode (webhook-receiving) adapter answers a
   post for an unknown or unserved source with 404, the body naming the
   source.

A chat-originating signal adapter SHALL additionally demonstrate that every
signal it emits carries the channel label, since the manager refuses a chat
signal it could not answer.

A behaviour a conformance fix pins SHALL live here whenever the fake manager
can observe it — a refusal body, a repeated id, a clip reported through the
status API.

A behaviour only a real upstream can observe stays pinned by that adapter's
own unit tests.

#### Scenario: Correlation is minted where the events are understood
- **WHEN** an adapter observes several upstream events it considers one problem
- **THEN** the signals it emits share one `initiationId`, and the manager performs no grouping of its own

#### Scenario: A re-send repeats its id
- **WHEN** an adapter re-sends a signal after a failed or unacknowledged post
- **THEN** the re-sent signal carries the same `initiationId` as the original

#### Scenario: A rejected post is not silently dropped
- **WHEN** the fake manager rejects a signal post
- **THEN** the adapter retries or surfaces the failure, and the suite can observe that it did

#### Scenario: An unknown source's refusal names it
- **WHEN** the suite posts a webhook for a source the adapter under test does not serve
- **THEN** the response is 404 and its body names the source

#### Scenario: A chat signal always carries its channel
- **WHEN** a chat-originating adapter emits a signal
- **THEN** the channel label is present, so the manager can answer where the person typed

### Requirement: Every shipped signal adapter's binary is under test

The suite's list of signal implementations under test SHALL cover every
adapter module under `signals/`, `signals/pulse` included.

The suite SHALL assert that coverage itself, from the repository's own
directory listing, so a new adapter module fails the suite until it is
listed — the specification's every-shipped-adapter claim is checked, never
assumed.

#### Scenario: The pulse binary runs in the suite

- **WHEN** the conformance suite runs
- **THEN** `signals/pulse`'s built binary is exercised against the fake
  manager exactly as the other shipped adapters are

#### Scenario: An unlisted adapter module fails the suite

- **WHEN** a module exists under `signals/` that the suite's implementation
  list omits
- **THEN** the suite fails naming the module, rather than silently excluding
  it from the claim

### Requirement: A thread-less send is visible or an explicit error

The suite SHALL deliver a `send` operation carrying no `threadId` to each
channel adapter under test, and SHALL accept exactly two outcomes:

1. The message surfaces somewhere a person can see it, and the operation
   completes successfully.
2. The operation completes with an error naming why the adapter could not
   present it.

A success completion with NO externally observable effect SHALL fail the
implementation. A message that nobody can see, reported as delivered, is the
defect this assertion pins.

#### Scenario: A visible fallback passes

- **WHEN** an adapter presents the thread-less send on a general or fallback surface it exposes
- **THEN** the suite observes the presentation and the successful completion, and the implementation passes

#### Scenario: An invisible success fails

- **WHEN** an adapter completes the thread-less send successfully with no observable effect
- **THEN** the suite fails that implementation

### Requirement: Delete-conversation semantics are exercised

The suite SHALL deliver `delete-conversation` to each channel adapter under
test, in both reachable states:

| State | Expected completion |
|---|---|
| the operation names a thread the adapter holds | an observable transport effect, then completion |
| the adapter cannot act — an unknown thread, or none | an ERROR completion, never silent success |

The error branch pins the contract's rule that an adapter that cannot act
SHALL complete the operation with an error.

#### Scenario: A held thread is acted on

- **WHEN** the fake manager delivers `delete-conversation` for a thread the adapter created earlier in the run
- **THEN** the suite observes the adapter's transport effect and the operation's completion

#### Scenario: Cannot act completes with an error

- **WHEN** `delete-conversation` names a thread the adapter does not hold
- **THEN** the adapter completes the operation with an error, and a silent success fails the implementation

### Requirement: The retry budget is measured against the claim window

The suite SHALL inject retryable backpressure — a `429` carrying a stated
wait, and transient `5xx` responses — into one operation's transport calls,
through the local double the adapter is already pointed at.

It SHALL assert the adapter's total in-process time for that one operation
stays strictly below the claim window, completing as success or failure before
the window ends.

The window judged against SHALL be the `reclaimAfterSeconds` the fake manager
stated on the claim. An adapter that ignores the field SHALL be judged against
the contract's documented default reclaim interval instead.

#### Scenario: Absorbed backpressure completes inside the window

- **WHEN** injected `429` and `5xx` responses clear after a few retries
- **THEN** the operation completes successfully, and its total elapsed time is below the stated window

#### Scenario: Unending backpressure is abandoned in time

- **WHEN** the injected backpressure never clears
- **THEN** the adapter reports the operation failed before the window ends, so a second claimant never overlaps the first

### Requirement: Runtime conformance is verified black-box, against the built runtime binary
A conformance suite SHALL exist that verifies an `AgentRuntime` implementation
against `docs/contracts/runtime.md` by running its **built binary** and
speaking HTTP to it, with a fake manager standing in for the real one.

It SHALL live beside the adapter conformance set in the manager's Go module
(`platform/manager/test/conformance/`), reusing the same `FakeManager`.

`FakeManager` SHALL serve `GET /work` and `POST /work/done`, mirroring the
existing `/channel/ops` and `/channel/ops/{id}/done` pair.

The suite SHALL cover `runtimes/ollama`. It asserts these requirement IDs
from `docs/contracts/runtime.md` behaviorally: `API-1`, `API-2`,
`API-4`, `API-5`, `API-7`, `API-8`, `API-9`, `LC-1`, `LC-2`, `RT-1`,
`RT-2`, `RT-3` and its sub-items, `COMPAT-1`.

It SHALL NOT assert `API-3`, `API-6`, `LC-3`, `LC-4`, `LC-5`, `SEC-1`,
`SEC-2`, `SEC-3`, `COMPAT-2`, `COMPAT-3` or any `MGR-n` requirement
(`MGR-5` included), each already covered elsewhere:

| Excluded | Because | Covered by |
|---|---|---|
| `API-3`, `API-6`, `MGR-n` (`MGR-5` included) | obligates whoever answers `GET /work` — the manager, not the runtime | envtest, the system tier |
| `LC-4`, `LC-5` | a crash on missing configuration and the pod's own termination signal are process and kubelet facts, not `/work` behavior | the runtime's own unit tests, the system tier |
| `COMPAT-2` | obligates the manager reading a report | the manager's own unit tests, envtest |
| `COMPAT-3` | a documentation fact about stable section anchors, no runtime behavior | the docs checks |
| `LC-3` | `/exit` refused mid-run is manager-side chat-router logic, no HTTP call to the runtime pod | the manager's own chat-router unit tests |
| `SEC-1`, `SEC-2`, `SEC-3` | facts about the manager/operator's own pod-spec and RBAC rendering, not runtime-binary behavior | `internal/runtimepod`'s unit tests, the chart's RBAC templates |

#### Scenario: A new runtime joins by being listed
- **WHEN** a new `AgentRuntime` implementation is added to the repository
- **THEN** it is covered by adding it to the suite's list of implementations
  under test, with no change to its own source

#### Scenario: An allowlist regression is caught before merge
- **WHEN** a change to a covered runtime breaks `toolsMode` composition, a
  bound of `turns[]`/`toolCalls[]`, or the idle-TTL exit
- **THEN** the suite fails that implementation and the failure names the
  broken requirement ID

#### Scenario: The vendor call is a local double, not a real model
A covered runtime's vendor call SHALL be pointed at a local double through
its configured endpoint, never at a real inference provider.

- **WHEN** the suite runs a covered runtime
- **THEN** every vendor call that runtime makes reaches a fake standing in
  for it, scripted deterministically by the task text, so the suite needs no
  network access and no model credential

### Requirement: Runtime conformance needs no cluster, no network and no credentials
The runtime conformance set SHALL run with no Kubernetes cluster, no outbound
network access and no third-party credentials, exactly as the adapter
conformance set does, so it gates every pull request including those from
forks.

#### Scenario: The runtime suite gates a fork pull request
- **WHEN** a pull request originates from a fork with no access to
  repository secrets
- **THEN** the runtime conformance suite runs in full and its result gates
  the pull request
