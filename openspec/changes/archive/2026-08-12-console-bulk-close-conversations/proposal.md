## Why

Closing conversations is a one-at-a-time gesture: `/close` inside a thread, or
`kubectl delete` for someone with cluster access. After an event storm — the
case the observing lanes exist for — an operator faces dozens of finished
conversations and the console, the surface built for exactly this person, can
only offer them one thread at a time. The console already lists, filters and
paginates those conversations server-side; it is the natural place to end a
batch of them, and the only place that does not require kubectl.

The manager-side counterpart is `conversation-housekeeping`, which proposes
automatic retention. That is the unattended half. This is the attended half: a
person looking at a filtered list and deciding *these* are done. Retention by
age cannot make that judgement, and waiting for retention to arrive leaves the
console with no close action at all.

## What Changes

- **The console conversation list gains checkbox selection and a Close action.**
  Selection is over the rows on screen — the operator picks conversations
  explicitly, and the batch is bounded by what was visible. There is no
  "select everything matching the filter" escape hatch: a mis-set filter would
  then close far more than was ever on screen.
- **Bulk close is the existing `/close`, fanned out — nothing new reaches the
  Kubernetes API.** The console posts `/close` on each selected conversation's
  own console thread through `POST /channel/inbound`, exactly as a person typing
  it would. The manager intercepts it on the reply path, archives the threads
  and deletes the `Conversation`. No manager endpoint is added, no adapter
  contract verb is added, and the console gains no Kubernetes write path — the
  standing invariant that its only write is `POST /channel/inbound` holds
  unchanged.
- **Reach is therefore JOINED conversations only.** A conversation the console
  merely observes has no console thread and so has nowhere to post; it is
  reported as skipped with that reason, not silently dropped. This makes the
  existing joined/observed distinction visible as a consequence rather than
  introducing a new rule.
- **Working conversations are excluded by default**, with an explicit opt-in
  that names what it does (`include working — abandons in-progress runs`).
  `/close` honors an inflight run by abandoning it; that is right for a
  deliberate single close and wrong as the silent default of a bulk gesture.
- **The result is per-conversation, not a single verdict.** A batch reports each
  conversation as closed, skipped (observed / working) or failed, because a
  partial batch is the normal outcome and "12 of 15 closed" is the only honest
  summary.
- **The action is a write**: the install-wide write gate and the identity
  requirement apply exactly as they do to origination and replies, and each
  close is logged against the identity that ordered it.

No breaking changes: the endpoint is new, the UI is additive, and `/close`
itself is untouched.

## Capabilities

### New Capabilities
- `console-bulk-close`: selecting conversations in the console and ending them as
  a batch — what may be selected, how the batch is scoped and confirmed, what
  each per-conversation outcome means, and how the operation is gated and
  attributed.

### Modified Capabilities
- `conversation-close`: `/close`'s existing behaviour is unchanged; the delta
  ADDS the rule it has always relied on and never stated — that the command on
  the reply path is the ONLY way to close, so a batch is N ordinary closes and
  the manager needs no bulk or administrative close verb, and that a closer's
  reach is bounded by the threads it holds.

## Impact

- **`console/convapi.go`** — a new bulk-close handler; per-item outcome type.
- **`console/api.go`** — one route registered behind `a.write(...)`.
- **`console/adapter.go`** — reuse of `Send`/`ThreadFor`; the `errNotJoined`
  path becomes a reported per-item outcome rather than a request-level error.
- **`console/ui/src/pages/Conversations.tsx`** — selection column, bulk action
  toolbar, confirmation modal, result summary.
- **`console/ui/src/api/client.ts` / `hooks.ts` / `types.ts`** — the mutation and
  its result type.
- **`docs/console.md`** — the action, its reach, and its trust boundary.
- **No changes** to the manager, the channel adapter contract, any CRD, RBAC, or
  the chart.
