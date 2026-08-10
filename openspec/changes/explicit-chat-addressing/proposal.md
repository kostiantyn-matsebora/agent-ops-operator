# Proposal: explicit-chat-addressing

## Why

A bare message on a chat surface is routed by an invisible fact: which Pipeline happens to list that surface's `SignalSource`. Nothing on screen says who will answer, and the person typing cannot find out. That implicit default is also the sole reason a chat source must have exactly one claimant — `sourceConflicts` puts every later Pipeline listing the same source at `Ready=False`, which in turn drops it from the `/agents` listing, because "who answers by default" would otherwise have two answers.

The fix is to stop having a default nobody can see. If reaching an agent is always explicit, the exclusivity rule loses its purpose on chat sources, several agents can serve one surface as equals, and the surface can show what is available instead of expecting people to know.

The `/agents` command already lists Ready pipelines, but it is a thing you must know to type. A `/` typeahead in the console composer puts the same list where the ambiguity actually occurs.

## What Changes

- **Ambiguous bare messages are refused, not guessed.** On a channel's general surface:
  - Exactly ONE Ready Pipeline lists the chat source → a bare message goes there, as today. Nothing is ambiguous, so nothing needs a prefix, and single-agent installs are unaffected.
  - TWO OR MORE list it → the message opens no conversation and is answered with the available pipelines and the `/<pipeline> <task>` form.
  - NONE list it → unchanged: `Wired=False`, drop reason back to the surface.
- **BREAKING (behavioural)**: an install that today relies on the oldest claimant answering bare messages while a second Pipeline sits at `Ready=False` will, after this change, have both Pipelines Ready and bare messages refused as ambiguous. The `CHANGELOG` entry names the fix — drop the source from every Pipeline but the intended default.
- **Chat sources stop being exclusive.** `sourceConflicts` applies to non-chat sources only. An alert source keeps exactly one claimant — one alert, one investigation, and no prefix exists to disambiguate. A chat source may be listed by any number of Ready Pipelines, which now means "I serve this surface", not "I own this inbox".
- **`signalSourceRefs` on a chat source changes meaning**, and the docs must say so: it is what makes a surface wired and a Pipeline addressable from it, no longer a claim of exclusive ownership.
- **A `/` typeahead in the console composer** listing Ready pipelines with their profile, filtered as you type, inserting `/<name> `. The console already list/watches `pipelines`, so this needs no new RBAC, no new manager endpoint, and no CRD field.
- **`/agents` stays the universal fallback** for surfaces with no typeahead. On Telegram it remains the discovery path; wiring the native command menu is noted as a follow-up rather than done here, because the pipeline list would have to reach the adapter and adapters read no CRs.

Unchanged, and load-bearing: replies **inside** a conversation thread carry a `threadId`, arrive via `/channel/inbound`, and never travel the signal path. They need no prefix and are untouched. Breaking that is the obvious way to implement this wrong.

## Capabilities

### New Capabilities

- `chat-addressing-discovery`: the `/` typeahead — what it lists, where its data comes from, and how it degrades on surfaces that cannot offer it.

### Modified Capabilities

- `chat-signal-origination`: a bare general-surface message is routed only when exactly one Ready Pipeline serves the source; otherwise it is refused with the choices.
- `pipeline-model`: source exclusivity narrows to non-chat sources; several Pipelines may serve one chat surface.
- `signal-source-model`: what listing a chat source means, and that its Ready-pipeline count is what decides bare-message behaviour.

## Impact

- **Operator**: `internal/httpapi/signals.go` (`routeChatSignals` — the bare-message branch), `internal/chat/pipelines.go` (a "pipelines serving this source" lookup beside `PipelineForSource`), `internal/controller/pipeline_controller.go` (`sourceConflicts` skips chat sources).
- **Deciding "is this a chat source"** needs a rule the reconciler can apply. The signal `kind` is a property of arriving signals, not of the `SignalSource` object, so this needs settling — carried as the design's first open question.
- **Console**: composer typeahead in `console/ui/src/pages/Conversation.tsx` plus a small BFF listing over the existing cache; no RBAC change.
- **Docs**: `docs/concepts.md` (what listing a chat source means), `docs/console.md` (the typeahead), `CHANGELOG.md` (the behavioural break and its fix), `CLAUDE.md` (claiming vs addressing, and the narrowed exclusivity).
- **Enables** `ha-bundle` to list the console source on both its Pipelines with both Ready — the concrete case that surfaced this.
- **Non-goals**: Telegram's native command menu; per-surface default overrides; changing thread replies; any new CRD field.
