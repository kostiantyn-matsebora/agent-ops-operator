# Design: conversation-message-timeline

## Context

Two mechanisms were built for different jobs and neither was asked to produce a conversation's history, so nothing does.

- **`spec.inputs[]` is a work queue.** `pruneProcessed` removes each entry once dispatch has consumed it, and deletes the overflow `ConversationInput` object with it. That is correct for a queue — an un-pruned queue re-runs answered work — and it means the object stops holding what a person said.
- **`status.runs[]` is a result list.** It is durable, ordered by time, and carries `inputIds` linking a run to what it answered. The link outlives the thing it names.
- **Ops are a delivery queue.** At-least-once, in-memory, re-derivable from CR state. Never storage.

So a conversation durably records the answers and not the questions, and a viewer can rebuild half a thread. The console demonstrated it: after a restart the Runs tab was full while the Transcript said no messages. `mergeTranscript` fixed that half by merging `status.runs[]` into the buffer — which works precisely because answers ARE durable, and cannot work for questions, which are not.

The delivery rule has the matching hole. `InputItem.PostToChannels` decides once, from the origin KIND, whether a person's message reaches bound threads. `signal` posts, `channel` does not, and `kind: chat` does not — with the last written down as an exception because a chat message is a signal whose sender already saw it.

The shape of that exception is the tell. "Already seen" was being asserted about the MESSAGE, and it is only ever true of a (message, destination) pair.

The specs already half-state the intended behaviour. `console-adapter` says: *"the manager fans a console user's own message back to the console thread as part of multi-channel relay — the console renders it (confirming the pending message) and does not re-submit it inbound."* The console implements its side. The manager never sends it, because the message is filtered out one layer earlier.

## Goals / Non-Goals

**Goals:**

- One durable ordered record per conversation, readable by any viewer, identical for all of them.
- One delivery rule with no per-lane exceptions.
- Delete the console workaround rather than keep it as a fallback.
- Additive migration: old conversations render what they have.

**Non-Goals:**

- Changing what pruning is for. The queue keeps being pruned.
- Changing addressing, the ops message types, or how adapters render.
- A message store outside the Kubernetes API.
- Reconstructing history for conversations that predate the change.

## Decisions

### D1: The record lives on the run that consumed the input

`status.runs[]` already has everything a record needs except the text: it is durable, ordered, and `inputIds` names the inputs each run answered. Adding the consumed inputs there gives the existing link something to point at, and the timeline becomes a read of one field in document order.

*Alternatives considered.*

- **A separate `status.messages[]`.** Cleaner to read, but it duplicates ordering and delivery bookkeeping that runs already carry, and it invites drift between two lists describing one conversation.
- **Stop pruning the queue.** Smallest diff, worst outcome: dispatch reads that queue, so retaining processed entries risks re-running answered work — the exact failure pruning exists to prevent.
- **Keep the console's capture-before-prune.** Rejected as the thing this change removes. It is per-viewer, in-memory, and needs text matching to recover facts the CR could simply carry.

*What it costs:* an input that is consumed by no run — dropped, refused, or still queued when a conversation is closed — has no run to hang on. Those are addressed in D4.

### D2: Delivery is decided per destination, from the origin SURFACE

One rule: deliver to every bound channel except the surface the message entered on.

The origin surface is already recorded. A chat signal carries `agentops.dev/channel`; a channel-origin input names its channel. So the rule needs no new state, only a different question — *did THIS destination display it?* instead of *has anyone seen it?*

Three special cases disappear into it:

| Case | Today | Under the rule |
|---|---|---|
| alert with no matching channel | posted (origin kind `signal`) | delivered — no surface displayed it |
| reply typed in a bound thread | withheld, relayed to siblings only | delivered to every channel but its own |
| `kind: chat` message | withheld everywhere, by a stated exception | delivered to every channel but its own |

The exception clause in `conversation-opens-with-its-input` is deleted, not reworded. An exception that exists because the general rule asks the wrong question is a symptom, not a feature.

### D3: A viewer's buffer becomes a cache, and its own message is confirmed rather than invented

The console currently renders a typed message optimistically and — since 0.15.7 — also captures the CR input, recovers the sender by matching text, and adopts the existing bubble so it is not shown twice. Under D1 and D2 all three disappear:

- the message arrives by the channel path, like every other message;
- its sender arrives with it, structured, as relays already do;
- a reload reads it from the record.

The optimistic bubble stays, because a viewer should not wait for a round trip to show what someone typed. It is confirmed by the delivered copy — which is exactly what the pending mechanism was built for and what `console-adapter` already specifies.

### D4: What the record does NOT get

An input that no run consumed has no run to be recorded on. Rather than inventing a home for each case, the change states them:

- **Still queued** — it is in `spec.inputs[]`, which is where a viewer reads pending work today. Nothing changes.
- **Refused or dropped before it became an input** — it never entered a conversation, and the surface was answered directly. Nothing to record.
- **Queued when the conversation was closed** — pruned with the queue, unrecorded. This is a real gap, accepted deliberately: the alternative is a second record for messages nobody answered.

### D5: The inline cap, and why it is not optional

`ConversationInput` exists so a large payload does not sit in the Conversation object. Copying that payload into status would undo it, and etcd object size is a hard limit rather than a preference.

So text is inlined up to a cap and left as a reference beyond it, with the reader told which it got. The cap belongs to the manager and is not a per-conversation setting: an installation that could opt out of it would be an installation that could break its own conversations.

## Risks / Trade-offs

- **Object growth.** Every message now adds bytes to one object. Bounded by D5's cap, and by conversation retention, which already exists. Worth watching on long-running conversations with many short messages.
- **More traffic to adapters.** Messages that were withheld are now delivered. This is the point, but it is a real increase, and a transport that echoes its own surface's messages must be excluded correctly or its users see doubles.
- **The no-relay-loops rule becomes load-bearing in a new place.** A message may now be delivered toward the transport it came from when that transport serves several surfaces. Every adapter already must not re-ingest its own outbound posts; this makes that guarantee matter more.
- **Deleting the console workaround is a behavior change for one release.** Conversations created under 0.15.7–0.15.9 hold their opening message only in a viewer's memory. After the upgrade they render from the record, which they do not have. Stated, not worked around.

## Migration Plan

1. The record is additive and optional. A run written before the change carries no inputs, and a viewer renders what it has.
2. The delivery rule changes what adapters receive. Deploy the manager before removing the console workaround, so the console never depends on a message that is not yet delivered.
3. Remove the console capture, hints and adoption once the manager delivers. The optimistic bubble and its confirmation stay.
4. `docs/concepts.md` "How a message travels" loses its *Not yet implemented in full* blockquote in the same change that makes it true.
5. Rollback is reverting the manager: the record stops being written, old records stay readable, and delivery returns to the origin-kind rule.
