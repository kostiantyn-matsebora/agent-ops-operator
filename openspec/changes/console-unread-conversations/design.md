## Context

The console is a viewer that is also a channel. Conversations bound to it get a
console thread, and its only write anywhere is `POST /channel/inbound`. It mounts
no volume, and its state is always one of three things: a cache of a Kubernetes
object, derivable from Kubernetes objects, or declared lossy telemetry.

"Read" is none of the three. It has to be stored, it has to survive a console
restart, and it has to be the same answer in every browser — so it goes in the
Kubernetes API, on the object it is about. The console cannot put it there
itself: no write path to the Kubernetes API exists in the module, and that is not
being changed. The manager writes it, on the console's report, exactly as it
already does for `POST /channel/channels/{name}/status` and
`PUT /channel/state/{channel}/{key}`.

The grain is the **thread**, not the conversation. A conversation bound to
Telegram and the console has two audiences reading it in two places; one shared
mark would let a Telegram reader clear the console's badge. `status.threads[]`
already pins one binding per bound channel, and the watermark belongs on it.

## Goals / Non-Goals

**Goals:**

- A durable, per-channel record of how far a thread has been seen, written by the
  manager and readable from `kubectl`.
- Server-side unread filtering and counting in the console, consistent with the
  existing server-side filter/sort/paginate contract.
- Bulk mark-as-read over an explicit selection, bounded like bulk close.
- An upgrade that does not present the whole namespace as new.

**Non-Goals:**

- **Per-person read state.** Read is per channel: the console has seen this, not
  *you* have seen it. Two operators share one console thread and therefore one
  watermark. Per-user state would need a per-user identity store the console does
  not have and will not grow.
- **Unread for observed conversations.** No console thread, no watermark, no
  claim about newness.
- **Read receipts in other adapters.** The verb is available to every adapter;
  no adapter but the console is changed to call it.
- **Marking unread again.** A watermark only moves forward. "Mark as unread" is a
  separate feature and would need a different mechanism.
- **A sweep over everything.** Mark-as-read is selection-scoped, bounded at 50.

## Decisions

### 1. The watermark lives on `status.threads[]`, written by the manager

`ThreadBinding` gains:

```go
// ReadAt is how far this thread has been SEEN — reported by the adapter that
// serves the channel, never inferred. Monotonic: it only moves forward.
// +optional
ReadAt *metav1.Time `json:"readAt,omitempty"`
// ReadTracked marks a binding created after read reporting existed. A binding
// WITHOUT it predates the mechanism, and is treated as READ.
// +optional
ReadTracked bool `json:"readTracked,omitempty"`
```

*Why not the console's memory or the browser's:* the console persists nothing,
so a pod restart would republish the entire namespace as new; and browser storage
gives a different answer per browser while forcing the marks onto every list
request to keep filtering server-side.

*Why not one field:* `readAt == nil` has to mean two different things — "never
read" for a new binding, and "predates this feature" for an old one — and no
timestamp can separate them. This is the same shape as
`status.runs[].deliveryTracked`, and takes the same fix for the same reason: a
marker on records created from now on, and a quiet backfill for everything
before. Without it, the first upgrade marks every conversation in the namespace
unread.

*Consequence for deepcopy:* `ThreadBinding` stops being a plain-copy type
(`*out = *in`), so `ConversationStatus.DeepCopyInto`'s `copy(*out, *in)` over
`Threads` becomes a per-element loop. `controller-gen` regenerates both; the
change is only worth naming because a hand-edited deepcopy here would alias the
`*metav1.Time` across copies.

### 2. `POST /channel/read` is a new, optional adapter contract verb

```
POST /channel/read
{"channel":"console","reads":[{"threadId":"…","readAt":"2026-08-13T10:12:04Z"}]}
->
{"results":[{"threadId":"…","outcome":"marked"|"skipped"|"failed","reason":"…"}],
 "marked":N,"skipped":N,"failed":N}
```

Adapter-token auth and the same channel scope check every other `/channel/*`
route uses (`stateChannel` → `scopeAllows(r, ch.Spec.Adapter)`), so an adapter
can only report reads for channels it serves.

*Why `threadId` and not a conversation name:* `/channel/inbound` addresses a
thread, an adapter knows thread ids and not conversation names, and the mapping
from one to the other is the manager's to own. Naming conversations here would
hand adapters a Kubernetes identifier they have no other reason to hold.

*Why a batch:* the console marks a selection, and one HTTP call per row is a
worse shape than one call with a bounded list. Bounded at 50, server-enforced,
for the reason bulk close is: the blast radius equals one screen of
conversations. Per-item outcomes, because a mixed batch is the normal result and
a single verdict cannot carry the reasons.

*Why not `/channel/inbound` with a typed non-message:* inbound means "a person
said something"; the router's job is to turn that into an input. A read receipt
is bookkeeping about a thread, is not attributable to a message, and would have
to be intercepted and discarded on the reply path — an interception is what
`/close` needs *because* it is genuinely text a person typed. Overloading inbound
would make "did this create an input?" depend on a field.

### 3. The watermark is monotonic and clamped to the manager's clock

A report with `readAt` at or before the stored value is `skipped` (not an error,
not a write). A report ahead of the manager's own `now` is clamped to `now`.

Both directions matter. Without monotonicity, two browsers racing — one showing a
stale page — would un-read a thread the other just cleared. Without clamping, one
client with a skewed clock marks all future activity read forever, and nothing
that arrives later is ever new again. The clamp uses the manager's clock because
it is the same clock that stamps `status.lastActivity`, which is what the
watermark is compared against.

### 4. Unread is `lastActivity > readAt`, evaluated per thread

For a conversation with a binding on channel C:

| State | Reading |
|---|---|
| binding has no `readTracked` | **read** — predates the mechanism |
| `readAt` unset | **unread** — bound and never seen |
| `lastActivity` after `readAt` | **unread** |
| otherwise | **read** |
| no binding on C at all | **not unread** — C has no thread here |

*Why `status.lastActivity` and not the newest delivered run:* the list already
sorts on `lastActivity` (falling back to creation), so deriving unreadness from
the same field keeps the ordering and the unread mark from ever disagreeing —
unread rows are exactly a prefix of the list. A runs-only rule would miss the
signal card that opens a thread and every relayed sibling-channel message, which
are precisely the things a person has not seen yet.

*The cost, and why it is small:* `lastActivity` also moves when the operator's
own reply is queued, which would re-mark a conversation you just answered. The
detail view keeps its watermark tracking while it is open (decision 6), so the
case only appears if you reply and navigate away in the same second, and it
resolves on the next read.

### 5. The console filters and counts server-side; unread is a filter, not a view

`ConversationFilter` gains `Unread bool` (`?unread=true`), evaluated with the
existing filters against the console's own thread. The list response gains
`unreadTotal`, computed over **all** conversations before any filter is applied,
and `?count=1` returns the totals with no items for the navigation badge.

Counting before filtering is the same rule the topology Display panel already
holds itself to: a count that moved because you narrowed the view would let a
filter hide a backlog without saying so.

### 6. The console reports reads; it does not decide them

- Opening a conversation detail reports its console thread read up to that
  conversation's current `lastActivity`, and re-reports as new activity arrives
  while the view stays open.
- The list's **Mark read** button reports the selected rows, batched, bounded at
  50 — the selection is over the rows on screen, and there is no
  "mark everything matching the filter", for the reason there is no
  "close everything matching the filter".
- The browser never reports a watermark it did not read off a conversation; it
  never sends `now`. The manager clamps anyway (decision 3), but a client that
  invents a timestamp would be marking activity it never rendered.
- Reports are debounced: nothing is sent when the watermark would not advance.

`POST /api/conversations/read` on the console BFF maps names to the console's
own thread ids and calls the manager once. Observed conversations come back
`skipped` with the existing `notJoinedReason` — the same answer bulk close gives,
because it is the same reach boundary.

### 7. Marking read is authenticated and attributed, but not behind the write gate

Every read report is logged against the resolved identity, exactly like a send or
a close. It is **not** gated by `console.write.enabled`.

That gate's declared job is to make the console a strict viewer: the composer and
the new-conversation action disappear because the console must not instruct an
agent or start work. A read watermark does neither — it changes no wiring, no
conversation content and no agent behaviour, and it is bookkeeping *for viewers*.
A read-only console is exactly the install where an unread badge earns its keep,
and one that could show a backlog but never clear it would be broken in the way
this change exists to fix.

The counter-argument is real and worth stating: it is a write to the Kubernetes
API ordered from a browser, and "read-only" is a promise some operators will read
literally. If that reading wins, this becomes one line — route it through
`a.write(...)` like bulk close — and a read-only console then shows unread
without being able to clear it.

## Risks / Trade-offs

- **Write amplification: one status patch per marked thread.** → Bounded at 50
  per request; skipped when the watermark would not advance, so re-opening a
  quiet conversation writes nothing; and merge-patched onto status, which the
  manager already patches on every run.
- **Two operators share one console watermark.** → This is the chosen model, not
  a defect: read is per channel. It is stated plainly in `docs/console.md`,
  because an operator who expects per-person unread would otherwise read a
  cleared badge as a bug.
- **Upgrading without re-applying the CRDs silently drops the field.** → The API
  server prunes unknown fields, so reports would 200 and change nothing, and
  every conversation would read as unread forever. Called out in `CHANGELOG.md`
  as an upgrade step, and pinned by an envtest that round-trips the field.
- **A conversation deleted mid-batch.** → Reported `failed` for that thread with
  a reason; the rest of the batch proceeds, as with bulk close.
- **Backfilling old bindings as read hides genuinely-unseen conversations once.**
  → Accepted, and it is the quieter of the two lies: the alternative announces a
  namespace-sized backlog that nobody can act on. Stated in the changelog so the
  one-time behaviour is not mistaken for a bug.
- **The unread mark is only as good as the adapter reporting it.** → An adapter
  that never reports leaves its threads unread forever; nothing but the console
  renders unreadness, so this is inert rather than wrong. `readTracked` is
  stamped for every channel so the rule stays one rule.
