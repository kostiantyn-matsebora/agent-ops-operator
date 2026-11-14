## Context

See `proposal.md` for why. Today `handleSignalInbound` does everything in the request:

1. validate, look up the source, record the receipt, partition by `attributeFilters`
2. `answerCommands` (in-place chat commands, through `Router.HandleCommand`)
3. `publish`, one `Signal` create per signal (`signal-persistence` D-2)
4. reply `200 {"queued": n}`

The `in.signal` queue is declared in the bus inventory: 5 attempts, backoff 1s to 30s with jitter, no TTL, 1 minute dedup window, 5 minute park cool-off. It has no producer and no consumer.

## Goals / Non-Goals

**Goals:**
- Move steps 2 and 3 out of the request, onto `in.signal`.
- Keep the adapter-visible contract additive, apart from the status.

**Non-Goals:**
- No routing change. The `Signal` consumer (claim, cooldown, capacity) is untouched.
- No change to the retry values already declared for `in.signal`.
- No adapter change.

## Decisions

### D-1: What truly changes against `signal-persistence`

`signal-persistence` already makes publication durable: the CR exists before the `200`. This change does NOT add durability. It trades one durable record (the CR) for another (the task), and the gain is narrower than the proposal's framing:

| Gain | Detail |
|---|---|
| Bounded retry | the manager retries, not only the adapter's re-send |
| Dead-letter | a signal that cannot be stored is a dead task with an error, not a lost `5xx` |
| Task visibility | `/state/tasks` shows the ingest path |
| Thinner request | `answerCommands` and the CR write leave the request |

The cost is a window where a signal is acknowledged and no `Signal` exists yet. The durable task covers it.

A signal whose task dies is visible but not stored, and nothing re-sends it, because the adapter was told `202`. Dead-letter replay is outside this change (see Open Questions).

### D-2: One task per signal, not per batch

One task per admitted signal. The key is source plus `initiationId`, and a batch may carry several keys.

- A per-batch task would need one key, so it would serialize unrelated `initiationId`s and let one poison signal fail its siblings.
- Per-signal keeps today's semantics: N signals, N `Signal`s, per-key order.
- Order across keys in one batch is no longer guaranteed. Today it is the batch order. The consumer never relied on it, because it arranges by `creationTimestamp` then name.
- `taskIds` follows the posted order of the admitted signals. `invocationId` names the request.

### D-3: A partly refused batch

Filtering stays at accept, before enqueue.

- A filtered signal enqueues nothing and the reply keeps `reason: filtered`.
- `queued` equals `len(taskIds)`.
- A fully filtered batch replies `202` with empty `taskIds`. The decision is recorded in Open Question 2.
- The `signalReceived` receipt is still emitted for every posted signal, filtered ones included, as today.
- An unwired source is not refused at accept. `Wired=False` is a routing outcome the consumer records on the `Signal`. The proposal no longer lists a refusal.

### D-4: `200` to `202`

The adapters under `signals/` all go through a `manager.go` client. Each one treats `>= 400` as failure and decodes the body only when it is not `204`:

- `signals/cron/manager.go`
- `signals/alertmanager/manager.go`
- `signals/ha/manager.go`
- `signals/k8s-events/manager.go`
- `signals/pulse/manager.go`
- `signals/telegram/manager.go`

None checks for exactly `200`. `202` is accepted unchanged, and decoding the new body into their existing reply struct ignores the added fields. The grep for `resp.StatusCode` found no `== 200` or `!= 200` outside tests. The `StatusCode` returns in `alertmanager/register.go` and `pulse/register.go` belong to webhook registration, not inbound.

The risk is external adapters. The contract says "any `2xx`" from now on, and the spec version moves to `2.4.0` (MINOR). The changelog names the status move. The tests under each adapter that stub a `200` need no change.

### D-5: How `answerCommands` replies from a consumer

`Router.HandleCommand` takes `sig.Reader`, the opaque salted key of the sender, and the chat labels. Both are already fields of the signal, so the task carries them unchanged.

- The task payload IS the normalized signal plus the source name. The consumer reads nothing from the request.
- The reply goes through the router, which delivers to the surface by `Channel`. Nothing in the answer needs the HTTP response writer.
- `chatChannel` resolves the `Channel` at consume time, from the client the consumer holds. An unknown channel succeeds with nothing sent.
- A failure from `HandleCommand` fails the task and is retried. A command is answered at least once, so a retry after a half-delivered answer can post twice. See Risks.

### D-6: Idempotent create under redelivery

A signal with a `signalId` already has a deterministic name, so a redelivery hits `AlreadyExists` and deduplicates. A signal without one is named from a clock and a random suffix, so a redelivery after a lost success would create a second `Signal`.

Decision: the task derives the name from the task id when there is no `signalId`. The name keeps the `sig-` prefix and time-ordered sort is lost for those signals, which the consumer's arrival order tolerates.

Alternative rejected: the existing generated name plus a lookup. Publication is lookup-free by design.

### D-7: Folding into the capabilities `async-command-bus` (#473) adds

No delta touches `async-operations` or `command-bus`. Neither exists in `openspec/specs/` on master. They are the capabilities `async-command-bus` (#473) introduces and archives.

At archive time, after #473:

- the `ingest-signal` row in the command table of `async-operations` and the `in.signal` entry in the queue table of `command-bus` must be folded in as MODIFIED deltas, if #473's text differs from what this change relies on
- the accept-step list in `async-operations` must stop saying the `Wired=False` refusal applies to a signal, as D-3 settles

It is a dependency, not a courtesy. This change relies on the accept step, the task and invocation ids, the retry and dead-letter semantics and the `in.signal` declaration. Archiving first would leave a spec that refers to concepts that do not exist.

### D-8: Test impact

About 15 cases in `internal/httpapi/publish_test.go` and `signals_test.go` call `handleSignalInbound` and assert `200`, `queued` and a `Signal` already present on return. They are the synchronous reply, and each needs one of two changes:

| Case shape | Change |
|---|---|
| asserts the status and `queued` only | expect `202`, add `taskIds` |
| asserts a `Signal` exists after the call | run the consumer in the test, then assert |

Tests of `answerCommands` and `publishOne` move to the consumer's package. A new case covers redelivery creating one `Signal`. The conformance suite gains the `ingest-signal` verb. `tasks.md` lists them.

## Risks / Trade-offs

- [Accepted signal, no `Signal` yet] -> the durable task, and a dead task is visible in `/state/tasks`.
- [Dead task never replays] -> the adapter was told `202`. Surface it in the task stream and in an activity event. Replay is a later change.
- [Answer posted twice on a retry] -> `HandleCommand` answers are idempotent per message where the surface allows. Otherwise accepted, an at-least-once answer beats a lost one.
- [Order across keys in a batch] -> not relied on by any consumer, D-2.
- [External adapter checks `== 200`] -> contract says any `2xx`, MINOR bump, changelog.
- [`queued` means accepted, not stored] -> documented on the contract page.

## Migration Plan

1. Ship the consumer and the enqueue together. A manager that enqueues without a consumer would accept and never store.
2. The `in.signal` queue and credential exist, so there is no chart change.
3. Rollback is the previous manager. Tasks still enqueued are abandoned with the bus's retained log, and their signals are re-sent only if the adapter re-sends. Drain `in.signal` before rolling back.

## Open Questions

1. **`Wired=False` at accept.** Settled: follow `signal-persistence`. An unwired source is accepted with `202`, and the consumer stores the `Signal` with no `status.phase`, and routing later records `Dropped` with reason `unclaimed`. The accept-step list in `async-operations` stops listing the refusal for a signal when D-7's fold happens.
2. **Fully filtered batch.** Settled: `202` with `queued: 0`, empty `taskIds` and `reason: filtered`.
3. **Header rename.** The requirement "The inbound response acknowledges storage, never routing" no longer describes storage. The delta keeps the exact header so it MODIFIES cleanly. A rename (RENAMED then MODIFIED) is cleaner and costs one more delta entry.
4. **Dedup requirement text.** "A re-send of the same message is deduplicated" still says "publication". D-1's delta defines publication as the consumer's create, so no third delta is made. Say so if a reader finds it unclear.
5. **Contract version.** Settled: `2.4.0` (MINOR) from `2.3.0` on master, as the signal-adapter-contract delta states. Task 5.1 re-checks that `2.3.0` is still current.
