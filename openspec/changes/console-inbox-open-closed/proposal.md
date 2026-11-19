## Why

Three things in the Conversations inbox read inconsistently:

- **All** hides closed rows behind a "Show closed" checkbox. The checkbox moves every badge but Closed's own, so one count means different things on different scopes.
- The Closed scope is labelled **Archive**, a second word for a state the rest of the UI calls Closed.
- The selection bar and the dialogs count rows, members included, though only roots can be acted on.

## What Changes

- **All becomes Open.** It lists root conversations with `phase != Closed` and is the default scope.
- **"Show closed" is removed everywhere.** The layout preference, the checkbox and the conversations endpoint's `showClosed` parameter go away. Every scope except Closed excludes closed rows. Closed lists only closed rows.
- **Every inbox badge counts unread roots.** Open and Closed join Working, Mine, Errored and the Pipeline and Coordinator scopes. A member never counts. The navigation badge equals Open's.
- **"Archive" becomes "Closed"** in every user-visible string: the inbox row, the thread banner, the close dialog, the `/close` chip and the docs.
- **Selection counts roots.** "N selected", the button suffixes, the dialog titles, the Working warning and the result banner count selected roots. Members cannot be selected. "Select all" picks roots only. The dialogs keep one informational line naming how many members the cascade reaches.
- **The local member-skip outcome disappears.** A member is never in a request, so "member of P — act on P instead" can no longer occur.
- **BREAKING (console only).** `GET /api/conversations` stops reading `showClosed` and stops returning `rootTotal`. The browser-stored `showClosed` key, kept by both looks, is ignored.
- **Both looks change alike.** The classic and workspace looks share one model layer, so the rules live there and each look only draws them.

The full list of counters and how each changes is the table in `design.md`.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `console-chat-layout`: the inbox scope list and order, the removed "Show closed" preference, the default scope, the collapsed strip's badges.
- `console-unread`: the badge counts unread roots, not a sum of messages, and no scope depends on a closed-row toggle.
- `console-conversation-tree`: root and member counts in the close and delete confirmations, members not selectable.
- `console-bulk-close`: selection and confirmation count roots, the Closed wording, the archived-thread banner wording.

## Impact

- `platform/console/ui/src/model/`: `conversationList.ts`, `bulk.ts`, `tree.ts`. Both looks draw their scopes, counts and selection facts from here.
- `platform/console/ui/src/looks/classic/pages/chat/` and `platform/console/ui/src/looks/workspace/`: the inbox or navigation panel, the conversations page, the selection bar, the dialogs, the thread banner and the `/close` chip text, in each look.
- `platform/console/ui/src/api/`: `hooks.ts`, `types.ts`.
- `platform/console/convapi.go`: `handleConversations`, `accumulateScopeCounts`.
- `docs/console.md`, and the console screenshots and demo.
- No manager, CRD, chart or runtime change. No cluster behaviour changes.
