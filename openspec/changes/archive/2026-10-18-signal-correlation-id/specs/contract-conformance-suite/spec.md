# contract-conformance-suite — delta

## REMOVED Requirements

### Requirement: The signal adapter conformance set

**Reason**: The set verified `fingerprint` emission and the adapter's abstention from grouping. Both are retired — the adapter now OWNS the grouping decision by minting the id.

**Migration**: Replaced by "Signal adapter conformance keys on the correlation id" in this change, which keeps the authentication, scoping and failure-handling items unchanged.

## ADDED Requirements

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

A chat-originating signal adapter SHALL additionally demonstrate that every
signal it emits carries the channel label, since the manager refuses a chat
signal it could not answer.

#### Scenario: Correlation is minted where the events are understood
- **WHEN** an adapter observes several upstream events it considers one problem
- **THEN** the signals it emits share one `initiationId`, and the manager performs no grouping of its own

#### Scenario: A re-send repeats its id
- **WHEN** an adapter re-sends a signal after a failed or unacknowledged post
- **THEN** the re-sent signal carries the same `initiationId` as the original

#### Scenario: A rejected post is not silently dropped
- **WHEN** the fake manager rejects a signal post
- **THEN** the adapter retries or surfaces the failure, and the suite can observe that it did

#### Scenario: A chat signal always carries its channel
- **WHEN** a chat-originating adapter emits a signal
- **THEN** the channel label is present, so the manager can answer where the person typed
