# Design: explicit-chat-addressing

## Context

Two mechanisms decide who answers a chat message, and only one of them is visible.

- **Addressing.** `/<pipeline> <task>` is parsed in `routeChatSignals` (`internal/httpapi/signals.go:348-355`) and handed to `Router.HandleCommand`, which resolves the Pipeline with a plain `Get` — no claim check, no Ready check. The comment on the call site says it plainly: an addressed command opens the conversation "on the pipeline it names rather than the one claiming the source."
- **Claiming.** A bare message falls through to `routeSignals`, which routes against whichever Ready Pipeline `PipelineForSource` finds listing the source (`internal/chat/pipelines.go:56-66`).

The second is invisible from the surface. Nothing tells the person who will answer, and there is no way to ask.

It also costs something structural. Because the bare-message default must be single-valued, `sourceConflicts` (`internal/controller/pipeline_controller.go:104-132`) puts every later Pipeline listing an already-listed source at `Ready=False, reason=SourceConflict` (lines 95-98). That Pipeline still answers when addressed — `HandleCommand` never checks Ready — but it drops out of `/agents`, which lists Ready pipelines only (`internal/chat/router.go:178-186`), and reads as broken everywhere Pipelines are displayed. So the price of an invisible default is that a second agent cannot serve a surface as an equal.

This surfaced concretely in `ha-bundle`, where a user-facing and an admin Pipeline both want the console.

## Goals / Non-Goals

**Goals:**

- No message is routed by a fact the person cannot see.
- Several Pipelines serve one chat surface as equals, all Ready, all listed.
- The single-agent install keeps working exactly as it does now.
- Finding out what a surface can reach is possible from the surface.

**Non-Goals:**

- Telegram's native command menu. It would need the Pipeline list to reach the adapter, and adapters read no CRs — a contract change with its own design.
- Changing thread replies in any way.
- A per-surface configurable default. That would be a new invisible fact, which is what this change removes.
- Any new CRD field.

## Decisions

### D1: Ambiguity is the trigger, not the prefix

Strict-always — every general-surface message must be addressed — was considered and rejected. The stated reason for the rule is that it is not obvious where a bare message goes; with exactly one Ready Pipeline serving the source, it IS obvious, and there is nothing to disambiguate. Strict-always would make every single-agent install type a prefix on every message to prevent an ambiguity that cannot arise.

So the rule keys on the count of Ready Pipelines listing the chat source: one routes, several refuse with the choices, none stays the existing unwired path.

A useful property falls out: an install grows into the requirement. Adding a second agent to a surface is exactly when bare messages become ambiguous, and exactly when the refusal explains the new form.

*Alternative considered:* keep a default and make it explicit — a `default: true` marker on one Pipeline's source ref. Rejected: it re-creates an invisible fact, just one spelled somewhere else, and needs a CRD field this change otherwise avoids.

### D2: Exclusivity narrows to non-chat sources

`sourceConflicts` keeps applying to alert and job sources — an alert carries no prefix, so "who investigates this" must have exactly one answer, and a second claimant would double every investigation. It stops applying to chat sources, where the prefix exists and the bare-message rule handles the rest.

Consequences to keep straight: listing a chat source stops meaning "I own this inbox" and starts meaning "I serve this surface". It is what makes the surface wired at all, and what makes the Pipeline appear in that surface's listing. It grants no exclusivity and no priority.

### D3: Deciding what counts as a chat source — the one genuinely unsettled piece

D2 needs the reconciler to distinguish a chat source from an alert source, and the obvious handle is not available: `kind` is a property of an arriving signal (`kind: chat`), not of the `SignalSource` object. The reconciler validates wiring without seeing traffic.

Three candidates, none free:

1. **Infer from the serving adapter.** A `SignalSource` names an adapter; a chat-originating adapter is a known set (`signal-telegram`, the console's signal identity). Needs a way for an adapter to declare "I originate chat" — plausibly a field on `SignalAdapter` beside `configSchema`, which is already interface metadata. Cleanest, but touches the adapter contract.
2. **Infer from grouping defaults.** Chat sources are already configured distinctly (`cooldownHours: 0`, no signature labels). Rejected on sight: that is a coincidence of configuration, not a declaration, and an alert source configured the same way would silently lose its exclusivity.
3. **Declare it on the source.** An explicit field on `SignalSource.spec`. Honest and simple, but adds a CRD field this change was trying to avoid, and duplicates something the adapter already knows.

Leaning to (1) — the adapter is the thing that knows what it emits, and `SignalAdapter` already carries interface metadata for exactly this kind of question. This wants settling before implementation; the tasks assume (1) and flag it.

### D4: The typeahead reads the console's existing cache

The console already list/watches `pipelines` (`console/configapi.go:54`, and its chart Role grants `get,list,watch` on the kind). The typeahead is therefore a small BFF listing over data already in memory plus composer UI in `console/ui/src/pages/Conversation.tsx` — no RBAC change, no manager endpoint, no CRD field. Ready-filtered, so it matches `/agents` exactly rather than becoming a second, divergent answer to the same question.

### D5: `/agents` stays, and the refusal teaches

Not every surface can offer input assistance, so the discovery path that needs no client support has to remain. `/agents` already exists and already lists Ready Pipelines; it stays the universal fallback.

The refusal message carries the same list. Someone who does not know `/agents` exists learns the form at the moment they need it, which is worth more than documentation.

## Risks / Trade-offs

- **A behavioural break for installs relying on the old default** → an install where a second Pipeline sits at `Ready=False` today will find both Ready and bare messages refused. The `CHANGELOG` entry leads with the one-line fix: remove the source from every Pipeline but the intended default. Rare by construction — that state reads as broken today, so few will have chosen it.
- **D3 is unsettled and D2 depends on it** → the tasks put the chat-source determination first, so implementation does not start against a guess.
- **The refusal could annoy a two-agent surface where one is obviously primary** → that is the case D1's rejected alternative would serve; if it turns out to matter, a default marker can be added later as an explicit, visible field. Adding it later is cheap; removing an invisible default is what this change is.
- **Two discovery paths could drift** → both filter on Ready and both derive from the same Pipeline list; the tests assert they agree.
- **Typeahead accuracy depends on cache freshness** → the console's cache resyncs on relist and the typeahead is advisory: a stale entry produces an addressed message to an unknown Pipeline, which is already answered with "unknown agent".

## Migration Plan

1. Settle D3, then land the reconciler narrowing and the routing rule together — a chat source that loses exclusivity while bare messages still resolve by oldest claimant would be a worse state than either end.
2. Console typeahead can land independently; it changes no behaviour.
3. `CHANGELOG` entry, newest first, leading with the behavioural break and its fix.
4. Rollback is a revert; no stored state changes shape, and no CRD field is added.
5. Verify live: two Pipelines on one console surface, both Ready and both listed; bare message refused with the choices; each addressable by name with replies in the right thread; a single-Pipeline surface still answering bare messages.

## Open Questions

- **D3's chat-source determination** — settle before implementation.
- **Should the ambiguity refusal create nothing, or create a conversation on request?** Currently it creates nothing, matching how `/agents` and usage errors behave. A "pick one" affordance in the console could turn the refusal into a choice that then originates — pleasant, but it makes a refusal path stateful, so it is out of scope here.
- **Does `/agents` want the profile alongside each name?** The typeahead will show it; the command currently shows names only. Worth aligning, and cheap.
