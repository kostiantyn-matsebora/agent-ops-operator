## Context

See `proposal.md` for the motivation. Three facts shape the approach:

- The console narrows closed rows **client-side**. The server returns every conversation and `visibleItems` in `model/conversationList.ts` drops closed ones unless `showClosed` is on. Only the count endpoint takes `showClosed`.
- Two looks, classic and workspace, draw one **shared model layer** (`ui/src/model/`). Scopes, counts, `visibleItems`, `selectableNames` and `partitionSelection` live there. Each look keeps its own `showClosed` preference in its own layout store and its own strings.
- The scope is not persisted in either look. Only `showClosed` is.
- Inbox counts come from `accumulateScopeCounts` in `convapi.go`. The code counts unread conversations. The `console-unread` spec says "a sum of unread counts", so the spec has drifted from the code and this change fixes the spec.
- The selection bar counts `selected.size`. Each dialog computes `send.length + memberCount` from the snapshot. Close and delete send only roots, so the three numbers can differ for one selection.
- A change in flight, `console-visual-refresh`, owns the looks and the workspace look. It modifies a different `console-chat-layout` requirement, so the two deltas do not collide.

## Goals / Non-Goals

**Goals:**

- One rule for every inbox count: unread roots in the scope, closed rows excluded except under Closed.
- One word, Closed, for the state in every user-visible string.
- Every number the selection surfaces counts roots.

**Non-Goals:**

- No change to the manager, the CRDs, the close and delete cascades or the `/channel/*` contract.
- No change to the `archived` field on the thread detail response. It is an internal name and renaming it would touch the manager-facing transcript code for no visible gain.
- No change to the `ArchiveIcon`.
- No per-member close, delete or read action.

## Decisions

### Open is a scope kind, not a filter on All

`Scope` in `model/conversationList.ts` gains `open` and loses `all`. Each look's default scope becomes `open`.

- **Alternative: keep `all` and relabel it.** Rejected. A scope called `all` that excludes closed rows has a name that lies.
- No scope is persisted in either look, so no stored `all` value can survive an upgrade.

### Closed rows are excluded by one rule, in one place

`visibleItems` drops `phase == Closed` for every scope except `closed`. The `showClosed` argument, each look's layout key, each look's checkbox and the `showClosed` query parameter on the count request are deleted.

- **Alternative: keep the preference and hide the checkbox on Open and Closed.** Rejected by the requester. A hidden preference that still changes Working and Mine is the inconsistency this change removes.
- The server computes counts with the same rule, so a badge always promises exactly the rows its list shows.
- A stored `showClosed` is never read, so no migration is needed.

### Counts are unread roots, per scope

`accumulateScopeCounts` takes no `closedVisible` argument. For a non-member row:

| Scope | Counted when |
|---|---|
| `open` and the navigation total | `unread ∧ phase≠Closed` |
| `working`, `mine`, `errored`, each Pipeline or Coordinator | its predicate `∧ unread ∧ phase≠Closed` |
| `closed` | `unread ∧ phase=Closed` |

`rootTotal` is deleted, because Open replaces the one badge that used it. `unreadTotal` stays and equals `scopes.open`.

### Selection is over roots, and members are not selectable

`selectableNames` and `selectionState` in the model layer filter members out, so no look renders a checkbox for one and select-all picks roots. `partitionSelection` loses its skip branch, so `send` is the selection. The dialogs compute `memberCount` from the snapshot as they do today and show it on a separate line.

- **Alternative: keep members selectable and exclude them from the count only.** Rejected. A ticked member that does nothing is a control that lies, and it keeps the local "member of P" skip alive.
- The Working warning counts working roots. The member line also names how many of the cascade's members are working, so a run is not abandoned unseen.
- The result banner reports the server's per-root outcomes. The manager reports a close as accepted and does not report cascaded members individually, so the banner does not claim a member outcome.

### One word for the state

The strings change in both looks: the Closed row, the scope title and the navigation panel, the thread banner, the close and batch dialog text and the `/close` chip text. Comments, the `archive` icon name and the `archived` wire field are left alone.

## Risks / Trade-offs

- **[A closed conversation with an unread message is invisible outside Closed]** → Its badge appears only on Closed, which now counts unread roots. This is the requested behaviour, and it means a newly closed conversation can carry a badge the operator must go to Closed to see.
- **[Operators used the checkbox to see closed rows next to open ones]** → That view is gone by design. Closed holds them.
- **[The Closed badge changes meaning]** → It was a total and is now unread-only. The docs table and the screenshots are updated in the same change.
- **[A scenario name in `console-unread` still says "sum"]** → The delta keeps the name so the archive merge matches it, and the scenario text states the new rule.

## Migration Plan

1. Ship the console image. No manager or chart change is needed.
2. A browser's stored `showClosed` is ignored and harmless.
3. Rollback is the previous console image. The API change is additive on the way back, because the older UI sends `showClosed` and the newer server ignores it.

## Counter reference

Every counter the console shows for conversations after this change. The **Change** column marks what differs from today.

| Counter | Where shown | Formula | Change |
|---|---|---|---|
| Row unread badge (`unreadCount`) | Conversation row | Count of messages with `kind ∈ {signal, agent, relay}`, not `InternalRound`, and `at > readAt`. Console thread only, and 0 if not joined. | — |
| Open | Inbox | Roots with `phase≠Closed ∧ unread`. | Replaces All. Was read or unread, now unread only. |
| Nav-rail badge (`unreadTotal`) | Navigation | Roots with `phase≠Closed ∧ unread`. | Same as Open, since closed rows are no longer optional. |
| Working | Inbox | Roots with `phase=Working ∧ unread`. | No `showClosed` term. |
| Mine | Inbox | Roots with `mine ∧ phase≠Closed ∧ unread`. | No `showClosed` term. |
| Errored | Inbox | Roots with `lastRun.status≠succeeded ∧ phase≠Closed ∧ unread`. | No `showClosed` term. |
| Pipeline / Coordinator | Inbox | Roots with `(pipeline=X ∨ coordinator=X) ∧ phase≠Closed ∧ unread`. | No `showClosed` term. |
| Closed | Inbox | Roots with `phase=Closed ∧ unread`. | Renamed from Archive. Was every closed root, now unread only. |
| Run count | Closed row snippet | `len(status.runs)` | — |
| Age | Row | `now − (lastActivity ∥ created)`, floored at 0. | — |
| Member count | Tree row | `len(descendantsOf(items, name))` | — |
| N selected | Selection bar | `count(selected roots)` | Was `selected.size`. Members are not selectable. |
| Close (n) / Delete (n) | Selection bar buttons | `n = count(selected roots)` | Was all ticked rows. |
| Close dialog title | Close dialog | `count(selected roots)` | Was `send.length + memberCount`. |
| Delete dialog title | Delete dialog | `count(selected roots)` | Was `send.length + memberCount`. |
| Member line | Close and delete dialogs | `M = Σ over selected roots of len(descendantsOf(items, root))` | Informational only, not part of the title. |
| Working warning | Close dialog | `count(selected roots with phase=Working)` | Was working rows in the selection. The member line also names working members. |
| N closed / skipped / failed | Close result | Per-root server outcomes. | No local member-skips. |
| N deleted / skipped / failed | Delete result | Per-root server outcomes. | No local member-skips. |
| Select all | List header | Selects all roots on the page that are not already closing or being deleted. | Was every row, members included. |
| Mark read / unread: marked / skipped / failed | Result of mark read or unread | Count of per-root outcomes. | — |
| Mark read sets | Read watermark | `max(newestCounted.at, lastActivity)` → `unreadCount=0` | — |
| Mark unread sets | Read watermark | `newestCounted.at − 1ns` → `unreadCount=1` | — |
| "New messages" divider | Thread | Placed before the first counted message with `at > readAt`. | — |
| "↓ N new messages" | Thread | +1 each time the body-message count grows while scrolled up and no divider is shown. | — |
| Body message count | Thread | `count(messages where kind ≠ ack)` | — |
| "N events" | Run event group | `group.events.length` (shown if greater than 1). | — |
| "new" tag | List row | Name not in the previous snapshot for the same scope. Clears after 4s. | — |
| Agents invoked | Coordinator budget | `agentsInvoked of maxAgents` (or just `agentsInvoked` if no max). | — |
| Turns (budget) | Coordinator budget | `turns of maxTurns` (or just `turns` if no max). | — |
| Turns table title | Run detail | `len(turns)` model calls. | — |
| Tool calls table title | Run detail | `len(toolCalls)` | — |
| Fresh tokens | Run turn | `tokensIn − cacheRead − cacheCreation`, shown as "—" if `tokensIn` is absent. | — |
| Work row queued | Queues | `len(spec.inputs)` | — |
| Inflight age | Queues | `now − inflight.dispatchedAt`, stuck if > 15 min. | — |
| Capacity in use / max / waiting | Queues | From the manager's `/status` `runtimeSlots`. | — |
| Delivery queued / claimed | Queues | Per adapter, from `/status`. | — |
| Delivery stuck | Queues | `adapter-wedged` if `claimed>0 ∧ oldestClaimed>2m`. Otherwise `nothing-claiming` if `queued>0 ∧ oldestQueued>2m`. | — |
| Edge Events | Topology | Count of hops on the edge within the window. | — |
| Edge Errors | Topology | Count of those hops with `status=error`. | — |
| Edge Rate/min | Topology | `Events / window.minutes` | — |
| Edge p50 | Topology | `sorted(latencies)[len/2]`, using only hops with latency > 0. | — |
| Edge max | Topology | `max(latencies)` | — |
| Problems (N) | Overview | Non-True conditions across all kinds, plus pod failures and console-derived findings. | — |
| Workload restarts | Overview | `Σ restartCount` over containers of pods named `<deployment>-*`. | — |
| Adapter "serves" | Overview | Count of Channels or SignalSources whose `spec.adapter` equals this adapter's name. | — |

Definitions:

- **root** = `causedBy.parent = ""`
- **unread** = `unreadCount > 0`
