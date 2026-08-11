# Proposal: explicit-chat-addressing

## Why

A bare message on a chat surface is routed by an invisible fact: which Pipeline happens to list that surface's `SignalSource`. Nothing on screen says who will answer, and the person typing cannot find out.

That invisible default is propped up by a second rule. A `SignalSource` may be claimed by at most one Pipeline: `sourceConflicts` puts every later Pipeline listing the same source at `Ready=False`, which drops it from the `/agents` listing and makes it read as broken everywhere Pipelines are displayed. Exclusivity exists to keep "who answers by default" single-valued, and it is paid for by every adopter who wanted two agents watching one thing.

Both go. A `SignalSource` becomes shareable exactly as a `Channel` already is — whether two Pipelines watch one source is the adopter's call, not the operator's — and a signal reaching several of them produces one conversation per Pipeline, which is what listing it twice plainly means. Chat is the single exception, because a person is waiting for ONE answer and, unlike an alert, can say which agent they want: addressing is explicit, and the surface shows what it can reach.

## What Changes

- **Source exclusivity is deleted.** `sourceConflicts`, the `SourceConflict` condition, and the oldest-claimant tiebreak go with it. Any number of Ready Pipelines MAY list one source, of any kind, and none of them reports a conflict or loses `Ready`.
- **Signals fan out.** A signal admitted on a source served by N Ready Pipelines produces N conversations — one per Pipeline, each with that Pipeline's own profile, channels and capabilities. Cooldown and signature grouping stay per-source and are evaluated ONCE, before the fan-out: a fingerprint is admitted once and then delivered to each server.
- **Chat refuses ambiguity instead of fanning out.** On a channel's general surface a bare message is the one case where several answers are worse than none — the person wants one reply and can name the agent:
  - exactly ONE Ready Pipeline lists the chat source → the message routes to it, as today;
  - TWO OR MORE → no conversation is created, and the surface is answered with the Pipelines available and the `/<pipeline> <task>` form;
  - NONE → unchanged: `Wired=False`, drop reason back to the surface.
- **No new CRD field is needed to tell the lanes apart.** The refusal lives in ingest, which already has `kind: chat` in hand; no reconciler ever has to decide whether a `SignalSource` is "a chat source".
- **`Conversation.spec.pipelineRef` records the origin.** Provenance ONLY — written at creation, never read to resolve wiring, so the snapshot rules are untouched. It is what scopes conversation reuse per Pipeline once a source fans out, and it replaces the console's guesswork attribution.
- **BREAKING (behavioural)**: a source listed by two Pipelines used to route through the older one while the younger sat at `Ready=False`. It now routes to BOTH — two conversations per signal, two agents, two runtimes. The `CHANGELOG` names the fix: drop the source from every Pipeline but the intended one.
- **BREAKING (behavioural)**: on a chat surface the same install finds both Pipelines Ready and bare messages refused as ambiguous. Same fix, or address the agent by name.
- **A `/` typeahead in the console composer** listing Ready pipelines with their profile, filtered as you type, inserting `/<name> `. The console already list/watches `pipelines`, so this needs no new RBAC, no new manager endpoint, and no CRD field.
- **`/agents` stays the universal fallback** for surfaces with no typeahead. On Telegram it remains the discovery path; wiring the native command menu is noted as a follow-up rather than done here, because the pipeline list would have to reach the adapter and adapters read no CRs.

Unchanged, and load-bearing: replies **inside** a conversation thread carry a `threadId`, arrive via `/channel/inbound`, and never travel the signal path. They need no prefix and are untouched. Breaking that is the obvious way to implement this wrong.

## Capabilities

### New Capabilities

- `chat-addressing-discovery`: the `/` typeahead — what it lists, where its data comes from, and how it degrades on surfaces that cannot offer it.

### Modified Capabilities

- `pipeline-model`: source exclusivity is removed; a signal fans out to every Ready Pipeline serving its source; a conversation records the Pipeline that originated it.
- `chat-signal-origination`: a bare general-surface message is routed only when exactly one Ready Pipeline serves the source; otherwise it is refused with the choices.
- `signal-source-model`: what listing a source means now, and that a chat source's Ready-pipeline count is what decides bare-message behaviour.

## Impact

- **Operator**: `internal/controller/pipeline_controller.go` (delete `sourceConflicts` and the condition), `internal/chat/pipelines.go` (`PipelinesForSource` — every Ready server, replacing the single-claimant lookup), `internal/httpapi/signals.go` (fan-out in `routeSignals`, the ambiguity branch in `routeChatSignals`, per-Pipeline conversation reuse in `routeSignalGroup`), `internal/controller/signalsource_controller.go` (`Wired` names every server).
- **API**: `Conversation.spec.pipelineRef` added (provenance); `SourceConflict` removed from `Pipeline.status` documentation. Deepcopy + CRD regeneration.
- **Console**: composer typeahead in `console/ui/src/pages/Conversation.tsx` plus a small BFF listing over the existing cache; attribution reads `pipelineRef` instead of inferring. No RBAC change.
- **Docs**: `docs/concepts.md`, `docs/console.md`, `CHANGELOG.md`, `CLAUDE.md` (two invariants change wording: one-pipeline-per-source, and conversations-carry-no-pipelineRef).
- **Enables** `ha-bundle` to list the console source on both its Pipelines with both Ready — the concrete case that surfaced this. That change's proposal and design argue against doing so on grounds that no longer hold, and need their own revision.
- **Non-goals**: Telegram's native command menu; per-surface default overrides; changing thread replies; reading `pipelineRef` for any wiring decision.
