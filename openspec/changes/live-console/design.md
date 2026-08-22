## Context

See `proposal.md` — Why. Four facts about the code as it stands.

- **The stream is already there**, open for the app's lifetime, carrying
  `resync`, `delta`, `activity`, `queues` and `message`.
- **Two of the five already do the right thing.** `queues` and `activity` apply
  what they receive. `delta` and `message` do not.
- **`delta` carries `{type, kind, name}`** — no object. `message` carries the
  whole message and the client discards it.
- **Thirteen queries fold a revision into their key.** That is the blank: a new
  key is a cache entry that was never filled, so `isLoading` is true and the
  view renders a spinner.

## Goals / Non-Goals

**Goals**

- After first paint, no view enters a loading state because something changed.
- One applier per kind, shared by every view that holds that kind.
- Every refetch that remains names its reason at the call site.

**Non-Goals**

- Replacing react-query. It is a good cache; it was being used as a
  fetch-trigger.
- Changing the transport. SSE is correct and already resumable.
- Offline support, optimistic mutation, or client-side persistence.

## Decisions

### D1. The delta carries the object, and the BFF already has it

`console/api.go` sends `{type, kind, name}` from a watch it is already running,
which means it holds the object at that moment. Sending it costs one
serialisation and removes one request per change per open browser.

*Alternative rejected:* keep the delta thin and have the client fetch just that
object. It is still a request per change, and it introduces an ordering problem
the thin delta cannot solve — two changes to one object can be fetched out of
order, and the older answer wins.

Size is bounded by what the console already serves per snapshot; a delta is one
object where a snapshot is all of them.

### D2. Appliers live in one place, keyed by kind

A single module maps `(kind, type, object)` onto cache writes. Views do not
subscribe to events at all — they read the cache, and the cache is what changes.

*Why not per-view subscriptions:* two views holding one object would each
implement the same update, and the day they disagree is the day the list and
the detail show different things. One applier cannot disagree with itself.

### D3. Revisions leave the query key

This is the fix. Keys become stable — `['conversation', name]`,
`['conversations', filters]` — and the cache is updated by writes rather than
invalidated by identity changes.

`placeholderData: keepPreviousData` is then unnecessary rather than required. It
stays only where a key legitimately changes on user input, such as switching
pages or filters, which is what it is actually for.

*Alternative rejected:* add `placeholderData` to the other twelve queries. It
hides the blank without removing the refetch — every change still costs a
request, and the next query somebody writes has the bug again because the
default is still wrong.

### D4. Refetching keeps four reasons, each stated where it happens

First load; resync (reconnect, or a manager-reported activity gap); an explicit
user action; and a value that decays with TIME rather than with change.

The fourth is the one worth naming: topology and overview show RATES. A rate is
not wrong because something changed, it is wrong because time passed, and no
event announces that. Those keep their timers and say so.

### D5. Correctness rests on the snapshot staying authoritative

Applying deltas is an optimisation over re-reading, and it is safe because a
resync replaces the applied state wholesale. An applier that is ever wrong is
corrected at the next reconnect rather than persisting — which is the same
property the existing "missed event costs staleness, never a wrong screen" rule
already relies on.

### D6. The rule is a test, not a convention

A per-page test asserts that after first paint, delivering a stream event
produces no loading state. Conventions of this kind decay one call site at a
time — that is precisely how twelve queries came to differ from the one that
had it right.

### D7. The cache is bounded in TIME, not held for the tab's lifetime

Applying events means a cached view stays correct without being re-read — which
also means it would stay RESIDENT indefinitely if nothing evicted it. A
conversation opened once an hour ago should not still cost memory.

Two settings, both explicit rather than inherited:

- **Eviction of unused data** — a view's data is released once it has been off
  screen for a bounded idle period. React-query calls this `gcTime`; it applies
  only to data no component is holding, so nothing on screen is ever collected
  from under a reader.
- **Freshness on remount** — returning to a view after that period loads fresh
  instead of rendering whatever was last applied. Because the previous data is
  either gone or shown while a background load runs, this never blanks.

Five minutes for eviction and a minute for freshness are the starting points:
long enough that flicking between two pages costs nothing, short enough that a
tab left open all afternoon is not carrying every conversation somebody glanced
at. They are stated in ONE place with this reasoning beside them, so changing
them is a decision rather than a discovery.

**Nothing is persisted.** No `localStorage`, no IndexedDB, no service worker
cache. The console holds a snapshot of cluster state and a transcript of what
agents said, and writing either to a browser's disk is a durability promise this
component has no business making — it would also survive a logout.

The bound is about MEMORY. Correctness is the resync rule's job: applied state
is replaced wholesale whenever the client may have missed an event, so a cache
that lives longer is not a cache that drifts further.

## Risks / Trade-offs

| Risk | Mitigation |
|---|---|
| **An applier writes a shape the snapshot would not have produced**, and the view renders something impossible. | The applier writes the object the BFF sent, in the same shape the snapshot serves. Pinned by a test comparing an applied cache entry against a fetched one for the same state. |
| **A delta is missed and the cache silently drifts.** | Unchanged from today: a gap or reconnect forces a resync, and the snapshot is authoritative. Applying does not weaken it. |
| **Larger events on a busy install.** | One object per change, against a snapshot of all of them per change today. Strictly less traffic. |
| **A view forgets to read a newly applied kind.** | Views read the cache, not events, so a newly applied kind reaches every reader of that cache entry without per-view work. |
| **Deletion.** | A delta carries its type. A delete removes the object from the views that hold it, rather than leaving a row nothing refetches away. |
| **Applied data resident forever**, because nothing re-reads it and nothing drops it. | Explicit eviction for off-screen data, and a freshness bound on remount — see D7. Correctness is unaffected either way; this is a memory bound. |

## Open Questions

- **Whether the activity ring should apply the same way.** It already appends
  from the stream, so it may need nothing — worth confirming rather than
  assuming while touching the same file.
