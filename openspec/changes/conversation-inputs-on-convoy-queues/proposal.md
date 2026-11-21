## Why

A conversation runs one input at a time, in order, while other conversations run in parallel. That is a **sequential convoy**. The command bus now has one for exactly this (`convoy-queues`, PR #532). Conversation inputs still use a hand-built copy of the same idea.

Today the copy works like this:

1. An input is appended to `Conversation.Inputs` on the conversation record.
2. The runtime pod long-polls `GET /work`. The manager re-reads the record every 2 seconds.
3. `dispatch.Next` returns nothing while `Inflight` is set. Otherwise it takes the pending inputs and sets `Inflight` with a compare-and-swap.
4. `POST /work/done` records the run, appends the input ids to `ProcessedInputIDs`, clears `Inflight` and moves the phase.
5. An unavailable context holds an input by leaving it pending.

Every part of that is delivery protocol, written by hand and kept on the record. The record is rewritten for each input, each dispatch and each report.

A convoy queue per conversation already gives one message in flight, order, redelivery and a delay. The server enforces it, with no lease and no compare-and-swap on a record.

## What Changes

- **Inputs use the bus's convoy queue.** One in flight, order, dedup by command id and delayed redelivery are the Bus's, and #532 already builds them. This change adds only what is specific to conversations:
  - a declared input family, durable and never dropped
  - a redelivery wait longer than a run
  - a purge when a conversation is deleted
  - the manager and record side: claim from the queue in `/work`, settle in `/work/done`, and the migration
- **`GET /work` claims from the conversation's convoy queue.** It no longer re-reads the record every 2 seconds. `POST /work/done` settles the claim.
- **The runtime contract does not change.** Runtimes still long-poll `/work` and report to `/work/done`. No runtime changes.
- **The hand-built protocol is retired.** `Conversation.Inputs` and `ProcessedInputIDs` leave the record. The compare-and-swap dispatch and the "nothing inflight" check go with them.
- **What the record keeps.** The run history, with the inputs recorded on each run, stays the permanent record. A small "current run" stays for what reads it today (see the design).
- **Phase and pending count come from the queue.** `Queued` and `Idle` are derived from it.
- **A held input becomes a delayed redelivery.** An unavailable context no longer leaves the input pending on the record.
- **Close and delete remove the conversation's convoy queue.** The cascade already enqueues in order.
- **The console reads queued inputs through `/state`**, from the queue.
- **One-time migration.** Inputs already stored on records move to their queues during the rollout.

This is an implementation change behind the existing `Bus` interface, plus the removal of the hand-built protocol. It adds no new abstraction.

**BREAKING:** the conversation record no longer carries `Inputs` and `ProcessedInputIDs`. Anything that reads them directly breaks. The design lists the readers.

## Open questions

The design settles these. None is guessed here.

- **Durability.** An input is a person's message. It cannot be derived again. The bus keeps an unconsumed message for 7 days, and the `memory` backend keeps nothing across a restart. The record keeps an input until its run is recorded. The queue must give the same guarantee, or the change loses messages. This question comes first.
- **Batching.** Today same-type pending inputs go out as one unit. A convoy queue hands over one message at a time.
- **Pending count and peek.** The console and the phase need the number and the text of the waiting inputs.
- **Migration during a rolling update.** Old replicas still write inputs to the record while new ones read the queue.
- **Direct consumption by the pod.** Whether a runtime pod could consume its queue itself. This is a spike, not a commitment.
- **Redelivery against the runtime's retry.** A redelivered input and a retried report must not run or record twice.
- **Large payloads.** An input's body can be a blob (`PayloadRef`).

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `command-bus`: the conversation input family is durable and never dropped, its redelivery wait outlives a run, and deleting a conversation purges its queue.
- `state-store`: the conversation record holds no queue of inputs, and a settled input releases its payload.

`conversation-capacity` and `console-application` do not change. "Nothing inflight and nothing queued" and the work queue view keep their behaviour. Only how they are derived changes, which is design and not spec.

## Impact

| Area | Change |
|---|---|
| `platform/manager/internal/httpapi/server.go` | `tryDispatch` and `handleWorkDone` claim and settle through the bus |
| `platform/manager/internal/dispatch/` | `Next` stops reading `Inputs` and `Inflight` |
| `platform/manager/internal/state/`, `state/kube/`, `state/nats/` | `ConversationRecord` loses `Inputs` and `ProcessedInputIDs`. The `kube` mapping carries 19 uses of them |
| `platform/manager/internal/chat/`, `internal/signalroute/` | every place that appends an input submits to the queue instead |
| `platform/manager/internal/controller/` | the conversation reconciler derives phase and capacity from the queue |
| `platform/manager/internal/httpapi/state.go` | `/state/conversations` serves queued inputs from the queue |
| `platform/console/` | the work queue view |
| `docs/` reference | `concepts.md`, `contracts.md`, `contracts/runtime.md` (the serial rule and how `/work` claims), `configuration.md`, `console.md`, `crds/conversation.md` and `crds/conversation-input.md` (the deprecated fields, generated), `testing.md`, `CHANGELOG.md` |
| `.claude/rules/` | `invariants.md` (the strictly-serial row, the queue logs as a bounded state home, the conversation messages row), `terminology.md`, `wiring.md` |
| Adopter site | `introduction.md`, `getting-started.md`, `installation.md` and `docs/guides/*` do not mention the input queue. The documentation task checks each and records that. The `installation.md` upgrade note for the migration is the one addition |

**Builds on #532, which is merged.** The convoy queue code is in master. The `command-bus` spec on master still describes the log engine that #532 replaced, and no spec change for #532 exists yet. This change's deltas are additive and do not rely on those stale requirements. That spec change is owed separately.
