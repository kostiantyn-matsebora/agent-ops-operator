## Why

The console's conversation list sorts by last activity and says nothing about
what has been *looked at*. After an event storm an operator has no way to tell
the three conversations that arrived while they were away from the four hundred
they already worked through — the only signal is an age column, and age is not
attention. Every triage surface that carries a backlog has an unread mark; this
one does not, so the list gets re-read from the top every time.

Nothing durable records it either: a viewer that answers "what is new" from
browser memory answers differently in every browser and forgets on every reload.
The `Conversation` CR is where the rest of a conversation's record already
lives, and the read mark belongs beside the thread it is about.

## What Changes

- **`Conversation.status.threads[]` gains a read watermark** — `readAt`
  (the newest activity this channel's thread has been seen up to) and
  `readTracked` (this binding was created after the mechanism existed).
  Read state is **per thread, therefore per channel**: reading a conversation in
  Telegram does not mark it read in the console, and vice versa. That is the
  point of putting it on the binding rather than on the conversation.
- **A new channel adapter contract verb, `POST /channel/read`**, by which an
  adapter reports "this thread has been seen up to T" for up to 50 threads at a
  time. **The manager writes the status**, exactly as it already does for
  `POST /channel/channels/{name}/status`. The console still performs no
  Kubernetes write.
  - The watermark is **monotonic** and clamped to the manager's own clock: a
    stale or skewed client can never un-read a thread, and can never mark future
    activity read.
  - Reporting is **optional per adapter**. An adapter that never reports leaves
    its threads unread, which is inert for every surface that does not render
    unreadness.
- **The console renders unreadness**: unread rows are marked in the conversation
  list, an *Unread only* filter narrows the list server-side, and an unread count
  rides in the response and on the navigation.
  - **Unread is a property of a thread the console holds.** An *observed*
    conversation — one with no console thread — is never unread, because the
    console has no watermark on it and no standing to call it new. This is the
    same reach boundary bulk close already draws.
  - The unread count is computed **before** the other filters, so it never moves
    because a filter hid something.
- **Bulk mark-as-read over the selected rows**, bounded at 50 — the list page
  size, the same blast radius and the same server-enforced bound as bulk close.
  Opening a conversation marks its console thread read automatically, and keeps
  the watermark tracking while the detail stays open.
- **Threads bound before this change are treated as read.** A binding with no
  `readTracked` marker predates the mechanism and cannot be told apart from one
  nobody read, so it is backfilled quiet — the same rule, for the same reason,
  as `status.runs[].deliveryTracked`. Without it, upgrading shows every
  conversation in the namespace as new.

## Capabilities

### New Capabilities
- `conversation-read-state`: the per-thread read watermark on `Conversation`
  status, its monotonic and clamping rules, the `POST /channel/read` contract
  verb the manager serves for it, and the backfill rule for bindings that
  predate the mechanism.
- `console-unread`: the console's unread surface — how unreadness is derived for
  the console's own thread, the server-side unread filter and pre-filter count,
  the selection-scoped bulk mark-as-read, and marking read on open.

### Modified Capabilities
- `channel-adapter-contract`: gains one optional inbound verb. An adapter that
  does not implement it stays fully conformant.
- `console-live-runs`: the conversation list gains an unread filter alongside the
  existing server-side filter set, and list rows carry their read state.

## Impact

- **CRD** — `api/v1alpha1/conversation_types.go` (`ThreadBinding` gains two
  fields; it stops being a plain-copy deepcopy type), regenerated deepcopy and
  `chart/files/crds/`. Additive and optional; **the CRDs must be re-applied on
  upgrade** for the field to be persisted.
- **Manager** — `internal/httpapi/server.go` (one route and handler),
  `internal/chat/ops.go` (stamp `readTracked` on bindings it creates). No RBAC
  change: the manager already patches `Conversation` status.
- **Console** — `console/adapter.go` (report reads upstream),
  `console/conversations.go` + `convapi.go` + `api.go` (unread derivation,
  filter, count, batch endpoint), and the conversations list and detail pages in
  `console/ui/`. Console RBAC is unchanged and stays read-only.
- **Chart** — manager and console image tags, chart version.
- **Docs** — `docs/concepts.md` (the CRD field), `docs/contracts.md` (the verb),
  `docs/console.md` (the surface and its per-channel semantics),
  `CHANGELOG.md` (re-apply the CRDs).
- **Not affected** — `channel-telegram` and every other adapter: the verb is
  optional and nothing calls it on their behalf.
