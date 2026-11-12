## Purpose
The contract every backend for accepted work meets, the conformance suite
that defines it, and the one chart selector that picks a backend.

A command bus carries accepted work to a consumer, records what became of it,
and retries what failed. It is independent of the state store.

## ADDED Requirements

### Requirement: The bus is independent of the state store
The command bus SHALL be its own contract with its own backends, its own
connection settings and its own credential. It SHALL add no collection to the
state store's closed inventory. Every state backend SHALL be able to run with
every bus backend.

The bus carries work. The conversation record in the state store stays the only
durable truth about what the work did.

#### Scenario: A kube-store install runs the bus
- **WHEN** an install selects the `kube` state backend and the `nats` bus backend
- **THEN** conversations stay Kubernetes objects and tasks flow through the bus

#### Scenario: The bus holds no conversation state
- **WHEN** the bus's whole record is lost
- **THEN** no conversation, input, run or signal is lost, and only task history and unfinished work are

### Requirement: A submit is durably accepted before it is acknowledged
A submit SHALL return success only after the bus has durably recorded the task.
A task whose submit returned success SHALL survive the restart of any manager
replica and the restart of the bus server.

A submit that cannot be recorded SHALL fail with `unavailable` and SHALL leave
no partial task.

#### Scenario: A restart after acknowledgement loses nothing
- **WHEN** a submit returns success and every manager replica then restarts before any consumer claims the task
- **THEN** the task is claimable after the restart and runs once to completion

#### Scenario: A failed submit leaves nothing behind
- **WHEN** the bus cannot record a task
- **THEN** the submit fails as `unavailable` and no task of that submit appears in the log

### Requirement: A submit is idempotent by command id
Every submit SHALL carry a command id. The command id is the deduplication
key. Every attempt of a task SHALL carry its own task uid, minted by the bus,
unique for the life of the log and never reused.

A submit whose command id the dedup rule suppresses SHALL succeed and SHALL
return the existing task. It SHALL create no second task.

#### Scenario: A repeated submit returns the same task
- **WHEN** a caller submits one command id twice while the first task is queued
- **THEN** the second submit succeeds, returns the first task's uid, and the queue holds one task

#### Scenario: Two attempts never share a task uid
- **WHEN** a task is retried twice
- **THEN** each attempt is recorded under its own task uid and all three share one command id

#### Scenario: Concurrent submits have one winner
- **WHEN** several replicas submit one command id at the same moment
- **THEN** exactly one task is created and every submit returns its uid

### Requirement: The dedup rule never records a failure as success
A command id SHALL be suppressed while its task is in any of these states:

| State of the latest task | Suppresses its id |
|---|---|
| queued | yes |
| retrying | yes |
| running | yes |
| dead, within its park cool-off | yes |
| succeeded, within the queue's dedup window | yes |
| succeeded, past the dedup window | no |
| dead, past its park cool-off | no |
| expired | no |

A failure SHALL NOT be recorded as a success. A dead task SHALL NOT suppress
its own recovery forever. After the park cool-off the id is released and the
same command id SHALL be resubmittable. The failed attempt's log entries
stays as history.

#### Scenario: A succeeded id is suppressed inside the window
- **WHEN** a command id succeeded one minute ago and the queue's dedup window is ten minutes
- **THEN** a resubmit is suppressed and returns the settled task

#### Scenario: A succeeded id is reusable after the window
- **WHEN** a command id succeeded and the dedup window has passed
- **THEN** a resubmit creates a new task

#### Scenario: A dead id is parked, then released
- **WHEN** a task exhausts its attempts and goes dead
- **THEN** a resubmit of its id inside the cool-off is suppressed, and the same resubmit after the cool-off creates a new task while the dead record remains

#### Scenario: A failure is not remembered as success
- **WHEN** every attempt of a task fails
- **THEN** no point in its life makes a resubmit return it as succeeded

### Requirement: Every queue is append-only
A queue SHALL be an append-only log. A message SHALL be appended once, when it
is submitted, and no entry SHALL ever be modified or removed by the bus.

Everything that later happens to a message, whether claimed, failed, retried,
completed or dead, SHALL be a further entry appended to the log. The state of
a message SHALL be derived from its own entries, the last one deciding.

The position of an entry SHALL be assigned by the log on append. A message's
identity, its command id, SHALL be derived before the append and SHALL carry no
order. A consumer's position SHALL be the command id of the last entry it
acknowledged.

Retention SHALL remove entries only by the log's own declared limits, never by
an operation of the bus on one entry.

#### Scenario: Nothing is rewritten
- **WHEN** a task is submitted, claimed, fails retryably, is claimed again and completes
- **THEN** the log holds the original message entry unchanged plus appended entries for each step, and no entry was written twice

#### Scenario: A submit is one write
- **WHEN** a consumer claims a task the instant it becomes visible
- **THEN** the submit still succeeds, since it made no write after the append

#### Scenario: A consumer resumes from its last acknowledged id
- **WHEN** a consumer reconnects naming the command id of the last entry it acknowledged
- **THEN** it receives every later entry that is not acknowledged, in log order

### Requirement: Every queue is FIFO and a retry keeps its place
A consumer SHALL claim the oldest eligible task of a queue first. A task is
eligible when it is queued or retrying with its backoff elapsed, and no
ordering rule below holds it back.

A retried task SHALL keep its original position in the queue. A retry never
queues behind tasks that were submitted after it.

#### Scenario: Oldest first
- **WHEN** three tasks of different keys are queued in order and one consumer claims three times
- **THEN** it receives them in submission order

#### Scenario: A retry keeps its position
- **WHEN** the oldest task fails once and is retrying while a newer task is queued
- **THEN** the retry is claimed before the newer task as soon as its backoff elapsed

### Requirement: A queue declares an ordering key and tasks sharing one are serialized
A queue SHALL declare how a task's ordering key is derived, or declare that it
has none.

- Tasks of one queue sharing a key SHALL be serialized.
- A task SHALL NOT be claimable until the previous task of its key has settled.
- Tasks of different keys SHALL be claimable in parallel.

A task has settled when it is succeeded, dead or expired.

The rule SHALL be enforced at claim time from the log. Partitioning is not
part of the contract and no behaviour SHALL depend on it.

#### Scenario: One key runs one task at a time
- **WHEN** four tasks share one key and four consumers claim at once
- **THEN** exactly one claim succeeds, and the next task of the key is claimable only after the previous settled

#### Scenario: Different keys run in parallel
- **WHEN** tasks of three different keys are queued and three consumers claim at once
- **THEN** all three claim succeed

#### Scenario: A dead task releases its key
- **WHEN** the running task of a key goes dead
- **THEN** the next task of that key becomes claimable

#### Scenario: An expired task releases its key
- **WHEN** a queued task of a key expires unclaimed
- **THEN** the next task of that key is not held back by it

### Requirement: A claim is a lease held in the log
A claim SHALL be an entry appended to the task's log carrying a claim token and
a lease-until instant, appended conditionally on the last entry the claimant
saw, so that of two simultaneous claims only one is appended. A claim made by
one replica SHALL be completable by another replica that presents the claim
token.

A completion carrying a token that is not the latest claim's token SHALL be
refused and SHALL append nothing.

A lease that lapses SHALL return the task to the queue for redelivery without any write: the task reads as retryable once its lease has passed. The
lapsed lease SHALL count as one attempt that failed with the code
`unavailable`, since no consumer reported one.

A lease SHALL NOT live in the memory of one process.

#### Scenario: A claim is completed on another replica
- **WHEN** replica A claims a task and replica B completes it with the claim token
- **THEN** the completion succeeds and the task is succeeded

#### Scenario: A stale token is refused
- **WHEN** a lease lapses, the task is claimed again, and the first claimant then completes it
- **THEN** the late completion is refused and the second claim stands

#### Scenario: A lapsed lease redelivers and counts
- **WHEN** a claimed task's lease passes with no completion
- **THEN** the task returns to the queue, its attempt count is one higher, the attempt is recorded as `unavailable`, and it is claimable again in its original position

### Requirement: Retry and backoff are owned by the bus
The bus SHALL retry a failed attempt. Attempts SHALL be bounded. The delay
between attempts SHALL grow exponentially with jitter. Each queue SHALL declare
the maximum attempts and the backoff, and a kind of command MAY override them.

A consumer SHALL NOT schedule a retry. It reports the attempt's outcome with a
code from the closed vocabulary and the bus decides.

An attempt whose code is not retryable SHALL send the task straight to dead
without a retry. After the last attempt a retryable failure SHALL also send it
to dead.

#### Scenario: Backoff grows and then the task is dead
- **WHEN** a task whose queue allows three attempts fails retryably each time
- **THEN** the gaps between attempts grow, the third failure makes the task dead, and a dead-letter record exists

#### Scenario: An invalid command is never retried
- **WHEN** a consumer reports `invalid_command`
- **THEN** the task is dead after that one attempt

#### Scenario: A retry is never scheduled by the consumer
- **WHEN** a consumer reports a retryable code
- **THEN** the next attempt time is the bus's, derived from the declared backoff

### Requirement: Failures use a closed code vocabulary
An attempt that does not succeed SHALL be reported with exactly one code. The
vocabulary is closed. Whether a code is retryable SHALL be derived from the
code alone and SHALL NOT be chosen by a consumer.

| Code | Retryable |
|---|---|
| `not_found` | no |
| `precondition_failed` | no |
| `forbidden` | no |
| `conflict` | yes |
| `unavailable` | yes |
| `invalid_command` | no |
| `internal` | yes |

A code outside the vocabulary SHALL be refused by the bus and recorded as
`internal`.

#### Scenario: Retryability follows the code
- **WHEN** a consumer reports `conflict` and then another task reports `not_found`
- **THEN** the first is retried and the second is dead

#### Scenario: An unknown code is not trusted
- **WHEN** a consumer reports a code the vocabulary does not name
- **THEN** the log records `internal` and the task follows the `internal` rule

### Requirement: A task is in one of six states
A task SHALL be in exactly one state. The states are:

| State | Meaning |
|---|---|
| `queued` | waiting for its first claim |
| `running` | claimed and inside its lease |
| `retrying` | failed an attempt and waiting out its backoff |
| `succeeded` | completed |
| `dead` | exhausted or non-retryable, its id parked |
| `expired` | its time to live passed before any claim |

`succeeded`, `dead` and `expired` are terminal.

#### Scenario: A task walks the states
- **WHEN** a task is submitted, claimed, fails retryably once, is claimed again and completes
- **THEN** the log shows queued, running, retrying, running and succeeded in that order

### Requirement: A kind may declare a time to live
A queue or a kind MAY declare a time to live for a task. A task still `queued`
or `retrying` past that instant SHALL become `expired` and SHALL NOT run. An
expired task SHALL NOT go dead and SHALL leave no dead-letter record.

A task already `running` SHALL NOT expire.

#### Scenario: A stale fire-and-forget task does not run
- **WHEN** a task with a one-minute time to live is still queued after one minute
- **THEN** it is `expired`, no consumer claims it, and no dead letter is recorded

#### Scenario: A running task is not expired
- **WHEN** a claimed task passes its time to live while running
- **THEN** it keeps running and settles by its own outcome

### Requirement: The log records every task and is bounded lossy telemetry
The bus SHALL keep, per task, the entries of its attempt chain. Together they SHALL carry:

- the command id and the task uid
- the invocation id
- `causedBy`
- the verb and the queue
- the ordering key
- the actor
- the enqueued instant
- the attempt number and the state
- the last error code and its message
- the lease, as claim token and lease-until

The log SHALL be bounded by a retention cap declared per queue, in age and
in size. Past either cap
the oldest settled entries SHALL be removed by the log's own limits. The log SHALL be declared
lossy telemetry. Its loss SHALL cost history and unfinished work, never
conversation correctness.

Entries of a task that is not settled SHALL NOT be removed by the retention cap.

#### Scenario: Old settled records are trimmed
- **WHEN** a queue's settled records exceed its retention cap
- **THEN** the oldest settled records are removed and no unsettled record is

#### Scenario: A record carries its whole identity
- **WHEN** a task is read from the log
- **THEN** it names its command id, task uid, invocation id, verb, queue, key, actor, state and attempt

### Requirement: The log feed is gap-free and resumable
The bus SHALL offer a change feed over the log's entries. A consumer SHALL be able
to resume from a point it kept.

After resuming it SHALL observe every change made after that point, including
changes made while it was not consuming. Duplicate deliveries are permitted.
A gap is not.

Each event SHALL carry the task's derived state as of an entry. Events of one
task SHALL arrive in revision order.

A consumer whose point has been trimmed past SHALL be told to resync and
SHALL NOT be served a feed with a silent gap.

#### Scenario: No gap across a disconnect
- **WHEN** a consumer loses its feed while tasks change, then resumes from its kept point
- **THEN** it observes every change made during the disconnect

#### Scenario: A trimmed point is reported
- **WHEN** a consumer resumes from a point older than the log retains
- **THEN** it is told to resync and is not given a feed that skips changes

### Requirement: Dead letters are listed read-only
A dead task SHALL leave a dead-letter record carrying its log entries, its
last code and its message. The bus SHALL offer a read-only listing and a
read-only feed of dead letters.

Retention SHALL be capped by age and by size, oldest removed first. The bus
SHALL offer no retry action and no dismiss action on a dead letter.

#### Scenario: A dead task is listed
- **WHEN** a task goes dead
- **THEN** the dead-letter listing carries it with its code and message

#### Scenario: Retention is bounded by age and size
- **WHEN** dead letters exceed the age cap or the size cap
- **THEN** the oldest are removed until both caps hold

#### Scenario: No action exists on a dead letter
- **WHEN** a caller looks for a way to retry or dismiss a dead letter
- **THEN** the bus offers none, and recovery is a new submit of the command

### Requirement: Per-queue statistics are readable
The bus SHALL report, per queue, the depth, the in-flight count and the age of
the oldest waiting task.

| Statistic | Counts |
|---|---|
| depth | tasks `queued` or `retrying` |
| in-flight | tasks `running` |
| oldest age | the age of the oldest task counted in depth, zero when none |

#### Scenario: Statistics follow the queue
- **WHEN** a queue holds two queued tasks, one retrying and one running
- **THEN** its depth is three, its in-flight count is one and its oldest age is that of the oldest of the three

### Requirement: The queue inventory is closed and each queue declares itself
The bus SHALL serve exactly the queues of the closed inventory. A submit to a
queue outside it SHALL be refused as `invalid_command`. Adding a queue SHALL be
a contract change with a conformance scenario.

| Queue | Ordering key |
|---|---|
| `conv` | the root conversation of the `causedBy` tree |
| `in.signal` | the source and the `initiationId` |
| `out.<channel>` | the conversation |

Each queue SHALL declare:

- its ordering key, or that it has none
- its maximum attempts and its backoff
- its time to live, where one applies
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

### Requirement: Delivery is at least once and handlers are idempotent
The bus SHALL deliver a task at least once. A handler SHALL be idempotent: a
second delivery of the same task SHALL leave the world as one delivery does.

#### Scenario: A redelivered task changes nothing twice
- **WHEN** a task's lease lapses after its handler already applied its effect and the task is delivered again
- **THEN** the second run observes the effect and completes without applying it again

### Requirement: Two backends, one contract
The bus SHALL ship two backends, `nats` and `memory`. Both SHALL pass every
scenario of the conformance suite. The `memory` backend is for tests and local
development only. Its tasks live in one process and do not survive its
restart. A production install SHALL select `nats`.

#### Scenario: The memory backend is not a production choice
- **WHEN** an install runs more than one manager replica with `memory`
- **THEN** the render is refused, because tasks claimed in one replica cannot be completed in another

### Requirement: Exactly one bus backend is selected, independently
The chart SHALL expose one selector, `commandBus.backend`, with the values
`nats` and `memory`. Its default is `nats`. It SHALL be independent of
`stateStore.backend`.

The render SHALL fail, naming the value, on a backend the chart does not know.

For `nats` an install SHALL by default use the state store's own `nats` server and credential, with nothing configured. It MAY instead render a dedicated server bundle, or be given the URL of a server in `commandBus.nats.url`. It SHALL NOT combine a dedicated bundle with a URL.

The manager SHALL learn its backend and its connection from the chart alone,
through variables of its own, and SHALL refuse to start on a backend it does
not implement.

#### Scenario: An unknown backend fails the render
- **WHEN** the selector names a backend the chart does not implement
- **THEN** the render fails and the message names the value

#### Scenario: A server must come from somewhere
- **WHEN** `nats` is selected with no bundle enabled and no server URL
- **THEN** the render fails naming both ways to supply a server

#### Scenario: The two selectors are independent
- **WHEN** `commandBus.backend` is changed
- **THEN** `stateStore.backend` and every state record are unchanged

### Requirement: The manager is not ready while the bus is unreachable
The manager SHALL report not ready while it cannot reach the bus. A replica
that cannot accept work durably SHALL NOT receive traffic that would need it.

Nothing except the manager SHALL talk to the bus server.

#### Scenario: A lost bus withdraws readiness
- **WHEN** a replica loses its connection to the bus server
- **THEN** its readiness fails until the connection is restored

#### Scenario: Components other than the manager never reach the bus
- **WHEN** an adapter, the console or a runtime pod tries to open a connection to the bus server
- **THEN** it is refused, and the credential is held by the manager alone

### Requirement: The conformance suite and the bench define the contract
There SHALL be one conformance suite, and every backend SHALL pass all of it
to be shipped. Every requirement in this capability SHALL be one or more
scenarios of the suite. A backend-specific scenario SHALL NOT exist.

The suite SHALL cover at minimum:

- ordering per key under parallel consumers
- idempotent submit
- dedup, park and release
- lease completion on a different replica
- lease lapse and redelivery
- dead-letter routing
- time-to-live expiry
- gap-free feed resume
- per-queue statistics

The suite SHALL be walked once per queue of the closed inventory. The bench
SHALL measure, per backend and per queue, the submit-to-complete latency, the
claims per second under parallel consumers and the bytes stored per task, and
SHALL report them on one table.

#### Scenario: A backend that fails one scenario is not shippable
- **WHEN** a backend passes every scenario but one
- **THEN** its build fails and the chart cannot select it

#### Scenario: Every queue is walked
- **WHEN** the suite runs
- **THEN** each scenario runs once for each queue of the closed inventory

#### Scenario: The bench reports every backend
- **WHEN** the bench runs
- **THEN** it reports the same measurements for `nats` and `memory`
