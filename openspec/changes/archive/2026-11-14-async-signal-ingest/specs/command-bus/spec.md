## MODIFIED Requirements

### Requirement: The queue inventory is closed and each queue declares itself
The bus SHALL serve exactly the queues of the closed inventory. A submit to a
queue outside it SHALL be refused as `invalid_command`. Adding a queue SHALL be
a contract change with a conformance scenario.

| Queue | Ordering key |
|---|---|
| `conv` | the root conversation of the `causedBy` tree |
| `out.<channel>` | the conversation |
| `in.signal` | the source and the signal's `initiationId` |

The `out.<channel>` queues SHALL declare four kinds, each a complete declaration of its own and each a hand-over queue: `stable` (replies and inputs, delivery TTL 10 minutes), `ephemeral` (acks and notices, 2 minutes), `topic` (`ensure-topic` and `close-topic`, 5 minutes) and `delete-conversation` (5 minutes).

A delivery TTL SHALL be declared per kind, never per adapter.

`in.signal` SHALL be a lease-and-retry queue, not a hand-over queue: 5 maximum attempts, backoff from 1 second to 30 seconds with jitter, no time to live.

Each queue SHALL declare:

- its ordering key, or that it has none
- its maximum attempts and its backoff, where it is a lease-and-retry queue
- its time to live, where one applies, and its delivery TTL, where it is a hand-over queue
- its log retention
- its dedup window and its park cool-off
- the most tasks of one invocation it allows in flight at once

#### Scenario: An unknown queue is refused
- **WHEN** a submit names a queue the inventory does not declare
- **THEN** it is refused as `invalid_command` and nothing is recorded

#### Scenario: A queue per channel
- **WHEN** a channel is wired
- **THEN** its operations flow through its own `out.<channel>` queue, keyed by conversation

#### Scenario: One channel's backlog does not delay another
- **WHEN** two channels of one adapter type are bound and one is unavailable
- **THEN** the other's operations are claimed and delivered without waiting

#### Scenario: in.signal retries before going dead
- **WHEN** an `in.signal` task fails retryably four times
- **THEN** its fifth attempt is its last, and a further retryable failure makes it dead
