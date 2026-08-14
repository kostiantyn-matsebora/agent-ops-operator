## ADDED Requirements

### Requirement: An adapter MAY report a thread read
The channel adapter contract SHALL offer `POST /channel/read`, by which an
adapter reports how far one or more of its threads have been seen. The request
SHALL name the channel and carry a list of `{threadId, readAt}` entries; the
manager SHALL resolve each thread to its conversation and write the watermark to
that conversation's status.

The verb SHALL be OPTIONAL. An adapter that never calls it SHALL remain fully
conformant, and its threads SHALL simply carry no watermark.

The route SHALL be guarded by the same adapter authentication and channel scope
as every other `/channel/*` route: an adapter SHALL be able to report reads only
for channels it serves.

#### Scenario: An adapter reports a thread read
- **WHEN** an adapter posts a read for a thread on a channel it serves
- **THEN** the manager writes the watermark to that thread's binding and answers with the per-thread outcome

#### Scenario: An adapter cannot report for another adapter's channel
- **WHEN** an adapter scoped to one adapter name posts a read for a channel served by another
- **THEN** the request is refused with 403 and nothing is written

#### Scenario: An unauthenticated report is refused
- **WHEN** a read report arrives without a valid adapter token
- **THEN** it is refused with 401

#### Scenario: An adapter that never reports stays conformant
- **WHEN** an adapter implements every other contract operation and never calls this one
- **THEN** it serves its channels normally and its threads carry no watermark

### Requirement: A read report is a bounded batch with per-thread outcomes
A read report SHALL carry at most 50 entries, bounded by the manager and not only
by the caller. The response SHALL carry one outcome per requested thread —
`marked`, `skipped` or `failed` — with a reason for anything not marked, plus
totals. A batch in which some threads were not marked SHALL still be a successful
request, and a failure on one entry SHALL NOT stop the rest.

#### Scenario: An oversized batch is refused
- **WHEN** a read report carries more than 50 entries
- **THEN** the request is rejected with 400 and nothing is written

#### Scenario: A mixed batch succeeds with per-thread detail
- **WHEN** a batch marks some threads, skips others whose watermark would not advance, and fails on one whose conversation has been deleted
- **THEN** the response is 200 and names each thread with its own outcome and reason, plus the totals

#### Scenario: An unknown thread does not fail the batch
- **WHEN** a batch names a thread no conversation holds
- **THEN** that entry is reported failed with a reason and the remaining entries are still marked
