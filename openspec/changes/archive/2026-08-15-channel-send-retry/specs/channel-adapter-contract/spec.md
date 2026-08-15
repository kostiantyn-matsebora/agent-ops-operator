## ADDED Requirements

### Requirement: Adapters absorb retryable backpressure inside the claim window
An adapter SHALL distinguish **retryable** transport conditions — rate limits,
timeouts, and transient server errors — from **terminal** ones, and SHALL retry
retryable conditions in-process before reporting the operation. Only a terminal
failure, or exhaustion of the adapter's retry budget, SHALL be reported as an
error to `POST /channel/ops/{id}/done`.

When the transport states how long to wait, the adapter SHALL honor that value
rather than applying its own backoff. The adapter's total in-process retry
budget for one operation SHALL remain strictly below the manager's claim
reclaim interval, so that an operation still being worked is never re-issued to
another claimant. An adapter that exceeds the reclaim interval converts the
manager's at-least-once redelivery into a duplicate the transport cannot
deduplicate, because the message has already been posted.

Reporting a retryable condition as an operation failure is permitted but
degrades the system to the manager's recovery path for a condition the adapter
could have ridden out; adapters SHALL NOT treat it as the normal path.

#### Scenario: Rate limit is slept out rather than reported
- **WHEN** the transport rejects a `send` with a retryable rate-limit error and a stated wait
- **THEN** the adapter waits the stated interval, retries the same operation, and reports success without the manager observing a failure

#### Scenario: Retry budget stays under the reclaim interval
- **WHEN** an operation's retries would exceed the manager's claim reclaim interval
- **THEN** the adapter abandons the retry, reports the operation as failed, and the manager re-derives it rather than two claimants posting the same message

#### Scenario: Terminal error is reported immediately
- **WHEN** the transport rejects an operation for a reason retrying cannot fix, such as a missing thread or a rejected credential
- **THEN** the adapter reports the failure without retrying and the manager applies its own recovery rules for that operation kind

### Requirement: Outbound pacing belongs to the adapter
An adapter SHALL pace its own outbound calls to its transport's documented
limits. The manager SHALL NOT model per-transport rate limits, expose a pacing
setting, or delay operation hand-out on a transport's behalf — a transport's
budget is transport knowledge, on the same footing as message length caps,
escaping, and thread naming, all of which the contract already places in the
adapter.

Pacing SHALL be applied to every call an operation makes, including thread
creation, so that a burst of new conversations is spread rather than rejected.

#### Scenario: Burst is spread rather than rejected
- **WHEN** the manager hands the adapter more operations in a short interval than the transport's budget allows
- **THEN** the adapter spreads the calls within budget and every operation completes, rather than a subset being rejected

#### Scenario: Manager declares no pacing
- **WHEN** an adapter for a transport with different limits is added
- **THEN** no manager-side change is required, because the manager holds no rate-limit knowledge for any transport
