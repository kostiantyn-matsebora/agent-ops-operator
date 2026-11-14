## ADDED Requirements

### Requirement: Signal ingest is one command on the `in.signal` queue

`POST /signal/inbound` SHALL enqueue each admitted signal as one `ingest-signal` task on the `in.signal` queue. The ordering key SHALL be the source name and the signal's `initiationId`.

- Two signals of one source sharing an `initiationId` SHALL run one after the other, in the order they were accepted.
- Signals with different keys MAY run in parallel.
- A batch of N admitted signals SHALL yield N tasks, even when several share an `initiationId`.

The request carries the signal to the task unchanged, `reader` included. The task SHALL NOT depend on the request that created it.

#### Scenario: One task per admitted signal
- **WHEN** an adapter posts a batch of three signals, two of them with the same `initiationId`
- **THEN** three `ingest-signal` tasks are recorded on `in.signal`, and the two sharing a key run in acceptance order

#### Scenario: Different sources never share a key
- **WHEN** two sources each post a signal with the same `initiationId`
- **THEN** the two tasks carry different ordering keys and may run in parallel

### Requirement: The accept step decides admission and enqueues

Before replying, `POST /signal/inbound` SHALL, in this order:

1. authenticate the caller and check its source scope
2. validate the body shape and every signal (`initiationId`, `severity`, `signalId`)
3. refuse an unknown source with `404`
4. record the "signal received" activity receipt for every signal
5. apply the source's `attributeFilters`
6. durably enqueue one task per admitted signal

The accept step SHALL NOT answer a chat command, create a `Signal` or make any routing decision.

- A refusal at steps 1 to 3 SHALL enqueue nothing and name its reason.
- An unwired source is not a refusal. Its signals are accepted, the consumer stores them unrouted, and routing records the drop.
- If enqueueing fails for any admitted signal, the reply SHALL be `5xx` with an `error`. The adapter's ordinary re-send repeats the batch.

#### Scenario: A refusal enqueues nothing
- **WHEN** a request names an unknown source, carries a bad `severity` or fails authentication
- **THEN** the manager refuses it with its status and reason, and no task is recorded

#### Scenario: The receipt is recorded at accept
- **WHEN** an adapter posts two signals
- **THEN** two "signal received" receipts exist when the reply is written, before either task has run

#### Scenario: An unwired source is accepted
- **WHEN** an adapter posts a signal for a source no Ready Pipeline claims
- **THEN** the reply is `202` and a task is recorded, and the consumer stores a `Signal` with no `status.phase`, and routing later records `Dropped` with reason `unclaimed`

#### Scenario: An enqueue failure is not acknowledged
- **WHEN** the manager cannot durably enqueue a signal of the batch
- **THEN** the reply is `5xx` with an `error`, and a re-send of the batch is accepted

### Requirement: The reply names the invocation and its tasks

A successful `POST /signal/inbound` SHALL reply `202` with the invocation id and one task id per admitted signal, in the order the signals were posted. It SHALL keep `queued` and `reason`.

```json
{
  "invocationId": "inv-4c2a",
  "taskIds": ["tsk-91e0", "tsk-91e1"],
  "queued": 2
}
```

When the source's `attributeFilters` refused part of the batch, `reason` is present and `taskIds` lists only the admitted signals:

```json
{
  "invocationId": "inv-7b10",
  "taskIds": ["tsk-91e2"],
  "queued": 1,
  "reason": "filtered"
}
```

- `queued` counts the signals accepted, which equals the length of `taskIds`.
- `queued` SHALL NOT claim a `Signal` exists.
- A batch where every signal is filtered SHALL reply `202` with `taskIds: []`, `queued: 0` and `reason`, and no invocation task.
- `conversations` SHALL NOT be returned.

#### Scenario: The reply carries ids and the legacy fields
- **WHEN** an adapter posts two admitted signals
- **THEN** the reply is `202` with an `invocationId`, two `taskIds` and `queued: 2`

#### Scenario: A filtered signal keeps its reason
- **WHEN** one of two posted signals fails the source's `attributeFilters`
- **THEN** the reply is `202` with `queued: 1`, one task id and `reason: filtered`

#### Scenario: A fully filtered batch enqueues nothing
- **WHEN** every posted signal fails the source's `attributeFilters`
- **THEN** the reply is `202` with `queued: 0`, an empty `taskIds` and `reason: filtered`

### Requirement: The consumer answers an in-place chat command

The `in.signal` consumer SHALL answer a chat signal that is an in-place command (the listing, a bare `/<pipeline>` asking for its task, `/close` or `/exit` on a general surface) through the router. It SHALL create no `Signal` for it.

- The reply SHALL go to the surface the signal's `reader` and chat labels name, exactly as a synchronous answer did.
- A chat signal naming an unknown channel SHALL succeed with nothing stored and nothing sent, as before.
- A failure to answer is a task failure, so the bus retries it.

#### Scenario: A listing command is answered by the consumer
- **WHEN** a chat signal whose payload is the listing command is accepted
- **THEN** the reply to the adapter is `202`, the consumer answers the surface, and no `Signal` exists for it

#### Scenario: An unknown channel stores nothing
- **WHEN** an accepted in-place command names a channel that does not exist
- **THEN** its task succeeds, no `Signal` is created and nothing is sent

#### Scenario: A failed answer is retried
- **WHEN** the router fails to answer an in-place command
- **THEN** the task fails and is retried by the bus, and the surface receives one answer once it succeeds

### Requirement: The consumer creates the Signal

For every other signal, the consumer SHALL create exactly one `Signal` as `signal-persistence` defines it, owned by its `SignalSource`. It SHALL look up no claimant and make no routing decision.

- A signal carrying a `signalId` that already names a `Signal` SHALL be deduplicated: nothing is created or changed, a duplicate activity event is recorded and the task succeeds.
- A redelivered task SHALL NOT create a second `Signal`, whether its first attempt failed before or after the create.
- A signal without a `signalId` that is redelivered after its create succeeded SHALL be recognised by the task, never stored twice.

#### Scenario: An accepted signal becomes a Signal
- **WHEN** an `ingest-signal` task for an admitted machine signal runs
- **THEN** one `Signal` owned by the source exists, with `spec` equal to the posted signal

#### Scenario: A redelivered task creates no second Signal
- **WHEN** a task's attempt creates the `Signal` and then fails to report success, and the bus redelivers it
- **THEN** the redelivery succeeds and exactly one `Signal` exists

#### Scenario: A landed signalId is deduplicated
- **WHEN** a task carries a `signalId` whose `Signal` already exists
- **THEN** the task succeeds, no second `Signal` is created and a duplicate activity event is recorded

### Requirement: Failure is retried, bounded and recorded

The `ingest-signal` task SHALL follow the retry policy the bus inventory declares for `in.signal`: 5 attempts, backoff from 1 second to 30 seconds with jitter, no TTL.

- A task that exhausts its attempts SHALL be dead, and its record SHALL stay visible in the task stream with its last error.
- A dead task SHALL NOT create a `Signal` afterwards by itself.
- A failed or dead task SHALL NOT change the reply the adapter already received.
- A non-retryable failure goes straight to dead.

#### Scenario: A transient failure is retried
- **WHEN** the consumer's create of the `Signal` fails twice and then succeeds
- **THEN** the task succeeds on its third attempt and exactly one `Signal` exists

#### Scenario: Exhausted attempts are dead and visible
- **WHEN** a task fails all 5 attempts
- **THEN** the task is dead, its last error is in the task stream and no `Signal` exists for it

#### Scenario: The task is visible
- **WHEN** a reader lists tasks after a signal was accepted
- **THEN** its `ingest-signal` task shows with its queue `in.signal`, its key and its state

### Requirement: Adapters need no change

A signal adapter that predates this behaviour SHALL keep working with the same request body.

- The request body SHALL be unchanged.
- An adapter that treats any `2xx` as accepted SHALL keep working.
- An adapter MAY ignore `invocationId` and `taskIds`.
- At-least-once re-send after a `5xx` and the `initiationId` and `signalId` rules SHALL be unchanged.

#### Scenario: A pre-existing adapter posts as before
- **WHEN** a shipped signal adapter posts a batch with its existing body
- **THEN** the reply is a `2xx` and the adapter treats the batch as delivered
