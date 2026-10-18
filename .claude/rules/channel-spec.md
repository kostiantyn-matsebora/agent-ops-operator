---
paths:
  - "channels/**"
  - "platform/console/*.go"
  - "platform/manager/internal/chat/**"
  - "platform/manager/internal/httpapi/**"
  - "docs/contracts/channel-adapter.md"
---

## The channel adapter specification governs this surface

**`docs/contracts/channel-adapter.md` is the one law for this surface** —
the shipped channel adapters plus the console's adapter surface and the
manager's channel surface.

It states what the manager owes a channel adapter and what a channel
adapter owes the manager. Build and review against it, never against a
shipped adapter's source.

| Obligation | Rule |
|---|---|
| Verify before done | A change on this surface is verified against `docs/contracts/channel-adapter.md` before it is claimed done |
| No unapproved spec edit | `docs/contracts/channel-adapter.md` is NEVER edited without the user's explicit approval |
| A spec change carries conformance | An approved specification change brings every shipped channel adapter — `channels/telegram`, `platform/console` — into conformance in the SAME change, and moves every claimed version. The change is not archivable before that |

**Reaching for the specification itself without approval is a STOP, not an
edit.** Approval is a conversation fact, not something a hook can read —
stop, ask, and report the conflict instead of writing it.
