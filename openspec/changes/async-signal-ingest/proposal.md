## Why

`async-command-bus` (#473) put every manager operation but one on the durable
command bus. `/signal/inbound` still does its work inside the HTTP request:

- It validates, records the "signal received" receipt and applies the source's
  attribute filters.
- It answers in-place chat commands through the router (`answerCommands`).
- It writes one `Signal` CR per admitted signal, then replies `200 {"queued": n}`.

A fault in any step is a `5xx` the adapter must re-send. There is no bounded
retry and no dead-letter record, and the task stream (`/state/tasks`) cannot
show this path. The `in.signal` queue is already declared in the bus inventory
with no producer or consumer. This change gives it both.

## What Changes

- **`/signal/inbound` accepts, then processes.** The request keeps authentication,
  scope, body shape, the unknown-source refusal, the attribute filters and
  the "signal received" receipt (an unwired source is not refused: the consumer
  records `Wired=False` on the `Signal`, as `signal-persistence` says today), then enqueues one
  `ingest-signal` task per admitted signal on `in.signal` and replies `202` with
  `{invocationId, taskIds}`, still carrying `queued` and `reason`.
- **An `in.signal` consumer does the rest**: answering an in-place chat command
  and creating the `Signal` CR, with the bus's bounded retry, backoff and
  dead-letter record.
- **Ordering and dedup are unchanged**: the key is the source and the
  `initiationId`, which stays the deduplication key. A signal carrying a
  `signalId` that already landed is still deduplicated.
- **No adapter changes.** An adapter that predates the bus keeps posting the same
  body and treats any `2xx` as accepted.
- **BREAKING (status only)**: the success status moves from `200` to `202`. The
  `queued` count now means "accepted", not "stored".

## Capabilities

### New Capabilities
- `async-signal-ingest`: the `ingest-signal` command on `in.signal`, its accept
  and process split, retry and dead-letter behaviour and its conformance
  scenarios.

### Modified Capabilities
- `signal-persistence`: a `Signal` record is created by the `in.signal`
  consumer, after acceptance, not inside the request.
- `signal-adapter-contract`: the inbound response is `202` with the invocation
  and task ids, and `queued` counts accepted signals.

Depends on `async-command-bus` being archived: its `async-operations` and
`command-bus` specs name the accept step and the queue table this change extends.

## Impact

- **Code**: `platform/manager/internal/httpapi` (`publish.go`, `accept.go`), the
  `in.signal` consumer in `internal/commandbus` wiring in `cmd/manager`, about 15
  cases in `publish_test.go` and `signals_test.go` that assume the synchronous
  reply, and the conformance suite for the new verb.
- **Reference docs**: `docs/contracts.md` (the inbound signal reply and the queue
  table), `docs/contracts/signal-adapter.md`, `docs/concepts.md`, and
  `docs/CHANGELOG.md` for the `200` to `202` move.
- **Adopter site**: the signal adapter guides under `docs/guides/` and
  `docs/integrations/` that describe the reply, plus `docs/installation.md` where
  it names the queues.
- **Not affected**: the signal adapters themselves, the chart (the queue and its
  credential exist), and the retry and TTL values already declared for `in.signal`.
