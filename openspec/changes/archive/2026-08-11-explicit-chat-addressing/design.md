# Design: explicit-chat-addressing

## Context

Two mechanisms decide who answers a chat message, and only one of them is visible.

- **Addressing.** `/<pipeline> <task>` is parsed in `routeChatSignals` (`internal/httpapi/signals.go:346-355`) and handed to `Router.HandleCommand`, which resolves the Pipeline with a plain `Get` — no claim check, no Ready check. The comment on the call site says it plainly: an addressed command opens the conversation "on the pipeline it names rather than the one claiming the source."
- **Claiming.** A bare message falls through to `routeSignals`, which routes against whichever Ready Pipeline `PipelineForSource` finds listing the source (`internal/chat/pipelines.go:56-66`) — the OLDEST, by creation timestamp.

The second is invisible from the surface. Nothing tells the person who will answer, and there is no way to ask.

It also costs something structural. Because that default must be single-valued, `sourceConflicts` (`internal/controller/pipeline_controller.go:104-132`) puts every later Pipeline listing an already-listed source at `Ready=False, reason=SourceConflict` (lines 95-98). That Pipeline still answers when addressed — `HandleCommand` never checks Ready — but it drops out of `/agents`, which lists Ready pipelines only (`internal/chat/router.go:178-186`), and reads as broken everywhere Pipelines are displayed.

So a rule about *chat defaults* is enforced by a constraint on *every source kind*, and the cost lands on any adopter who wanted two agents watching one thing. `ha-bundle`, where a user-facing and an admin Pipeline both want the console, is where this surfaced.

## Goals / Non-Goals

**Goals:**

- No message is routed by a fact the person cannot see.
- Whether two Pipelines share a source is the adopter's decision, not a rule the operator enforces.
- Several Pipelines serve one chat surface as equals, all Ready, all listed.
- The single-agent install keeps working exactly as it does now.
- Finding out what a surface can reach is possible from the surface.

**Non-Goals:**

- Telegram's native command menu. It would need the Pipeline list to reach the adapter, and adapters read no CRs — a contract change with its own design.
- Changing thread replies in any way.
- A per-surface configurable default. That would be a new invisible fact, which is what this change removes.
- Reading `pipelineRef` for any wiring decision (see D4).

## Decisions

### D1: Ambiguity is the trigger, not the prefix

Strict-always — every general-surface message must be addressed — was considered and rejected. The stated reason for the rule is that it is not obvious where a bare message goes; with exactly one Ready Pipeline serving the source, it IS obvious, and there is nothing to disambiguate. Strict-always would make every single-agent install type a prefix on every message to prevent an ambiguity that cannot arise.

So the rule keys on the count of Ready Pipelines listing the chat source: one routes, several refuse with the choices, none stays the existing unwired path.

A useful property falls out: an install grows into the requirement. Adding a second agent to a surface is exactly when bare messages become ambiguous, and exactly when the refusal explains the new form.

*Alternative considered:* keep a default and make it explicit — a `default: true` marker on one Pipeline's source ref. Rejected: it re-creates an invisible fact, just one spelled somewhere else, and needs a CRD field this change otherwise avoids.

### D2: Exclusivity is deleted, and signals fan out

`sourceConflicts` is removed entirely, along with the `SourceConflict` condition and the oldest-claimant tiebreak in `PipelineForSource`. A `SignalSource` becomes shareable exactly as a `Channel` already is.

A signal admitted on a source served by N Ready Pipelines therefore produces N conversations, one per Pipeline. That is not a compromise position — it is what listing a source on two Pipelines plainly means, and each conversation is a genuinely different thing: its own profile, its own channels, its own toolsets, its own runtime. Two agents investigating one alert from two angles is a configuration an adopter may want; refusing to let them is the operator inventing policy.

*Alternative considered and rejected:* narrowing exclusivity to non-chat sources, so an alert would keep exactly one claimant. It survives the "who investigates this" objection but fails a simpler test — it needs the reconciler to distinguish a chat source from an alert source, and `kind` is a property of an arriving signal, not of the `SignalSource` object. Every candidate handle (a field on `SignalAdapter` beside `configSchema`, a field on `SignalSource.spec`, inference from grouping defaults) buys one `if` in one reconciler at the price of a new declaration every adapter author or installer can get wrong. Deleting the rule needs no handle at all.

Consequences to keep straight: listing a source stops meaning "I own this inbox" and starts meaning "I watch this". It is what makes the source wired at all, and what makes the Pipeline appear in a chat surface's listing. It grants no exclusivity and no priority.

### D3: Chat is the one lane that refuses instead of fanning out

Fan-out is right where the consumer of the answer is a system: two investigations of one alert are two useful artifacts. It is wrong where the consumer is a person who asked one question on one surface — two agents answering in two threads is noise, and unlike an alert, the person CAN say which one they meant.

So a bare `kind: chat` signal fans out only when the fan is exactly one. More than one is refused with the list of servers and the addressed form; the refusal is where somebody who has never heard of `/agents` learns the form, at the moment it matters.

This distinction lives entirely in ingest, which knows the arriving signal's `kind` (`isChat`, `signals.go:681`). No reconciler and no CRD field ever has to answer "is this a chat source" — which is what makes D2's deletion cheap rather than a trade.

### D4: `pipelineRef` on Conversation — provenance, never wiring

`Conversation` snapshots the BINDINGS a Pipeline gave it (`profileRef`, `channelRefs`, `toolsets`, `mcpConfigs`) so that re-wiring a Pipeline affects only new conversations. It has carried no reference to the Pipeline itself, and attribution has been INFERENCE (`chat.PipelineForConversation`), returning nil whenever two Pipelines wire identically.

Fan-out makes that untenable in two places at once:

- **Conversation reuse would cross-contaminate.** `routeSignalGroup` finds a reusable conversation by signature-hash label (`signals.go:435`). Two Pipelines fanning out from one source produce two conversations with the SAME signature, so the second Pipeline's next signal would land on the first Pipeline's conversation — and run under the wrong profile with the wrong tools.
- **Attribution goes blank exactly when it matters.** Two Pipelines sharing a source and a profile are indistinguishable to `MatchPipeline`, so the console would show nothing for both.

So the Conversation records the Pipeline that created it. The rule that made the ref suspect is preserved literally: it is written once at creation and NEVER read to resolve wiring — not for the profile, not for channels, not for capabilities, all of which keep coming from the conversation's own materialized fields. It is provenance, and reuse scoping is a provenance question.

*Alternative considered:* folding the Pipeline name into the signature string so the hashes differ per Pipeline. Rejected — it solves reuse but not attribution, hides an identity inside an opaque hash, and changes every existing conversation's hash.

**Legacy conversations** carry no `pipelineRef`. An empty ref matches a reuse candidate ONLY when exactly one Ready Pipeline serves the source — the pre-upgrade state, so grouping continuity is preserved for every install that has not adopted sharing. Once several serve it, empty refs match nothing and each Pipeline opens its own conversation. Nothing backfills the field: inference is what this replaces.

### D5: The typeahead reads the console's existing cache

The console already list/watches `pipelines` (`console/configapi.go:54`, and its chart Role grants `get,list,watch` on the kind). The typeahead is therefore a small BFF listing over data already in memory plus composer UI in `console/ui/src/pages/Conversation.tsx` — no RBAC change, no manager endpoint, no CRD field. Ready-filtered, so it matches `/agents` exactly rather than becoming a second, divergent answer to the same question.

### D6: `/agents` stays, and the refusal teaches

Not every surface can offer input assistance, so the discovery path that needs no client support has to remain. `/agents` already exists and already lists Ready Pipelines; it stays the universal fallback.

## Risks / Trade-offs

- **An install sharing a source silently doubles its work** → two Pipelines listing one alert source now both investigate, where the younger used to sit at `Ready=False` doing nothing. Two runtimes, two LLM bills, per signal. The `CHANGELOG` leads with it; the state is rare by construction, because it reads as broken today.
- **Capacity fills N× faster on a shared source** → `MAX_ACTIVE_CONVERSATIONS` and `MAX_QUEUED_CONVERSATIONS` are unchanged and still bound the system, but a burst on a doubly-listed source consumes the budget twice as fast. The admission gate handles it the same way it handles any burst.
- **A behavioural break on chat surfaces** → both Pipelines Ready and bare messages refused. Fix is one line of wiring or one prefix.
- **The refusal could annoy a two-agent surface where one is obviously primary** → that is the case D1's rejected alternative would serve; if it turns out to matter, a default marker can be added later as an explicit, visible field. Adding it later is cheap; removing an invisible default is what this change is.
- **Two discovery paths could drift** → both filter on Ready and both derive from the same Pipeline list; the tests assert they agree.
- **Typeahead accuracy depends on cache freshness** → the console's cache resyncs on relist and the typeahead is advisory: a stale entry produces an addressed message to an unknown Pipeline, which is already answered with "unknown agent".

## Migration Plan

1. Land the exclusivity deletion, the fan-out, and the chat ambiguity rule together. A source that loses exclusivity while bare messages still resolve by oldest claimant would be a worse state than either end.
2. `pipelineRef` lands in the same release as fan-out — reuse scoping depends on it.
3. Console typeahead can land independently; it changes no behaviour.
4. `CHANGELOG` entry, newest first, leading with the two behavioural breaks and their one-line fixes.
5. Rollback is a revert. `pipelineRef` is additive and ignored by the previous version; no stored state changes shape.
6. Verify live: two Pipelines on one alert source both investigating; two on one console surface both Ready and both listed; bare message refused with the choices; each addressable by name with replies in the right thread; a single-Pipeline surface still answering bare messages.

## Open Questions

- **Should the ambiguity refusal create nothing, or create a conversation on request?** Currently it creates nothing, matching how `/agents` and usage errors behave. A "pick one" affordance in the console could turn the refusal into a choice that then originates — pleasant, but it makes a refusal path stateful, so it is out of scope here.
- **Does `/agents` want the profile alongside each name?** The typeahead will show it; the command currently shows names only. Worth aligning, and cheap.
