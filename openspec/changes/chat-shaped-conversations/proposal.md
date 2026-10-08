# Proposal: chat-shaped-conversations

Prototype: `prototype/` in this change — direction C (`C-rail.html`), its
collapsed variant (`C-collapsed.html`), coordination (`D-incident.html`) and
the shared board (`States.html`: the unread rule, row states, selection mode).
The two directions not chosen (`A-inbox.html`, `B-peek.html`) are kept as the
record of what was compared.

## Why

The Conversations page is a table you leave to read anything. Its unread mark
also fires on the wrong event:

- unread is `status.lastActivity` newer than the reader's watermark
- the manager stamps `lastActivity` when a run is DISPATCHED
- so the question you just typed turns its row unread a second later, with
  nothing behind it but the `🔧 On it…` ack

An operator watching a fleet needs what every chat application gives them.
Switching conversations without a list round trip, filters that mean
something, motion when something arrives, and a badge that counts messages
they have not read.

## What Changes

- **One chat-shaped view replaces the list page and the detail page.** An
  icon navigation rail, an inbox column (scopes with counts: All, Unread,
  Working, Mine and Errored, then every Ready Pipeline and Coordinator, then
  Closed), the conversation list, and the thread pane.
  Switching conversations never leaves the view. `↑`/`↓` move, `Enter` opens,
  `⌘`-click selects. Runs, Graph, Sequence and YAML stay reachable from the
  thread header as secondary views.
- **Panes resize and the inbox collapses.** Two splitter handles (drag,
  double-click resets). The inbox column collapses to a 56px icon strip that
  keeps every badge. Widths and the collapsed state are per-viewer
  conveniences kept in the browser's local storage. This is the first thing
  the console persists in a browser, and it is layout, never conversation
  state — the "nothing is persisted" rule is restated to say so.
- **Unread is counted from MESSAGES, by kind.** For the console's own thread:
  `signal`, `agent` and `relay` messages after the reader's watermark count.
  `ack`, the reader's own messages, phase changes, listings and refusals do
  not. Computed console-side from the merged transcript the console already
  builds. A row carries a COUNT, the navigation badge is the SUM, and the
  unread filter and count-only endpoint answer by the same rule. Acks are not
  in `status.runs[]`, so excluding them is what makes the count survive a
  console restart.
- **Presence is drawn apart from unread.** A pulsing dot on the row's avatar
  while a run is inflight, and a typing row in the thread standing in for the
  ack.
- **Mark unread**, per row and over a selection: an EXPLICIT rewind of the
  named reader's own watermark to just before the newest counted message.
  The manager's monotonic rule keeps refusing a stale client. A rewind is a
  new, reader-scoped verb, refused where the console resolves no reader.
- **Live cues.** A new conversation slides in at the top with a fading tint
  and a toast naming its pipeline. An arriving message fades in. A "New
  messages" divider sits above the first uncounted message and opening
  scrolls to it. A "↓ N new messages" pill appears when scrolled up.
  Autoscroll only when already at the bottom.
- **Run events inline.** Run dispatched, runtime starting, context restored
  and run completed render as muted one-line items inside the transcript,
  from the activity events the stream already carries, collapsible per run.
  The thread header names the bound channels.
- **Quick actions.** Chips to start a conversation with a Ready Pipeline or
  Coordinator (from the vocabulary the console already holds, prefilling
  `/<name> ` in the new-conversation composer), thread command chips
  (`/exit`, `/close`), and the last message's `choices[]` as chips. A row menu
  with mark unread/read, open in new tab, copy link, open incident, reopen,
  exit runtime, close and delete — reduced, for a member row, to open in new
  tab, copy link and open incident, since a member holds no channel binding
  of its own to act through.
- **Every bulk action kept, with its rule.** Page-scoped select-all, Mark
  read, Mark unread (new), Close… with the include-working modal, Delete only
  when the whole selection is closed, per-row Reopen with no bulk form, rows
  held by a finalizer unselectable. Selection mode is entered from "Select"
  or `⌘`-click.
- **The coordination tree, in this view.** Roots show a caret, member count,
  turn and budget. Members nest by indentation to ANY depth — a member that
  coordinates nests its own. Grouped by incident by default, flatten toggle.
  A root's thread is the incident timeline: coordinator turns, member start
  and result lines, an escalation divider, then ordinary chat. A member has no
  console thread, so it is never unread on its own — its result counts on the
  root. Before escalation the root's pane is read-only. Closing or deleting a
  root reaches its members too — see below — and the confirmation says so. The
  header shows the parent chain. **Dependency:** `coordinated-agents`,
  archived into `coordinator-model`, `conversation-provenance`,
  `coordination-escalation` and `coordination-loop`, supplies `causedBy` (the
  immediate PARENT, one hop, nesting to any depth), `coordinatorRef`, `brief`,
  `closeReason`, `escalatedAt` and a per-level `budget`. This change renders
  that tree and closes one gap those specs left open: closing or deleting a
  root from a human surface — the only way a console operator ever acts on
  one — cascades to every live descendant, below.
- **Closing or deleting a root reaches every descendant, for every
  originator.** Today only the coordinator's own MCP `close` verb cascades to
  members — a human's `/close`, the console's bulk close (a fan-out of
  `/close`), and the idle timer each close the ONE conversation they are
  given and stop. `conversation-close`'s own invariant ("one implementation,
  whatever ordered it") is widened to include the cascade: closing a
  conversation closes every live conversation it caused, recursively, with no
  reason required or threaded through unless a coordinator itself issued the
  close. Deleting gains the matching cascade — closing already guarantees
  every descendant is `Closed` by the time the root is, so deleting the root
  deletes every descendant too, skipping any already gone. The confirmation
  names the count for both actions, and a member reached directly (never
  through its root) is refused, naming its parent.
- **Not breaking.** No CRD or chart value changes. Every endpoint the old
  pages used keeps working: two gain fields, the read verb gains the rewind
  form, and the close and delete verbs gain a cascade through `causedBy` —
  a behavior change to the `conversation-close` capability, not a new
  endpoint or field.

## Capabilities

### New Capabilities

- `console-chat-layout`: the four-column view — inbox scopes with counts,
  list, thread, secondary views — switching in place, keyboard navigation,
  resizable panes, the collapsible inbox, and what the browser persists.
- `console-thread-live-cues`: presence drawn apart from unread, arrival
  motion for rows and messages, the new-messages divider, the jump pill, the
  autoscroll rule, the new-conversation toast, and run events inline in the
  transcript.
- `console-quick-actions`: start-with-a-pipeline chips, thread command chips,
  choice chips, and the row menu.
- `console-conversation-tree`: the coordination tree in the list and the
  incident timeline in the thread, at any depth the API states.

### Modified Capabilities

- `console-unread`: unread is a COUNT of messages by kind on the console
  thread, not activity newer than a watermark. The row carries the count,
  the badge is the sum, the filter and count-only endpoint follow the rule.
  Opening reports the newest counted message's time. Mark unread exists.
- `conversation-read-state`: the watermark still never moves backwards for a
  stale report, but a named reader may EXPLICITLY rewind their own entry.
  The channel-wide mark never rewinds.
- `console-live-runs`: the list's filter set gains Mine and the
  coordination-root filter, rows gain the
  unread count and the last counted message, the unread scenario is
  restated by the new rule, and the detail's views become secondary views of
  one thread pane.
- `conversation-close`: closing a conversation now cascades to every live
  conversation it caused, recursively, for every originator — not only the
  coordinator's own MCP verb. Deleting a `Closed` conversation cascades the
  same way. No new field or endpoint: the existing `/close` command, the
  idle timer and the delete verb all gain it.
- `console-coordination-view`: superseded. Its requirements described the
  list/detail pages this change deletes — the grouping toggle, the flat
  default, the old transcript's parent link. `console-conversation-tree`
  (new, below) carries the same substance forward as the one view's own
  behavior, and this capability's delta marks that handoff explicitly rather
  than leaving a published spec describing a page that no longer exists.

## Impact

**Code**

- `platform/console/ui/src/`: `App.tsx` (rail, routes), `pages/Conversations.tsx`
  and `pages/Conversation.tsx` replaced by one view under `pages/chat/`
  (layout, splitter, inbox, list, row, thread, timeline, composer, quick
  chips, selection bar, row menu), `api/types.ts`, `api/hooks.ts`,
  `api/apply.ts` (count fields on applied rows), `theme/theme.css`
  (keyframes). Tests beside each.
- `platform/console/` (Go): `conversations.go` (per-row unread count and last
  counted message from the merged transcript), `convapi.go` (filter and
  count-only by the rule, `mine`, `incidents`, tree grouping, mark-unread
  handler, member-in-selection skip for close AND delete), `adapter.go` (the
  rewind report), `transcript.go` (kind-aware counting helper).
- `platform/manager/internal/chat/router.go` and `coordinate.go`: the
  ordinary close path (`closeConversation`, which `/close` and the console's
  bulk close both reach) cascades through the same `cascadeCloseMembers`
  helper the coordinator's own MCP verb already uses.
- `platform/manager/internal/httpapi/server.go`: `handleConversationDelete`
  cascades through `causedBy` before deleting the named conversation.
- `platform/manager/internal/httpapi/channels.go` and `api/v1alpha1`: the
  read verb accepts a reader-scoped rewind. No CRD field changes.
- `platform/console/ui/screenshots/fixture.ts` and `demo/story.ts`: the
  fixture gains a working-and-read conversation, an incident, and an
  autosolved one, so both assets show the new rules.

**Documents made untrue — reference docs**

- `docs/console.md`: the Conversations, Unread ("no mark as unread", "nothing
  is persisted"), Closing, Reopening and Deleting sections, and "What the
  browser keeps". Closing and Deleting both state the cascade to members.
- `docs/concepts.md`: the read-state section's "a thread is unread when"
  paragraph gains the console's message-kind rule and the rewind. The
  Closing and Deletion sections state that both now cascade through
  `causedBy` for every originator, not only the coordinator's own verb.
- `docs/contracts.md`: `POST /channel/read` gains the rewind form.
- `docs/CHANGELOG.md`: the console image entry, plus a manager entry for the
  close/delete cascade (a behavior change, not a new field).
- `.claude/rules/structure.md`: the console section (the pages it holds).
- `openspec/specs/console-unread`, `conversation-read-state`,
  `console-live-runs`, `conversation-close`, `console-coordination-view`:
  folded at archive.

**Documents made untrue — adopter site**

- `docs/console-guide.md`: the Conversations and Conversation tour entries,
  their captions, and the screenshots `assets/img/console/conversations-*.png`
  and `conversation-*.png` (regenerated by `npm run screenshots`).
- `docs/index.md`: the landing recording and poster under `assets/video/`
  (regenerated by `npm run demo`), since the story walks the conversation
  view.
- `docs/getting-started.md`: the console-first walkthrough where it names the
  list, the unread switch or the detail tabs.

**`coordinated-agents` already archived before this change was applied**

- It shipped tasks 5.3 and 5.4 in full — the incident view this proposal
  once expected it to shrink away from — as the now-published
  `console-coordination-view` capability, on the pages this change deletes.
- `console-conversation-tree` (above) supersedes it rather than coordinating
  with it. The `conversation-close` and `console-coordination-view` entries
  under Modified Capabilities, and their delta specs, already created, carry
  that supersession explicitly.
