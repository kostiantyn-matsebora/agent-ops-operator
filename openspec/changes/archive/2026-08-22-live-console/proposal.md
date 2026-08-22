# A live console, not a polling one

## Why

The console already holds an SSE stream for its whole lifetime, and still
reloads its data over HTTP on every change. Worse, it BLANKS while doing it.

The mechanism is one line, repeated thirteen times. A stream event bumps a
counter, the counter is part of the react-query KEY, and a new key is a cache
entry that has never been filled — so `data` is undefined, `isLoading` is true,
and the page swaps its content for a spinner until the refetch lands.

Send a message and three events arrive in a row: your message, the ack, the
agent's reply. That is three refetches of the whole conversation and three
blanks, which is what "the page refreshes for a few seconds" looks like from the
outside.

**The data is already on the wire.** The `message` event carries the complete
message object — the client parses it, throws the payload away, and refetches
the conversation to learn what it was just told. `queues` and `activity` already
do the right thing and apply what they receive.

One query, `useConversations`, works around the blank with
`placeholderData: keepPreviousData`, and its comment describes the failure
exactly. Twelve others do not. That is not twelve oversights — it is a default
that is wrong, being patched one call site at a time.

## What Changes

- **A delta CARRIES its object.** `delta` sends `{type, kind, name}` and nothing
  else, so every consumer must go and ask what changed. It will carry the
  changed object, so a consumer can apply it.

- **The client APPLIES events to the cache** instead of invalidating keys.
  A message is appended to the conversation that owns it. A changed
  conversation, pipeline or source is written into whatever views hold it.

- **Revisions leave the query key.** Keys become stable — `['conversation', name]`
  — so there is no cache miss to blank the page. This is the whole fix: the
  counter-in-the-key pattern is removed, not worked around with
  `placeholderData` at each call site.

- **Refetching becomes the exception, and each one has a reason**: first load,
  a resync (reconnect or an activity gap, where the client has provably missed
  events), and an explicit user action. Two timed refetches survive on stated
  grounds — topology and overview show RATES, which decay whether or not
  anything changed.

- **Every page, not just conversations.** Overview, Queues, Topology,
  Configuration, Conversations and one Conversation all go through the same
  path, so none of them can regress to a spinner independently.

- **The rule is testable**: after first paint, no view may enter a loading state
  because of a stream event.

Not in scope: replacing react-query, changing the transport, or offline support.

## Capabilities

### New Capabilities

- `console-live-updates`: how a loaded console stays current — what a stream
  event carries, how it is applied, when a refetch is still correct, and the
  rule that no view may blank once it has painted.

### Modified Capabilities

- `console-application`: the snapshot-and-stream contract states that snapshots
  are authoritative and the stream carries cursors. It gains what the stream
  carries as DATA, and the requirement that a delta is applied rather than
  used as a trigger.
- `console-live-runs`: run and message updates arrive as content and are applied
  to the view that holds them.

## Impact

| Area | Change |
|---|---|
| `console/api.go` | the delta event carries the changed object |
| `console/ui/src/api/stream.ts` | events carry typed payloads and are dispatched to appliers |
| `console/ui/src/api/hooks.ts` | revisions leave query keys; thirteen queries stop refetching on change |
| `console/ui/src/api/*` | a cache-apply layer, one place, per kind |
| `console/ui` tests | a test per page asserting no loading state follows a stream event |

No CRD change, no chart value change, no manager change. The console's own BFF
sends more in an event it already sends.
