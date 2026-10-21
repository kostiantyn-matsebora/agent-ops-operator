# channel-adapter-contract — delta

## MODIFIED Requirements

### Requirement: Adapters absorb retryable backpressure inside the claim window
An adapter SHALL distinguish **retryable** transport conditions — rate limits,
timeouts, and transient server errors — from **terminal** ones. It SHALL retry
retryable conditions in-process before reporting the operation.

Only a terminal failure, or exhaustion of the adapter's retry budget, SHALL be
reported as an error to `POST /channel/ops/{id}/done`.

When the transport states how long to wait, the adapter SHALL honor that value
rather than applying its own backoff. The adapter's total in-process retry
budget for one operation SHALL remain strictly below the manager's claim
reclaim interval.

The budget is PER OPERATION, never per transport call. An operation that makes
several calls — a reopen plus a create, one send per chunk, a fallback — SHALL
charge every call's wait, retry and own timeout against the one budget.

An adapter that exceeds the reclaim interval converts the manager's
at-least-once redelivery into a duplicate the transport cannot deduplicate,
because the message has already been posted.

Reporting a retryable condition as an operation failure is permitted but
degrades the system to the manager's recovery path for a condition the adapter
could have ridden out. Adapters SHALL NOT treat it as the normal path.

#### Scenario: Rate limit is slept out rather than reported
- **WHEN** the transport rejects a `send` with a retryable rate-limit error and a stated wait
- **THEN** the adapter waits the stated interval, retries the same operation, and reports success without the manager observing a failure

#### Scenario: Retry budget stays under the reclaim interval
- **WHEN** an operation's retries would exceed the manager's claim reclaim interval
- **THEN** the adapter abandons the retry, reports the operation as failed, and the manager re-derives it rather than two claimants posting the same message

#### Scenario: A multi-call operation shares one budget
- **WHEN** an operation requires several transport calls and each is rate-limited in turn
- **THEN** the summed waits and timeouts stay below the reclaim interval, or the adapter reports failure while the claim is still valid

#### Scenario: Terminal error is reported immediately
- **WHEN** the transport rejects an operation for a reason retrying cannot fix, such as a missing thread or a rejected credential
- **THEN** the adapter reports the failure without retrying and the manager applies its own recovery rules for that operation kind

## ADDED Requirements

### Requirement: Delivery operation ids are published shapes

The manager SHALL mint stable, published ids for the two `send` operations
that deliver conversation content:

| Delivery | Id shape |
|---|---|
| an agent's reply, per run | `send:<conversation>:<channel>:<runId>` |
| a person's input, fanned out | `input:<conversation>:<input>:<channel>` |

Each SHALL be stable across redelivery and manager restarts, because the id is
what makes at-least-once delivery deduplicable.

A consumer MAY parse a published shape. In each, the conversation and channel
segments are Kubernetes names, which cannot contain the separator — so the one
remaining segment is recovered by position, whatever it contains.

Any id matching NO published shape SHALL stay opaque. An adapter SHALL NOT
infer structure the contract does not publish, so an unpublished id can change
without breaking anyone.

#### Scenario: An input delivery carries the published id

- **WHEN** one input is fanned out to a bound channel and the operation is later redelivered
- **THEN** both deliveries carry `input:<conversation>:<input>:<channel>`, the same id both times

#### Scenario: A consumer correlates by the published shape

- **WHEN** an adapter correlates a delivered `send` with the input or run it carries
- **THEN** the published shape yields the conversation, the channel and the remaining segment, anchored on the name segments that cannot contain a separator

#### Scenario: An unpublished id stays opaque

- **WHEN** an operation arrives whose id matches no published shape
- **THEN** the adapter treats the id as opaque, completes the operation by that id verbatim, and infers nothing from its content
