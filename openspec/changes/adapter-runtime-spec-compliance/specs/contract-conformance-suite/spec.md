# contract-conformance-suite — delta

## ADDED Requirements

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
