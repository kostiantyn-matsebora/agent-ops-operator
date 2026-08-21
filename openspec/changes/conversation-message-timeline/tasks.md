# Tasks: conversation-message-timeline

## 1. The durable record

- [ ] 1.1 Add the consumed-input record to `Conversation.status.runs[]` in `api/v1alpha1/conversation_types.go` — id, text, received time, origin surface, and the payload reference for oversized text — as optional fields, then regenerate deepcopy and CRDs
- [ ] 1.2 Write the record where a run's inputs are resolved, so it is stored with the run rather than patched in a second pass that a restart could lose
- [ ] 1.3 Enforce the inline cap: text up to the limit is stored, longer text keeps its `PayloadRef` and is marked as elsewhere. State the cap in one place and name it in the CRD doc comment
- [ ] 1.4 Leave `pruneProcessed` doing exactly what it does, and add the ordering guarantee it now depends on: the record is written BEFORE the queue entry is pruned, or a crash between the two loses the message permanently
- [ ] 1.5 Tests: a consumed input is readable after pruning, an oversized payload is referenced rather than copied, a crash between recording and pruning loses nothing, and a run predating the change renders with no inputs

## 2. The delivery rule

- [ ] 2.1 Replace `InputItem.PostToChannels` with a per-destination decision that takes the input and the destination channel and answers whether that surface displayed it
- [ ] 2.2 Resolve the origin SURFACE for both origin kinds — a chat signal's `agentops.dev/channel`, and a channel-origin input's own channel — in one place, so the two lanes cannot drift
- [ ] 2.3 Delete the `kind: chat` exception and the "channel origin is never posted" clause. Both are subsumed, and leaving either as a guard would re-create the bug one layer down
- [ ] 2.4 Fold the sibling-channel relay into the same path: a relayed user message and an input card become one delivery decided by the same rule, keeping the structured attribution (`origin`, `sender`) adapters already receive
- [ ] 2.5 Keep the origin-less input behaviour exactly as it is — delivered nowhere — and keep its test, because it is what stops an upgrade filling every open thread with history
- [ ] 2.6 Keep delivery idempotent per message×channel with the existing stable op ids, so re-derivation after a restart cannot double-post
- [ ] 2.7 Tests: a message reaches every bound channel except its origin surface, two surfaces on one transport get the correct halves, an alert on a source nobody reads reaches every channel, and a single-surface conversation delivers a person's message to that surface

## 3. The console

- [ ] 3.1 Delete `console/typedinputs.go` and its tests — the watcher, the sender hints, the as-typed recovery
- [ ] 3.2 Delete the bubble adoption from `console/transcript.go` and the `AppendTyped` entry point it exists for
- [ ] 3.3 Rewrite `mergeTranscript` to merge the live buffer with the conversation's message record — inputs and results in time order — rather than results alone
- [ ] 3.4 Confirm the optimistic bubble against the DELIVERED copy, which is what `console-adapter`'s existing requirement already describes, so a typed message renders once
- [ ] 3.5 Keep the speaker label fix from 0.15.9: attribute to the sender when known, otherwise a word for who spoke, never an internal kind identifier
- [ ] 3.6 Tests: a conversation started from the composer reads question-then-answer, a typed reply appears exactly once, a restart rebuilds the whole thread from the record, and a relayed message stays attributed to its remote sender

## 4. Documentation and invariants

- [ ] 4.1 Rewrite the `CLAUDE.md` invariant "A thread opens with the event that caused it": the rule is per DESTINATION and is read off the origin SURFACE. State that the origin-KIND rule and its chat exception are deleted, and that re-adding either is the regression
- [ ] 4.2 Record in `CLAUDE.md` that a conversation's messages are Kubernetes-API state, that the queue and the record are different things, and that pruning must never be the only copy of what a person said
- [ ] 4.3 Note in `CLAUDE.md` that no-relay-loops is now load-bearing for same-transport siblings, since a message may be delivered toward the transport it entered through
- [ ] 4.4 Remove the "Not yet implemented in full" blockquote from `docs/concepts.md` "How a message travels" in the same change that makes it true
- [ ] 4.5 Update `docs/contracts.md` for what an adapter now receives, and `docs/console.md` for how the transcript is built
- [ ] 4.6 `CHANGELOG.md` entry, newest first, leading with the delivery-rule change and what an operator will SEE differently — messages that used to be withheld now arrive

## 5. Verification

- [ ] 5.1 `go build ./... && go vet ./... && go test ./...` at the root and in `console/`
- [ ] 5.2 Envtest coverage for the record surviving pruning and for delivery reaching the right destinations
- [ ] 5.3 Live smoke on a multi-surface conversation: start it from one surface, reply from another, and confirm each surface shows every message exactly once
- [ ] 5.4 Live smoke on restart: restart the console and the manager mid-conversation, reload, and confirm the full sequence rebuilds from the record
- [ ] 5.5 Confirm on a real install that object size stays sane for a long conversation, and that the inline cap behaves as intended for an oversized payload
