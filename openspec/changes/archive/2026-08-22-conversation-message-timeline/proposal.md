# Proposal: conversation-message-timeline

## Why

A conversation durably records **the answers and not the questions**. `status.runs[]` survives forever; `spec.inputs[]` is a work queue that `pruneProcessed` deletes entry by entry as the agent consumes it, taking the overflow `ConversationInput` object with it. The link survives — `runs[].inputIds` still names the input a run answered — but the text it names is gone.

The delivery rule has the matching hole. The manager decides ONCE, globally, from `InputItem.Origin`, whether a person's message reaches bound threads: `signal` posts a card, `channel` does not, and a `kind: chat` origin does not, because "the person typed it". That treats **already seen** as a property of the MESSAGE. It is a property of the **(message, destination) pair**: a message typed on surface A was seen on A and is new to every other bound channel.

Together the two holes mean a viewer can rebuild half a thread. A conversation started from the console composer showed a transcript beginning at the agent's answer, with the question that caused it missing — the message was never delivered to the console, and by the time anything looked for it the input had been pruned.

The console shipped a workaround for this in 0.15.7–0.15.9: watch conversations, capture typed inputs into an in-memory buffer before pruning, match them to origination hints by exact text to recover the sender and the as-typed form, and adopt an existing local bubble so a reply is not rendered twice. Three defects surfaced in one evening — a missing message, a truncated one, a duplicated one — and each was a symptom of reconstructing a timeline from a work queue. The workaround is the reason to fix the model rather than evidence that patching works.

## What Changes

**One delivery rule, decided per destination.** Deliver every message — a person's input and the agent's output alike — to every bound channel EXCEPT the one it entered on, because that surface displayed it already. The origin SURFACE is what the rule reads, not the origin KIND.

- **BREAKING to a stated invariant**: `InputItem.PostToChannels` and its three-way `signal` / `channel` / `kind: chat` decision are replaced. The stated exception ("a chat message reaches the manager as a signal, but the person typed it") disappears, because the surface-based rule already covers it.
- The sibling-channel relay stops being a special case and becomes the general rule: A→B is simply "B is not the origin". A source with no matching channel (an alert) falls out of the same rule, since no surface displayed it.
- An input carrying no origin keeps its current behaviour — delivered nowhere — so upgrading cannot fill open threads with history.

**One durable sequence.** The text a person sent is kept on the run that consumed it, so the whole timeline is derivable from `status` in order: input, result, input, result.

- `status.runs[]` gains the consumed inputs (id, text, received time, origin surface). `runs[].inputIds` already carried the link; this gives it something to point at.
- The queue keeps being pruned exactly as it is now. Pruning is what stops dispatch re-running answered work, and nothing about it changes.
- **Bounded**: text is inlined up to a cap and left as a `PayloadRef` beyond it. Large payloads are offloaded to a `ConversationInput` object today precisely to keep etcd small, and an uncapped copy in status would undo that.

**The console stops reconstructing.** Its transcript becomes one ordered read of CR state plus live ops for what was never CR state (acks, signal cards). The watcher, the sender hints, the as-typed recovery and the bubble adoption added in 0.15.7–0.15.9 are deleted, not kept as a fallback.

## Capabilities

### New Capabilities

- `conversation-message-timeline`: a conversation's messages as ONE durable ordered record — what is kept, in what order, and what a viewer may reconstruct from it after a restart, a reopen, or a surface joining late.

### Modified Capabilities

- `conversation-opens-with-its-input`: the posting rule becomes per-destination and is read off the origin SURFACE. The `kind: chat` exception and the "channel origin is never posted" clause are removed.
- `multi-channel-conversations`: relay generalizes from "sibling channels" to "every bound channel except the origin surface", so a single-channel conversation delivers a person's message to its own viewer when that surface did not display it.
- `state-durability`: the message timeline is named as Kubernetes-API state. A person's words move out of the deliberately-lossy class, where nothing ever declared them.
- `console-adapter`: the transcript is rebuilt from the durable sequence rather than from a captured buffer, and the console's own user message is confirmed by the copy the manager delivers back.

## Impact

- **Manager**: `InputItem.PostToChannels` and its callers; the input-card path in the Conversation reconciler; the relay path in `internal/chat`; `pruneProcessed` gains a copy-before-prune step.
- **CRD**: `Conversation.status.runs[]` gains an inputs list. Additive and optional — a run written before this change simply has none, and a viewer renders what it has.
- **Console**: `typedinputs.go` and the hint/adoption code in `transcript.go` are deleted; `mergeTranscript` becomes an ordered merge of `status.runs[]` (inputs and results) with live ops.
- **Docs**: `docs/concepts.md` "How a message travels" loses its "Not yet implemented in full" blockquote; `docs/contracts.md` for the delivery rule; `CLAUDE.md` invariant rewrite; `CHANGELOG.md`.
- **Non-goals**: no change to addressing, to the ops contract's message types, to pruning's purpose, or to how adapters render. The no-relay-loops rule is unchanged and becomes load-bearing in one more place, since a message may now be delivered toward its own transport's siblings.
- **Migration**: additive. Old runs carry no inputs and render as they do today. The first upgrade posts nothing retroactively.
