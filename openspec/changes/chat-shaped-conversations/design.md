# Design: chat-shaped-conversations

See `proposal.md` for why. The prototype is `prototype/` in this change,
board by board, and `prototype/README.md` maps file to board.

**The prototype is the reference for COMPOSITION and the specs are the
reference for BEHAVIOUR.** Geometry, timings, spacing and wording are read
from the prototype files and are written down nowhere else.

## Context

- The list (`pages/Conversations.tsx`) and the detail (`pages/Conversation.tsx`)
  are two routes sharing nothing but the react-query cache. Every row and
  every detail is already LIVE: the stream applies CR deltas and messages to
  what the browser holds (`api/apply.ts`), so no polling is added here.
- Unread today is `ThreadBinding.unread(lastActivity, reader)` in the
  console's `conversations.go`, mirroring the manager's rule. The manager
  stamps `lastActivity` at dispatch (`httpapi/server.go`), at work done and
  at reopen, and nowhere on an input. The dispatch stamp is the false
  unread.
- The transcript is `mergeTranscript` in the console's `transcript.go`: the
  live buffer over `status.runs[]`. Every MESSAGE is in the record. Acks and
  notices are only in the buffer and vanish on restart.
- `POST /channel/read` reports `{conversation, readAt}` per reader key. The
  manager skips a report at or before the stored mark.
- The console persists nothing in the browser today, by a documented rule in
  `docs/console.md` and `api/queryClient.ts`. That rule guards CORRECTNESS
  state, which a resync replaces wholesale.
- `coordinated-agents` (in flight) adds `spec.causedBy {parent, entry}` —
  the immediate parent, one hop, nesting to any depth — `spec.coordinatorRef`
  on a conversation whose entry point is a Coordinator, `status.brief`,
  `status.closeReason`, `status.escalatedAt` and a per-level `status.budget`.
  Only the uncaused root ever opens a human thread: a nested coordinator's
  escalation closes its own conversation and reaches its parent as an
  ordinary result.

## Goals / Non-Goals

**Goals:**

- One view, one URL scheme (`/conversations/:name` keeps working).
- The unread count is restart-safe and never counts the reader's own turn.
- Every existing action survives with its existing rule.
- The tree renders at any depth, following `causedBy` one hop at a time to
  the uncaused root.

**Non-Goals:**

- No CRD field, no contract change beyond the read verb's rewind form, no
  chart value.
- No app-wide navigation redesign. The icon rail is PatternFly's collapsed
  sidebar on this view, not a new component.
- No change to what Telegram or any other channel considers unread. The
  manager's thread rule stays as the transport-wide answer.
- No mobile application. The narrow-window layout is the responsive
  fallback, and `mobile-native-apps` stays its own change.

## Decisions

### D-A — One view under `pages/chat/`, composed of named parts

- `ChatView` owns the four columns and the URL. `Inbox`, `ConversationList`,
  `ConversationRow`, `ThreadPane`, `Timeline`, `Composer`, `QuickChips`,
  `SelectionBar`, `RowMenu`, `Splitter`.
- `Conversations.tsx` and `Conversation.tsx` are deleted, and their tests
  move with the parts they covered. `App.tsx` routes both paths to the view.
- Alternative rejected: keep the table page and add a drawer. That is
  direction B, not chosen.

### D-B — Splitters are ours, and layout is the one thing the browser keeps

- PatternFly 6 has a resizable Drawer and no three-pane splitter. `Splitter`
  is two pointer listeners on a 6px handle, `aria-orientation="vertical"`,
  keyboard `←`/`→` in 16px steps, `Enter` resets.
- Defaults and the collapsed strip width are the prototype's. Minimum widths
  are 200px for the inbox, 280px for the list, 480px for the thread.
- The values live under one `localStorage` key, `agentops.console.layout`,
  read and written inside `try`/`catch`. The "nothing is persisted" rule is
  restated in `docs/console.md`: no CONVERSATION state is persisted, layout
  preferences are.
- Alternative rejected: a server-side per-reader preference. It would put a
  browser's pane width on a Conversation-adjacent object for no reader
  benefit.

### D-C — The count is computed in the console's Go side, per row, from the record

- `conversations.go` gains `unreadCount`, `lastMessage {kind, sender, text}`
  per summary, computed for the console thread and the requesting reader.
- Counted kinds: `signal`, `agent`, `relay`. Not counted: `ack`, `local`,
  listings, refusals, run events. Phase changes are not messages at all.
- The source is `status.runs[]` — the record holds every counted kind: a
  run's inputs (signal and relay origins with their times) and its result
  (the agent, at the run's end) — plus the live buffer for messages a run
  has not yet recorded. Ten runs are retained, so the cost per row is
  bounded and no transcript is fetched for the list.
- A member conversation has no console thread and gets no count. Its result
  arrives on the root as an input and is counted there.
- Alternative rejected: counting in the manager. The manager's rule is
  per-thread and transport-neutral, and a chat surface's idea of "a message
  a person must read" is the surface's.

### D-D — What "read" reports

- Opening reports `max(newest counted message time, lastActivity)`. Both are
  read off the conversation's own state, never the browser's clock. The
  second term keeps the report monotone across a dispatch that stamped
  `lastActivity` after the newest message.
- `stampRead` after a send is unchanged.

### D-E — Mark unread is a reader-scoped rewind on the read verb

- `POST /channel/read` entries gain `rewind: true`. The manager sets the
  named reader's entry to `readAt` regardless of the stored value, clamps
  to now, never touches the channel-wide mark, and refuses an entry with
  `rewind` and no reader.
- The console sends the newest counted message's time minus one
  nanosecond, so exactly that message counts again.
- Bounded at 50 names and attributed exactly as mark read.
- Alternative rejected: a client-side "unread" set. It would not survive a
  reload and would differ per device.

### D-F — The tree is derived client-side by following `causedBy` to the uncaused root

- `tree.ts` builds parent chains from the conversation snapshot: each
  conversation's `causedBy.parent`, followed until a conversation with none.
  The list groups every descendant under that uncaused root. Depth is the
  chain's length, and the row's indent, the header chain and the nested
  cards all read it.
- A conversation with `coordinatorRef` is a coordinator's root at its own
  level, so its row carries the caret, member count, turn and deadline from
  its own `status.budget`. Budgets are per level and never summed here.
- `lastMessage` on any parent includes its members' results, since they are
  inputs on it.
- The escalation divider is placed at `status.escalatedAt`, which only the
  uncaused root carries. A nested coordinator's escalation arrives at its
  parent as a result and renders as that member's result card, marked as an
  escalation. Before the uncaused root escalates, its pane is read-only
  with the reason from the spec.
- Alternative rejected: a server-side tree endpoint. The snapshot already
  holds every conversation the list shows, and a chain walk over it is
  linear.

### D-G — Inline events come from the activity events already on the detail

- `ConversationDetail.events` already carries the run-shaped activity
  kinds. `Timeline` interleaves them with messages by time and folds each
  run's group to its first line. The activity gap marker stays.

### D-H — Quick chips read the vocabulary

- Start chips are the `pipeline` and `coordinator` entries of the vocabulary
  the console already fetches for the header. Choosing one opens
  `NewConversation` with `/<name> ` prefilled.
- Thread chips are the vocabulary's `thread`-position entries. Choice chips
  are `message.choices[]`, already on the wire.

### D-I — Selection mode and the bar

- `Select` in the list header, or `⌘`-click, shows the checkbox column and
  the bar. The bar carries the same actions, with the same server-side
  rules, that the toolbar carries today. `Esc` clears.

### D-J — Where the prototype is deliberately NOT the target

- **The masthead and the rail.** The prototype draws its own chrome. The
  view keeps the app's masthead and collapses the PatternFly sidebar to its
  icon form, with the hamburger to expand it.
- **Colours and fonts.** The prototype carries hex literals and Google
  Fonts. The view uses `theme.css` tokens and the vendored Red Hat fonts.
  Every status is still carried by a word or a shape.
- **Annotation text.** Lines such as "the ack is presence, not a message"
  and "remembered per browser" explain the board and are not UI copy.
- **Fixture names.** The prototype's conversations and people are
  placeholders. The screenshot fixture is the curated one.
- **Toasts.** The prototype draws one. The view uses PatternFly's alert
  group, so it stacks and dismisses like every other notice.
- **The navigation badge.** The prototype shows counts on the rail. The
  view keeps the badge on the Conversations navigation item.
- **The pane mid-drag.** `C-collapsed.html` shows a width readout on the
  handle. The view shows it only while dragging.

## Risks / Trade-offs

- [The count reads `status.runs[]` per row on every list request] → the
  record is capped at ten runs, the computation is a linear scan, and the
  summary is cached per `resourceVersion` and reader key.
- [A message the buffer holds and the record does not yet, right after
  work done] → the buffer is merged for the row too, so the count moves
  with the message and settles when the record lands.
- [A reader can rewind into a message the manager clamps forward again on
  the next open] → opening reports only when the watermark would advance,
  and a rewound reader's mark is behind, so the first open clears it. That
  is the intended cycle.
- [A `causedBy` naming a conversation the snapshot does not hold] → the
  chain stops there and the conversation is shown at the top level with a
  marker that its parent is missing, never dropped.
- [The first browser persistence in the console] → one key, layout only,
  guarded reads. Documented as the exception it is.
- [Two changes touching the console for coordination] → this change owns
  the view. `coordinated-agents` keeps the kinds and the inventory rows.
  Its phase 5 is expected to shrink to that by its own update, and the two
  delta specs are written to agree so either archive order folds cleanly.

## Migration Plan

- Ships as a console image. No CRD, no chart value, no data migration.
- The read verb's `rewind` field is additive. An older manager ignores it,
  and the console then hides mark unread after the first refused rewind.
- Rollback is the previous console image. A stored layout key is harmless
  to the old build, which never reads it.

## Open Questions

- Whether `Mine` should also match conversations the reader's identity was
  mentioned in by a relay. It does not change the specs, the approach or
  the tasks, and can be widened later.
