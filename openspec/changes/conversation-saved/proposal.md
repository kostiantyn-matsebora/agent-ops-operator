## Why

A conversation worth keeping — a diagnosis, a runbook an agent wrote, an incident
timeline — is indistinguishable from the rest. Autoclose and autodelete reclaim it
on the same clock as a throwaway chat, and nothing in the console says "this one
matters". Operators need to mark a conversation as kept.

## What Changes

- A conversation can be **Saved**: a mark the operator sets and clears, independent
  of its phase. A Saved conversation can be Idle, Working or Closed.
- `Conversation.status.savedAt` records when, written only by the manager.
- Two contract verbs, `POST /channel/conversations/{name}/save` and `.../unsave`,
  and the thread commands `/save` and `/unsave` that reach them. Both are
  idempotent and follow accept-and-process.
- A Saved conversation is **never reclaimed automatically**: autoclose skips it and
  autodelete skips it. A manual `delete` is refused until it is unsaved. Closing by
  hand stays allowed — closing is inert, not destructive.
- A member of a coordination takes its retention from its uncaused root, and cannot
  be saved on its own.
- The console shows a Saved mark on a row and the thread header, offers Save/Unsave
  in the row menu, and filters the list to Saved.
- The state read surface carries `savedAt` on a conversation.

## Capabilities

### New Capabilities
- `conversation-saved`: the mark, its verbs and commands, who may set it, what it
  protects, and how a coordination's members inherit it.

### Modified Capabilities
- `conversation-housekeeping`: autoclose and autodelete never act on a Saved conversation.
- `console-quick-actions`: the row menu gains Save and Unsave.
- `state-read-surface`: a conversation read carries `savedAt`.

## Impact

- Code: `platform/manager/api/v1alpha1` (status field, regenerate deepcopy and
  `chart/crds/`), `internal/httpapi` (two verbs), `internal/chat` (two commands),
  `internal/controller` (autoclose/autodelete guard), `internal/state` (record
  field, every backend), `platform/console` (menu, mark, filter).
- Reference docs: `docs/concepts.md` (the field), `docs/contracts.md` (the two
  endpoints and the accept table), `docs/console.md`, `docs/CHANGELOG.md`;
  regenerate via `docs-generate.py`.
- Adopter site: `docs/console-guide.md` (keeping a conversation), the
  housekeeping/retention page under `docs/guides/` and `docs/configuration.md`
  where the autoclose and autodelete values are described; re-run `screenshots`
  and `demo`.
- No new chart value, no RBAC change: the manager already patches Conversation status.
