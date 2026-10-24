# contract-conformance-suite delta

## MODIFIED Requirements

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

## ADDED Requirements

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
