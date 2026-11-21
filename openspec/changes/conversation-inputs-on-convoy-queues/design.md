## Context

See `proposal.md` for the why. This page holds the facts the decisions rest on.

| Fact | Where |
|---|---|
| A conversation's queued inputs are `ConversationRecord.Inputs`. A claim is `Inflight` plus a compare-and-swap on the record revision | `internal/state/record.go`, `internal/httpapi/server.go` `tryDispatch` |
| `GET /work` polls the record every 2 seconds, up to 30 seconds per call | `handleWork` |
| `dispatch.Next` returns nothing while `Inflight` is set. It batches consecutive same-type pending inputs into one unit | `internal/dispatch/dispatch.go` `batchSameType` |
| `POST /work/done` records the run, appends to `ProcessedInputIDs` (bounded to 50), clears `Inflight`, then moves the phase in a second write | `handleWorkDone` |
| An unavailable context holds the input by not consuming it | `handleWorkDone`, `tryDispatch` |
| `Claim(ctx, queue, consumerID)` takes the next task of a **queue**, not of a key | `internal/commandbus/commandbus.go` |
| Settle and fail need the task uid and the claim's token | same |
| Routing maps a queue name to regular, convoy or hand-over. The default is convoy | `internal/commandbus/nats/bus.go`, `wm_pubsub.go` |
| A claim reads one subscription per queue per process and returns any key's next message | `internal/commandbus/nats/lifecycle.go` `feedFor` |
| The record is shared by the `kube` and `nats` state backends. The `kube` mapping has 19 uses of the input fields | `internal/state/kube/mapping.go` |
| 13 non-test files read `Inputs`, `Inflight`, `ProcessedInputIDs` or `PendingInputs` | grep over `internal/` and `cmd/` |
| The same specs lean on an "inflight run" on the conversation | `aops-mcp-server`, `console-live-runs` |

## Goals / Non-Goals

**Goals:**

- One place owns "one input at a time, in order" for conversations: the convoy queue.
- No input is lost, by a restart, by the bus's retention or by a backend that forgets.
- A rolling update loses nothing and needs no downtime.

**Non-Goals:**

- No change to the runtime contract. `/work` and `/work/done` keep their shape and their status codes.
- No change to how channels, signals or the `conv` queue deliver.
- No new abstraction. The two additions, `ClaimKey` in D3 and `Purge` in D5, are called out and can be refused.

## Decisions

### D1. An input is never dropped by the bus

An input is a person's message. It cannot be derived again. Every other queue may lose a message, because the reconciler re-derives the work. This one may not.

The record keeps an input until its run is recorded. The queue must give the same guarantee.

| Option | Verdict |
|---|---|
| **A. A durable input family.** No time to live, no expiry, no dead-letter drop. Its own records bucket, because the shared one has a bucket-wide time to live. | **Chosen** |
| B. Keep inputs on the record and use the queue only as a doorbell | Rejected. It keeps the hand-built protocol |
| C. Write the record and the queue both | Rejected. Two sources of truth that drift |

What option A needs:

1. The input family declares no time to live. A task of this family leaves the queue only by settle, or when its conversation is deleted.
2. Its records live in a bucket with no age limit. Whether NATS 2.10 can expire per key is **to verify** in the first task. If it cannot, a separate bucket is the answer either way.
3. A failure parks and retries. It never ends in the dead-letter drop.
4. The `memory` bus loses inputs on a restart. That is declared, not hidden. It stays for tests and a single replica.

### D2. One family, the conversation is the key

The family's working name is `in.conversation`. It is not final until the inventory is changed, see Open Questions. A message's key is the conversation name.

Routing in the bus is a function from the queue name to a mode, and the default is convoy. So the family needs no routing entry. One convoy queue per key, one message in flight, order and parallelism across keys are the bus's own (#532).

### D3. `/work` claims the conversation's own key

`Claim(queue, consumerID)` takes the next message of **any** key of a queue. `GET /work?convo=X` needs X's message, and a claim that returned another conversation's input would run it in the wrong pod.

| Option | Verdict |
|---|---|
| **A. A key-scoped claim, as an optional interface beside `Renewer`** | **Chosen** |
| B. One queue name per conversation, as `out.<channel>` does | Rejected. Each name adds a subscription and a registry watch in every process, and `feedFor` keeps them for the life of the process, so deleted conversations leak |
| C. Claim any key and route the message to its pod | Rejected. The pod pulls today, and a dispatcher would invert the model |

The optional interface is `ClaimKey(ctx, queue, key, consumerID)`. The convoy queue consumer controller already attaches to one key at a time, so a key-scoped claim attaches only that key's convoy queue. The `memory` backend implements it too, and the conformance suite covers it.

The long poll waits on that claim and not on a record re-read every 2 seconds. A new input reaches an idle pod as soon as it is published.

### D4. The record keeps a small current run

`Inflight` stays, smaller. It holds the run id, the dispatch time, the task uid and the claim's token. It drops `InputIDs`.

Two reasons:

1. `POST /work/done` arrives in a separate request, possibly on another replica. Settle needs the task uid and the claim's token. They have to live somewhere shared.
2. The `aops-mcp-server` and console specs record a move and show presence on "the inflight run".

`Inputs` and `ProcessedInputIDs` go. Settle by task uid and the command id dedup replace the processed ids.

### D5. Closing, deleting and pending counts need reads and one write on the bus

| Need | How |
|---|---|
| Is anything waiting, and how many | `Stats(queue)`. The phase is `Queued` when it is above 0 or a run is inflight, otherwise `Idle` |
| The text of what is waiting, for the console | `Peek(queue, limit)`. Today it reads hand-over queues. Whether it can read a convoy queue is **to verify** |
| A held input | `Fail` with a delay, so the redelivery waits for the context to come back |
| Delete a conversation | A new `Purge(queue, key)` on the bus. It discards the key's waiting messages and removes its convoy queue |

`Purge` and `ClaimKey` are the two additions. `ClaimKey` is an optional interface. `Purge` is on `Bus` itself.

- The alternative is to drain every message with a settle. That records a success for each message that never ran.
- A closed conversation keeps its waiting inputs, as it keeps everything else today.
- Only delete removes them.

### D6. The redelivery timer outlives a run

Exclusivity does not come from a lease. The server allows one message in flight per convoy queue, and every replica attaches to it. No replica holds the queue.

The broker still redelivers a message nobody acknowledges. That timer is the ack wait. It is not exclusivity. It is the safety net for a message whose holder is gone.

A run can last 3600 seconds, the runtime's `RUN_TIMEOUT_MS` default. If the timer is shorter, the broker delivers the input again while the run goes on.

| Option | Verdict |
|---|---|
| **A. Set the ack wait of the input family above the run limit, plus a margin** | **Chosen** |
| B. A replica sends in-progress signals while the run is inflight | Rejected. It needs a holder that survives, a heartbeat and renewal logic, which is the leasing we dropped |

A pod that dies mid-run does not wait for the timer. The conversation reconciler already clears `Inflight` when the pod is gone. It will now fail the task with a delay, so the input redelivers.

### D7. Batching is dropped unless measured

Today consecutive pending inputs of one type go out as one unit. A convoy queue holds one message in flight, so it cannot hand over a batch without a second claim.

| Option | Verdict |
|---|---|
| **A. One input, one run** | **Chosen, unless the measurement says otherwise** |
| B. Claim the head, peek the followers, settle them together | Needs a second claim path and breaks one-in-flight |
| C. Merge a new input into a waiting message at submit | Messages are immutable once published |

The measurement:

- It is the share of runs that carry more than one input, read from `runs[].inputs` in the run history.
- The task list runs it before any code changes.
- If the share is high, this decision is reopened first.

### D8. Direct consumption by the pod is not done

A runtime pod could consume its queue itself. That needs a bus credential in the pod. The invariants say the manager reads no secrets and runtime pods talk only to the manager. The `/work` contract is also the one every runtime already implements.

The first task spikes it, to record the answer with evidence. The expected answer is no.

### D9. Redelivery and a retried report

Nothing changes in kind:

- A retried report finds `Inflight` already cleared for that run id and is judged as today.
- A redelivered input after a pod crash runs again, as it does today when `Inflight` is cleared and the inputs re-dispatch.
- Delivery stays at least once, and the report stays idempotent per run id.

The `ProcessedInputIDs` list was bounded to 50 and could forget. The command id dedup window replaces it and is also bounded. Whether the window is long enough for a retry after a manager restart is **to verify** in the system tests.

### D10. Large payloads

A message carries the input's `PayloadRef` and not the body. The blob is deleted after the settle, as it is today after pruning. `releaseInputBlobs` already deletes the blobs of a deleted conversation's queued inputs. It reads the queue through `Peek` instead of the record.

### D11. Migration: dual read for one release

This repository already has the pattern, for example the deprecated agent field in `dispatch.Next`.

| Release | Behaviour |
|---|---|
| N | Every writer submits to the queue. The reader also finds legacy inputs on a record. A leader sweep submits each one with its input id as the command id, which makes it idempotent, then clears it from the record. The record field and the CRD field stay, deprecated |
| N+1 | The fields are removed |

During N's rollout an old replica still appends to the record. The sweep runs on a timer, not once, so it catches those too.

## Risks / Trade-offs

- **[A message is lost]** → D1, and a system test that restarts the whole bus and the manager with inputs waiting.
- **[A redelivery timer shorter than a run]** → D6, and a test with a run longer than the ack wait of the other queues.
- **[Two sources of truth during N]** → the sweep is idempotent by input id and runs on a timer. The system test rolls the manager under load.
- **[Dropping batching costs more runs]** → D7 measures first.
- **[The `kube` state backend has no bus]** → a `kube` installation now needs a durable bus. The `memory` bus stays valid only for tests and one replica, and the docs say so.
- **[`Purge` widens the `Bus` interface]** → it is the only change to `Bus` itself, and both backends implement it in the conformance suite.

## Migration Plan

1. Release N with the dual read and the sweep, behind no flag. A rollback to N-1 finds inputs on the queue that N-1 does not read. So the sweep also writes nothing back, and N-1 is not a supported rollback target after the first sweep. The release notes say so.
2. Wait one release. Confirm no record carries a legacy input.
3. Release N+1 removes the fields.

## Open Questions

- Whether `Peek` can serve the console's list for a convoy queue, or the console needs its own read.
- The exact family name. `in.conversation` is a working name until the inventory is changed.
